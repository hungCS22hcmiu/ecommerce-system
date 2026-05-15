package com.ecommerce.product_service.integration;

import com.ecommerce.product_service.dto.AISearchResponse;
import com.ecommerce.product_service.dto.ApiResponse;
import com.ecommerce.product_service.model.Category;
import com.ecommerce.product_service.model.Product;
import com.ecommerce.product_service.model.ProductStatus;
import com.ecommerce.product_service.repository.CategoryRepository;
import com.ecommerce.product_service.repository.ProductRepository;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.github.tomakehurst.wiremock.WireMockServer;
import com.github.tomakehurst.wiremock.core.WireMockConfiguration;
import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.client.TestRestTemplate;
import org.springframework.cache.CacheManager;
import org.springframework.http.ResponseEntity;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.test.context.ActiveProfiles;
import org.springframework.test.context.DynamicPropertyRegistry;
import org.springframework.test.context.DynamicPropertySource;
import org.testcontainers.containers.GenericContainer;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;
import org.testcontainers.utility.DockerImageName;

import java.math.BigDecimal;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.stream.Collectors;
import java.util.stream.IntStream;

import static com.github.tomakehurst.wiremock.client.WireMock.*;
import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
@Testcontainers
@ActiveProfiles("test")
class AISearchIntegrationTest {

    // ── Containers ─────────────────────────────────────────────────────────────

    @Container
    @SuppressWarnings("resource")
    static PostgreSQLContainer<?> postgres = new PostgreSQLContainer<>(
            DockerImageName.parse("pgvector/pgvector:pg15").asCompatibleSubstituteFor("postgres"))
            .withDatabaseName("ecommerce_products")
            .withUsername("postgres")
            .withPassword("postgres")
            .withInitScript("test-pgvector-init.sql");

    @Container
    @SuppressWarnings("resource")
    static GenericContainer<?> redis = new GenericContainer<>(
            DockerImageName.parse("redis:7-alpine"))
            .withExposedPorts(6379);

    static WireMockServer wireMock;

    @BeforeAll
    static void startWireMock() {
        wireMock = new WireMockServer(WireMockConfiguration.options().dynamicPort());
        wireMock.start();
    }

    @AfterAll
    static void stopWireMock() {
        if (wireMock != null) wireMock.stop();
    }

    @DynamicPropertySource
    static void overrideProperties(DynamicPropertyRegistry registry) {
        registry.add("spring.datasource.url", postgres::getJdbcUrl);
        registry.add("spring.datasource.username", postgres::getUsername);
        registry.add("spring.datasource.password", postgres::getPassword);
        registry.add("spring.data.redis.host", redis::getHost);
        registry.add("spring.data.redis.port", () -> redis.getMappedPort(6379));
        registry.add("spring.cache.type", () -> "redis");
        registry.add("ai-service.url", wireMock::baseUrl);
        registry.add("ai-service.timeout-ms", () -> "5000");
    }

    // ── Autowired ──────────────────────────────────────────────────────────────

    @Autowired ProductRepository productRepository;
    @Autowired CategoryRepository categoryRepository;
    @Autowired JdbcTemplate jdbc;
    @Autowired TestRestTemplate restTemplate;
    @Autowired ObjectMapper objectMapper;
    @Autowired CacheManager cacheManager;

    // ── Test data ──────────────────────────────────────────────────────────────

    Category category;
    Product productA; // most similar to query (dim 0)
    Product productB; // moderate similarity (dim 1)
    Product productC; // least similar (dim 2)

    @BeforeEach
    void setUp() {
        var c = cacheManager.getCache("aiSearch");
        if (c != null) c.clear();

        productRepository.deleteAll();
        categoryRepository.deleteAll();

        category = categoryRepository.save(buildCategory());

        productA = productRepository.save(buildProduct("Shoes Runner", category));
        productB = productRepository.save(buildProduct("Laptop Pro", category));
        productC = productRepository.save(buildProduct("Kitchen Blender", category));

        // Drop IVFFLAT index so sequential scan is used (correct for tiny datasets)
        jdbc.execute("DROP INDEX IF EXISTS idx_products_embedding");

        // Assign orthogonal unit vectors: A→dim0, B→dim1, C→dim2
        setEmbedding(productA.getId(), unitVec(0));
        setEmbedding(productB.getId(), unitVec(1));
        setEmbedding(productC.getId(), unitVec(2));

        // Stub: WireMock returns a vector pointing at dim 0 → productA is most similar
        stubEmbedEndpoint(unitVec(0));
    }

    @Test
    void aiSearchReturnsRankedResultsWithModeAi() {
        ResponseEntity<String> raw = restTemplate.getForEntity(
                "/api/v1/products/ai-search?q=running+shoes&limit=3", String.class);

        assertThat(raw.getStatusCode().is2xxSuccessful()).isTrue();

        ApiResponse<AISearchResponse> resp = parseResponse(raw.getBody());
        assertThat(resp.isSuccess()).isTrue();

        AISearchResponse data = resp.getData();
        assertThat(data.mode()).isEqualTo("ai");
        assertThat(data.query()).isEqualTo("running shoes");
        assertThat(data.results()).isNotEmpty();

        // First result must be productA (most similar to query vector)
        assertThat(data.results().get(0).getId()).isEqualTo(productA.getId());
    }

    @Test
    void scoresAreDescending() {
        ResponseEntity<String> raw = restTemplate.getForEntity(
                "/api/v1/products/ai-search?q=running+shoes&limit=3", String.class);

        ApiResponse<AISearchResponse> resp = parseResponse(raw.getBody());
        assertThat(resp.getData().scores()).isNotEmpty();

        var scores = resp.getData().scores();
        for (int i = 1; i < scores.size(); i++) {
            assertThat(scores.get(i - 1)).isGreaterThanOrEqualTo(scores.get(i));
        }
    }

    @Test
    void limitIsRespected() {
        ResponseEntity<String> raw = restTemplate.getForEntity(
                "/api/v1/products/ai-search?q=shoes&limit=1", String.class);

        ApiResponse<AISearchResponse> resp = parseResponse(raw.getBody());
        assertThat(resp.getData().results()).hasSize(1);
    }

    // ── helpers ────────────────────────────────────────────────────────────────

    private void stubEmbedEndpoint(float[] vec) {
        String json = "{\"embedding\":[" +
                IntStream.range(0, vec.length)
                        .mapToObj(i -> String.valueOf(vec[i]))
                        .collect(Collectors.joining(",")) + "]}";
        wireMock.stubFor(post(urlEqualTo("/embed"))
                .willReturn(aResponse()
                        .withHeader("Content-Type", "application/json")
                        .withBody(json)));
    }

    private void setEmbedding(Long productId, float[] vec) {
        String literal = "[" + IntStream.range(0, vec.length)
                .mapToObj(i -> String.valueOf(vec[i]))
                .collect(Collectors.joining(",")) + "]";
        jdbc.update("UPDATE products SET embedding = ?::vector WHERE id = ?", literal, productId);
    }

    private float[] unitVec(int hotDim) {
        float[] v = new float[384];
        v[hotDim] = 1.0f;
        return v;
    }

    @SuppressWarnings("unchecked")
    private ApiResponse<AISearchResponse> parseResponse(String body) {
        try {
            return objectMapper.readValue(body,
                    new TypeReference<>() {});
        } catch (Exception e) {
            throw new RuntimeException("Failed to parse response: " + body, e);
        }
    }

    private Category buildCategory() {
        Category c = new Category();
        c.setName("Test Category");
        c.setSlug("test-category");
        c.setSortOrder(0);
        return c;
    }

    private Product buildProduct(String name, Category cat) {
        return Product.builder()
                .name(name)
                .price(BigDecimal.valueOf(99.99))
                .category(cat)
                .sellerId(java.util.UUID.randomUUID())
                .status(ProductStatus.ACTIVE)
                .stockQuantity(10)
                .stockReserved(0)
                .images(new ArrayList<>())
                .build();
    }
}

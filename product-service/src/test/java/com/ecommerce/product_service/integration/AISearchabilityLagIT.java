package com.ecommerce.product_service.integration;

import com.ecommerce.product_service.dto.AISearchResponse;
import com.ecommerce.product_service.dto.ApiResponse;
import com.ecommerce.product_service.dto.CreateProductRequest;
import com.ecommerce.product_service.model.Category;
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
import org.springframework.http.HttpEntity;
import org.springframework.http.HttpHeaders;
import org.springframework.http.HttpMethod;
import org.springframework.http.MediaType;
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
import java.util.Collections;
import java.util.List;
import java.util.UUID;
import java.util.stream.Collectors;
import java.util.stream.IntStream;

import static com.github.tomakehurst.wiremock.client.WireMock.aResponse;
import static com.github.tomakehurst.wiremock.client.WireMock.post;
import static com.github.tomakehurst.wiremock.client.WireMock.urlEqualTo;
import static org.assertj.core.api.Assertions.assertThat;

/**
 * Phase 2 — Searchability lag.
 *
 * For each iteration: POST a product with a unique tag in its name, then poll
 * /ai-search?q=&lt;tag&gt; until the new product appears in results. Records elapsed
 * time. Target: P95 lag &lt; 1.0s (testing_target.md §4.B).
 *
 * Uses WireMock to stub /embed with a fixed unit vector — every product (and
 * every query) gets the same embedding, so similarity == 1.0 for everything.
 * That makes the only real timing signal the async write-through pipeline
 * (ProductEmbeddingService.scheduleEmbedding) finishing its UPDATE.
 */
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
@Testcontainers
@ActiveProfiles("test")
class AISearchabilityLagIT {

    private static final int ITERATIONS = 30;
    private static final long POLL_INTERVAL_MS = 50;
    private static final long POLL_TIMEOUT_MS = 5000;
    private static final long TARGET_P95_MS = 1000;

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
        stubEmbed();
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
        registry.add("ai-service.write-through-enabled", () -> "true");
    }

    @Autowired ProductRepository productRepository;
    @Autowired CategoryRepository categoryRepository;
    @Autowired JdbcTemplate jdbc;
    @Autowired TestRestTemplate restTemplate;
    @Autowired ObjectMapper objectMapper;
    @Autowired CacheManager cacheManager;

    Category category;
    UUID sellerId;

    @BeforeEach
    void setUp() {
        var c = cacheManager.getCache("aiSearch");
        if (c != null) c.clear();

        productRepository.deleteAll();
        categoryRepository.deleteAll();

        Category cat = new Category();
        cat.setName("Lag Category");
        cat.setSlug("lag-category");
        cat.setSortOrder(0);
        category = categoryRepository.save(cat);

        sellerId = UUID.randomUUID();

        // Tiny dataset → no IVFFLAT scan needed.
        jdbc.execute("DROP INDEX IF EXISTS idx_products_embedding");
    }

    @Test
    void searchabilityLag_p95_under_1s() throws Exception {
        List<Long> lags = new ArrayList<>(ITERATIONS);

        for (int i = 0; i < ITERATIONS; i++) {
            String tag = "lagtest" + UUID.randomUUID().toString().replace("-", "").substring(0, 12);

            long t0 = System.currentTimeMillis();
            Long createdId = createProduct(tag);
            assertThat(createdId).isNotNull();

            long elapsed = pollUntilFindable(tag, createdId);
            lags.add(elapsed);
        }

        long p95 = percentile(lags, 95);
        long p50 = percentile(lags, 50);
        long max = Collections.max(lags);
        System.out.printf("[searchability-lag] iterations=%d p50=%dms p95=%dms max=%dms%n",
                ITERATIONS, p50, p95, max);
        assertThat(p95).as("P95 searchability lag (ms)").isLessThan(TARGET_P95_MS);
    }

    private Long createProduct(String tag) throws Exception {
        CreateProductRequest req = CreateProductRequest.builder()
                .name(tag + " product")
                .description("phase 2 lag test for " + tag)
                .price(BigDecimal.valueOf(9.99))
                .categoryId(category.getId())
                .stockQuantity(1)
                .images(new ArrayList<>())
                .build();

        HttpHeaders headers = new HttpHeaders();
        headers.setContentType(MediaType.APPLICATION_JSON);
        headers.add("X-Seller-Id", sellerId.toString());

        ResponseEntity<String> resp = restTemplate.exchange(
                "/api/v1/products",
                HttpMethod.POST,
                new HttpEntity<>(objectMapper.writeValueAsString(req), headers),
                String.class);

        assertThat(resp.getStatusCode().is2xxSuccessful()).isTrue();
        @SuppressWarnings("unchecked")
        var body = objectMapper.readValue(resp.getBody(), java.util.Map.class);
        @SuppressWarnings("unchecked")
        var data = (java.util.Map<String, Object>) body.get("data");
        Number id = (Number) data.get("id");
        return id.longValue();
    }

    private long pollUntilFindable(String tag, Long createdId) {
        long t0 = System.currentTimeMillis();
        while (System.currentTimeMillis() - t0 < POLL_TIMEOUT_MS) {
            ResponseEntity<String> raw = restTemplate.getForEntity(
                    "/api/v1/products/ai-search?q=" + tag + "&limit=10", String.class);
            if (raw.getStatusCode().is2xxSuccessful() && raw.getBody() != null) {
                ApiResponse<AISearchResponse> resp = parseResponse(raw.getBody());
                if (resp != null && resp.getData() != null
                        && resp.getData().results().stream()
                                .anyMatch(p -> createdId.equals(p.getId()))) {
                    return System.currentTimeMillis() - t0;
                }
            }
            try { Thread.sleep(POLL_INTERVAL_MS); } catch (InterruptedException e) { Thread.currentThread().interrupt(); break; }
        }
        return POLL_TIMEOUT_MS; // count timeout as the cap
    }

    private static void stubEmbed() {
        // Fixed unit vector on dim 0 — every product and every query has the same embedding,
        // so similarity is 1.0 for any product that has had its embedding written.
        StringBuilder vec = new StringBuilder("[1.0");
        for (int i = 1; i < 384; i++) vec.append(",0.0");
        vec.append("]");
        wireMock.stubFor(post(urlEqualTo("/embed"))
                .willReturn(aResponse()
                        .withHeader("Content-Type", "application/json")
                        .withBody("{\"embedding\":" + vec + "}")));
    }

    @SuppressWarnings("unchecked")
    private ApiResponse<AISearchResponse> parseResponse(String body) {
        try {
            return objectMapper.readValue(body, new TypeReference<>() {});
        } catch (Exception e) {
            return null;
        }
    }

    private static long percentile(List<Long> values, int pct) {
        List<Long> sorted = values.stream().sorted().collect(Collectors.toList());
        int idx = (int) Math.ceil(pct / 100.0 * sorted.size()) - 1;
        return sorted.get(Math.max(0, Math.min(sorted.size() - 1, idx)));
    }
}

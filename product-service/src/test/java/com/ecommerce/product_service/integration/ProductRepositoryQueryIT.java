package com.ecommerce.product_service.integration;

import com.ecommerce.product_service.model.Category;
import com.ecommerce.product_service.model.Product;
import com.ecommerce.product_service.model.ProductStatus;
import com.ecommerce.product_service.repository.CategoryRepository;
import com.ecommerce.product_service.repository.ProductRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.dao.InvalidDataAccessResourceUsageException;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.PageRequest;
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
import java.util.List;
import java.util.UUID;
import java.util.stream.Collectors;
import java.util.stream.IntStream;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

/**
 * Phase 4 — Testing-debt coverage for ProductRepository's two native queries:
 * - {@code searchActive} (full-text search with ts_rank ordering)
 * - {@code findIdsBySemanticSimilarity} (pgvector cosine similarity)
 *
 * Existing AISearchIntegrationTest covers the happy path; this IT covers the
 * edge cases called out in testing_plan.md §Phase 4 / testing_target.md §9.A:
 * FTS empty result, tsquery special-character handling, pgvector empty result,
 * and pgvector dimension mismatch.
 */
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.NONE)
@Testcontainers
@ActiveProfiles("test")
class ProductRepositoryQueryIT {

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

    @DynamicPropertySource
    static void overrideProperties(DynamicPropertyRegistry registry) {
        registry.add("spring.datasource.url", postgres::getJdbcUrl);
        registry.add("spring.datasource.username", postgres::getUsername);
        registry.add("spring.datasource.password", postgres::getPassword);
        registry.add("spring.data.redis.host", redis::getHost);
        registry.add("spring.data.redis.port", () -> redis.getMappedPort(6379));
        registry.add("spring.cache.type", () -> "redis");
        // Tests don't hit the embedding service; stub URL with a non-routable port.
        registry.add("ai-service.url", () -> "http://localhost:1");
        registry.add("ai-service.timeout-ms", () -> "100");
        registry.add("ai-service.write-through-enabled", () -> "false");
    }

    @Autowired ProductRepository productRepository;
    @Autowired CategoryRepository categoryRepository;
    @Autowired JdbcTemplate jdbc;

    Category category;

    @BeforeEach
    void setUp() {
        productRepository.deleteAll();
        categoryRepository.deleteAll();

        Category c = new Category();
        c.setName("Q Category");
        c.setSlug("q-category");
        c.setSortOrder(0);
        category = categoryRepository.save(c);

        // Drop IVFFLAT index so a sequential scan is used on tiny datasets.
        jdbc.execute("DROP INDEX IF EXISTS idx_products_embedding");
    }

    // ─────────────────────────── FTS: searchActive ───────────────────────────

    @Test
    void searchActive_returnsEmptyPageWhenNoMatch() {
        productRepository.save(makeProduct("Running shoes for trail"));
        productRepository.save(makeProduct("Aluminum kitchen blender"));

        Page<Product> page = productRepository.searchActive(
                "nonsense_unmatched_keyword_xyz", null, PageRequest.of(0, 10));

        assertThat(page.getContent()).isEmpty();
        assertThat(page.getTotalElements()).isZero();
    }

    @Test
    void searchActive_handlesSpecialCharactersInQuery() {
        // plainto_tsquery normalizes input, so punctuation should NOT crash and should
        // still match indexed lexemes.
        productRepository.save(makeProduct("Running shoes — black & blue (size 10)"));

        // Query with multiple special chars that would break to_tsquery but not plainto_tsquery.
        Page<Product> page = productRepository.searchActive(
                "running !@#$ shoes ; -- ' \"", null, PageRequest.of(0, 10));

        assertThat(page.getContent()).isNotEmpty();
        assertThat(page.getContent().get(0).getName()).contains("Running shoes");
    }

    @Test
    void searchActive_excludesInactiveProducts() {
        Product active = makeProduct("Running shoes red");
        Product inactive = makeProduct("Running shoes blue");
        inactive.setStatus(ProductStatus.INACTIVE);
        productRepository.save(active);
        productRepository.save(inactive);

        Page<Product> page = productRepository.searchActive("running shoes", null, PageRequest.of(0, 10));
        assertThat(page.getContent()).hasSize(1);
        assertThat(page.getContent().get(0).getId()).isEqualTo(active.getId());
    }

    @Test
    void searchActive_categoryFilterScopesResults() {
        Category other = new Category();
        other.setName("Other");
        other.setSlug("other");
        other.setSortOrder(1);
        Category otherSaved = categoryRepository.save(other);

        Product inCat = makeProduct("Trail shoes alpha");
        Product elsewhere = makeProduct("Trail shoes beta");
        elsewhere.setCategory(otherSaved);
        productRepository.save(inCat);
        productRepository.save(elsewhere);

        Page<Product> scoped = productRepository.searchActive(
                "trail shoes", category.getId(), PageRequest.of(0, 10));
        assertThat(scoped.getContent()).hasSize(1);
        assertThat(scoped.getContent().get(0).getId()).isEqualTo(inCat.getId());
    }

    // ────────────────────── pgvector: findIdsBySemanticSimilarity ──────────────────────

    @Test
    void findIdsBySemanticSimilarity_returnsEmptyWhenNoActiveProductsHaveEmbeddings() {
        // Save a product but DO NOT assign an embedding — query must return empty.
        productRepository.save(makeProduct("No embedding here"));

        List<Object[]> rows = productRepository.findIdsBySemanticSimilarity(
                unitLiteral(0), null, 10);

        assertThat(rows).isEmpty();
    }

    @Test
    void findIdsBySemanticSimilarity_returnsSortedByCosineSimilarity() {
        Product a = productRepository.save(makeProduct("A — dim 0"));
        Product b = productRepository.save(makeProduct("B — dim 1"));
        Product c = productRepository.save(makeProduct("C — dim 2"));
        setEmbedding(a.getId(), unitVec(0));
        setEmbedding(b.getId(), unitVec(1));
        setEmbedding(c.getId(), unitVec(2));

        // Query vector pointing at dim 0 → product A is most similar (cos = 1.0).
        List<Object[]> rows = productRepository.findIdsBySemanticSimilarity(
                unitLiteral(0), null, 10);

        assertThat(rows).hasSize(3);
        // First result's id must be A's id.
        Long firstId = ((Number) rows.get(0)[0]).longValue();
        assertThat(firstId).isEqualTo(a.getId());
        // Score (col 1) of first row must be the max.
        double firstScore = ((Number) rows.get(0)[1]).doubleValue();
        double lastScore = ((Number) rows.get(rows.size() - 1)[1]).doubleValue();
        assertThat(firstScore).isGreaterThanOrEqualTo(lastScore);
    }

    @Test
    void findIdsBySemanticSimilarity_dimensionMismatch_throws() {
        // Save a product with a real 384-dim embedding...
        Product p = productRepository.save(makeProduct("dim-test"));
        setEmbedding(p.getId(), unitVec(0));

        // ...then query with a 4-dimensional vector. pgvector must reject it.
        String wrongDimLiteral = "[1.0,0.0,0.0,0.0]";

        assertThatThrownBy(() ->
                productRepository.findIdsBySemanticSimilarity(wrongDimLiteral, null, 5))
                .isInstanceOfAny(InvalidDataAccessResourceUsageException.class,
                                 org.springframework.dao.DataIntegrityViolationException.class,
                                 org.springframework.jdbc.UncategorizedSQLException.class,
                                 org.springframework.orm.jpa.JpaSystemException.class);
    }

    @Test
    void findIdsBySemanticSimilarity_limitIsHonored() {
        for (int i = 0; i < 5; i++) {
            Product p = productRepository.save(makeProduct("p-" + i));
            setEmbedding(p.getId(), unitVec(i % 384));
        }

        List<Object[]> rows = productRepository.findIdsBySemanticSimilarity(
                unitLiteral(0), null, 3);
        assertThat(rows).hasSize(3);
    }

    // ────────────────────── helpers ──────────────────────

    private Product makeProduct(String name) {
        return Product.builder()
                .name(name)
                .price(BigDecimal.valueOf(9.99))
                .category(category)
                .sellerId(UUID.randomUUID())
                .status(ProductStatus.ACTIVE)
                .stockQuantity(10)
                .stockReserved(0)
                .images(new ArrayList<>())
                .build();
    }

    private void setEmbedding(Long productId, float[] vec) {
        jdbc.update("UPDATE products SET embedding = ?::vector WHERE id = ?",
                literal(vec), productId);
    }

    private float[] unitVec(int hotDim) {
        float[] v = new float[384];
        v[hotDim % 384] = 1.0f;
        return v;
    }

    private String unitLiteral(int hotDim) {
        return literal(unitVec(hotDim));
    }

    private String literal(float[] v) {
        return "[" + IntStream.range(0, v.length)
                .mapToObj(i -> String.valueOf(v[i]))
                .collect(Collectors.joining(",")) + "]";
    }
}

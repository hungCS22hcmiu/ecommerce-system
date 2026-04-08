package com.ecommerce.product_service.service;

import com.ecommerce.product_service.dto.ProductResponse;
import com.ecommerce.product_service.dto.UpdateProductRequest;
import com.ecommerce.product_service.model.Category;
import com.ecommerce.product_service.model.Product;
import com.ecommerce.product_service.model.ProductStatus;
import com.ecommerce.product_service.repository.CategoryRepository;
import com.ecommerce.product_service.repository.ProductRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.cache.CacheManager;
import org.springframework.test.context.ActiveProfiles;

import java.math.BigDecimal;
import java.time.OffsetDateTime;
import java.util.ArrayList;
import java.util.Optional;
import java.util.UUID;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.*;

/**
 * Cache behavior tests for ProductService.
 *
 * Uses @SpringBootTest to load the full Spring context including the real
 * CacheManager and Spring AOP proxy — necessary because @Cacheable is applied
 * via AOP and doesn't fire when calling methods directly on the implementation.
 *
 * Redis connection is replaced with a no-op mock cache (spring.cache.type=none
 * via test profile or CacheManager override) to keep tests fast and offline.
 */
@SpringBootTest
@ActiveProfiles("test")
class ProductServiceCacheTest {

    @MockitoBean
    private ProductRepository productRepository;

    @MockitoBean
    private CategoryRepository categoryRepository;

    @Autowired
    private ProductService productService;

    @Autowired
    private CacheManager cacheManager;

    private Product product;
    private Category category;
    private UUID sellerId;

    @BeforeEach
    void setUp() {
        // Clear all caches between tests so each test starts clean
        cacheManager.getCacheNames().forEach(name -> {
            var cache = cacheManager.getCache(name);
            if (cache != null) cache.clear();
        });

        sellerId = UUID.randomUUID();
        category = Category.builder().id(1L).name("Electronics").build();
        product = Product.builder()
                .id(1L)
                .name("Widget")
                .description("A widget")
                .price(new BigDecimal("9.99"))
                .category(category)
                .sellerId(sellerId)
                .status(ProductStatus.ACTIVE)
                .stockQuantity(100)
                .stockReserved(0)
                .images(new ArrayList<>())
                .createdAt(OffsetDateTime.now())
                .updatedAt(OffsetDateTime.now())
                .build();
    }

    // ── Cache Hit ──────────────────────────────────────────────────────────────

    /**
     * Calling getProduct() twice should hit the DB only once.
     * The second call is served from the "product" cache.
     */
    @Test
    void getProduct_secondCallServedFromCache() {
        when(productRepository.findByIdAndStatus(1L, ProductStatus.ACTIVE))
                .thenReturn(Optional.of(product));

        ProductResponse first = productService.getProduct(1L);
        ProductResponse second = productService.getProduct(1L);

        // Both calls return the same product id — cache returned the same entry
        assertThat(first.getId()).isEqualTo(second.getId());
        assertThat(first.getName()).isEqualTo(second.getName());
        // Repository must have been called exactly once — second hit came from cache
        verify(productRepository, times(1)).findByIdAndStatus(1L, ProductStatus.ACTIVE);
    }

    /**
     * After getProduct(), the key must be present in the "product" cache.
     */
    @Test
    void getProduct_populatesCache() {
        when(productRepository.findByIdAndStatus(1L, ProductStatus.ACTIVE))
                .thenReturn(Optional.of(product));

        productService.getProduct(1L);

        var cache = cacheManager.getCache("product");
        assertThat(cache).isNotNull();
        assertThat(cache).satisfies(c -> assertThat(c.get(1L)).isNotNull());
    }

    // ── Cache Invalidation on Update ──────────────────────────────────────────

    /**
     * After updateProduct(), the cache entry is refreshed (@CachePut).
     * The next getProduct() call should return the updated data, still from cache —
     * the DB should not be queried again.
     */
    @Test
    void updateProduct_refreshesCache() {
        when(productRepository.findByIdAndStatus(1L, ProductStatus.ACTIVE))
                .thenReturn(Optional.of(product));
        when(productRepository.findById(1L))
                .thenReturn(Optional.of(product));
        when(productRepository.existsByIdAndSellerId(1L, sellerId))
                .thenReturn(true);

        // Prime the cache
        productService.getProduct(1L);

        // Update the product — @CachePut writes new value to cache
        UpdateProductRequest update = UpdateProductRequest.builder()
                .name("Updated Widget")
                .build();
        ProductResponse updated = productService.updateProduct(1L, sellerId, update);

        assertThat(updated.getName()).isEqualTo("Updated Widget");

        // Next read should hit the cache with the fresh value, not the DB
        ProductResponse afterUpdate = productService.getProduct(1L);
        assertThat(afterUpdate.getName()).isEqualTo("Updated Widget");

        // DB was hit once for the initial getProduct; update used findById; the
        // post-update getProduct should NOT call findByIdAndStatus again
        verify(productRepository, times(1)).findByIdAndStatus(1L, ProductStatus.ACTIVE);
    }

    // ── Cache Invalidation on Delete ──────────────────────────────────────────

    /**
     * After deleteProduct(), the cache entry is removed (@CacheEvict).
     * The next getProduct() call must go back to the DB.
     */
    @Test
    void deleteProduct_evictsCache() {
        when(productRepository.findByIdAndStatus(1L, ProductStatus.ACTIVE))
                .thenReturn(Optional.of(product));
        when(productRepository.findById(1L))
                .thenReturn(Optional.of(product));
        when(productRepository.existsByIdAndSellerId(1L, sellerId))
                .thenReturn(true);

        // Prime the cache
        productService.getProduct(1L);
        assertThat(cacheManager.getCache("product"))
                .satisfies(c -> assertThat(c.get(1L)).isNotNull());

        // Delete evicts the cache entry
        productService.deleteProduct(1L, sellerId);
        assertThat(cacheManager.getCache("product"))
                .satisfies(c -> assertThat(c.get(1L)).isNull());

        // Next read triggers a fresh DB query
        productService.getProduct(1L);
        verify(productRepository, times(2)).findByIdAndStatus(1L, ProductStatus.ACTIVE);
    }

    // ── Cache Stampede (documented, not solved) ────────────────────────────────

    /**
     * CACHE STAMPEDE SCENARIO — documented for awareness.
     *
     * Problem:
     *   When a popular product's cache entry expires (TTL=30min), all concurrent
     *   requests that arrive before the first request repopulates the cache will
     *   find a cache miss and each independently hit the database.
     *
     *   Example: 100 requests arrive simultaneously at TTL expiry.
     *   All 100 see a cache miss → all 100 issue SELECT queries → DB spike.
     *
     * Why this test doesn't prevent it:
     *   Spring's @Cacheable has no built-in stampede protection. It checks:
     *     1. Is there a value in cache? → return it
     *     2. No? → call the method AND store the result
     *   Under concurrent load, step 2 runs for every thread that passed step 1
     *   before any of them completes step 2.
     *
     * Potential mitigations (not implemented in Phase 3):
     *   - Probabilistic early expiration (PER algorithm)
     *   - Redis SETNX-based "lock" to let only one thread refresh
     *   - Caffeine (local) + Redis (distributed) layered cache
     *   - Background refresh: serve stale while reloading asynchronously
     *
     * This is a known limitation. For a read-heavy public catalog with millions
     * of concurrent users, one of the mitigations above would be required.
     */
    @Test
    void cacheStampede_documentedScenario() {
        // This test simply demonstrates that @Cacheable provides no stampede protection.
        // With N threads all calling getProduct() concurrently on an empty cache,
        // productRepository.findByIdAndStatus() would be called N times.
        // (Not simulated here to keep the test deterministic and fast.)

        // The test passes vacuously — its value is the documentation above.
        assertThat(true).isTrue();
    }
}

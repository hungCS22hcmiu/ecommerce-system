package com.ecommerce.product_service.integration;

import com.ecommerce.product_service.exception.InsufficientStockException;
import com.ecommerce.product_service.model.MovementType;
import com.ecommerce.product_service.model.Product;
import com.ecommerce.product_service.model.ProductStatus;
import com.ecommerce.product_service.model.StockMovement;
import com.ecommerce.product_service.repository.ProductRepository;
import com.ecommerce.product_service.repository.StockMovementRepository;
import com.ecommerce.product_service.service.InventoryService;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.test.context.ActiveProfiles;
import org.springframework.test.context.DynamicPropertyRegistry;
import org.springframework.test.context.DynamicPropertySource;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;
import org.testcontainers.utility.DockerImageName;

import java.math.BigDecimal;
import java.util.ArrayList;
import java.util.List;
import java.util.UUID;
import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicInteger;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * Proves that the conditional UPDATE strategy correctly handles concurrent stock
 * reservations without lost updates or false OOS errors.
 *
 * Each reservation executes a single atomic SQL UPDATE with a WHERE guard:
 *   UPDATE products SET stock_reserved = stock_reserved + qty
 *   WHERE id = ? AND (stock_quantity - stock_reserved) >= qty
 * The DB serializes concurrent writes at the row level — no retry loop needed.
 */
@SpringBootTest
@Testcontainers
@ActiveProfiles("test")
class InventoryConcurrencyTest {

    @Container
    static PostgreSQLContainer<?> postgres = new PostgreSQLContainer<>(
            DockerImageName.parse("pgvector/pgvector:pg16").asCompatibleSubstituteFor("postgres"))
            .withDatabaseName("ecommerce_products")
            .withUsername("postgres")
            .withPassword("postgres")
            .withInitScript("test-pgvector-init.sql");

    @DynamicPropertySource
    static void configureDataSource(DynamicPropertyRegistry registry) {
        registry.add("spring.datasource.url", postgres::getJdbcUrl);
        registry.add("spring.datasource.username", postgres::getUsername);
        registry.add("spring.datasource.password", postgres::getPassword);
    }

    @Autowired
    private InventoryService inventoryService;

    @Autowired
    private ProductRepository productRepository;

    @Autowired
    private StockMovementRepository stockMovementRepository;

    private Long productId;

    @BeforeEach
    void setUp() {
        stockMovementRepository.deleteAll();
        productRepository.deleteAll();

        Product product = Product.builder()
                .name("Concurrent Test Product")
                .price(new BigDecimal("49.99"))
                .sellerId(UUID.randomUUID())
                .status(ProductStatus.ACTIVE)
                .stockQuantity(5)
                .stockReserved(0)
                .build();

        productId = productRepository.save(product).getId();
    }

    /**
     * 10 threads compete to reserve 1 unit each from a stock of 5.
     * Exactly 5 succeed. The other 5 fail with InsufficientStockException (genuine OOS).
     *
     * With conditional UPDATE, the DB serializes concurrent writes at the row level.
     * The 5 threads that arrive after stock is exhausted get 0 rows updated and
     * immediately receive InsufficientStockException — no retries, no false failures.
     *
     * Core invariant: final stockReserved == 5 and exactly 5 RESERVE movements.
     */
    @Test
    void concurrent_reservations_exactly_five_succeed() throws InterruptedException {
        int threadCount = 10;
        ExecutorService executor = Executors.newFixedThreadPool(threadCount);
        CountDownLatch startGate = new CountDownLatch(1);

        AtomicInteger successCount = new AtomicInteger(0);
        AtomicInteger failCount = new AtomicInteger(0);
        List<Future<Void>> futures = new ArrayList<>();

        for (int i = 0; i < threadCount; i++) {
            final String orderId = "order-" + i;
            futures.add(executor.submit(() -> {
                startGate.await(); // all threads start simultaneously
                try {
                    inventoryService.reserveStock(productId, 1, orderId);
                    successCount.incrementAndGet();
                } catch (InsufficientStockException e) {
                    failCount.incrementAndGet();
                }
                return null;
            }));
        }

        startGate.countDown(); // release all threads at once

        for (Future<Void> f : futures) {
            try {
                f.get(10, TimeUnit.SECONDS);
            } catch (ExecutionException | TimeoutException e) {
                throw new RuntimeException("Thread threw unexpected exception", e);
            }
        }
        executor.shutdown();

        // Core invariant: exactly 5 units reserved, no lost updates
        assertThat(successCount.get()).isEqualTo(5);
        assertThat(failCount.get()).isEqualTo(5);

        Product finalProduct = productRepository.findById(productId).orElseThrow();
        assertThat(finalProduct.getStockReserved()).isEqualTo(5);

        // Exactly 5 audit records — proves no double-writes
        List<StockMovement> movements = stockMovementRepository.findAll();
        long reserveMovements = movements.stream()
                .filter(m -> m.getType() == MovementType.RESERVE)
                .count();
        assertThat(reserveMovements).isEqualTo(5);
    }

    /**
     * 3 threads compete with ample stock (20 units). All 3 must succeed.
     *
     * With conditional UPDATE each thread's UPDATE either succeeds atomically or
     * fails immediately (no retries). With 20 units and only 3 threads reserving 1
     * each, the condition is satisfied for all — no contention on the guard clause.
     */
    @Test
    void concurrent_reserve_all_succeed_when_stock_sufficient() throws InterruptedException {
        Product product = productRepository.findById(productId).orElseThrow();
        product.setStockQuantity(20);
        productRepository.save(product);

        int threadCount = 3;
        ExecutorService executor = Executors.newFixedThreadPool(threadCount);
        CountDownLatch startGate = new CountDownLatch(1);

        AtomicInteger successCount = new AtomicInteger(0);
        List<Future<Void>> futures = new ArrayList<>();

        for (int i = 0; i < threadCount; i++) {
            final String orderId = "order-sufficient-" + i;
            futures.add(executor.submit(() -> {
                startGate.await();
                try {
                    inventoryService.reserveStock(productId, 1, orderId);
                    successCount.incrementAndGet();
                } catch (InsufficientStockException e) {
                    // should not happen: 3 threads, 20 stock
                }
                return null;
            }));
        }

        startGate.countDown();

        for (Future<Void> f : futures) {
            try {
                f.get(15, TimeUnit.SECONDS);
            } catch (ExecutionException | TimeoutException e) {
                throw new RuntimeException("Thread threw unexpected exception", e);
            }
        }
        executor.shutdown();

        // All 3 must succeed — conditional UPDATE handles concurrent writes without retries
        assertThat(successCount.get()).isEqualTo(threadCount);

        Product finalProduct = productRepository.findById(productId).orElseThrow();
        assertThat(finalProduct.getStockReserved()).isEqualTo(threadCount);
    }
}

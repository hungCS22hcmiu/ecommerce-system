package com.ecommerce.order_service.integration;

import com.ecommerce.order_service.client.ProductServiceClient;
import com.ecommerce.order_service.exception.InvalidOrderStateException;
import com.ecommerce.order_service.kafka.OrderEventProducer;
import com.ecommerce.order_service.model.Order;
import com.ecommerce.order_service.model.OrderStatus;
import com.ecommerce.order_service.model.ShippingAddress;
import com.ecommerce.order_service.repository.OrderRepository;
import com.ecommerce.order_service.repository.OrderStatusHistoryRepository;
import com.ecommerce.order_service.service.OrderService;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.kafka.test.context.EmbeddedKafka;
import org.springframework.test.context.ActiveProfiles;
import org.springframework.test.context.DynamicPropertyRegistry;
import org.springframework.test.context.DynamicPropertySource;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

import java.math.BigDecimal;
import java.util.UUID;
import java.util.concurrent.Callable;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.stream.Stream;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * Verifies that the pessimistic lock on order state transitions prevents
 * two concurrent requests from both succeeding on the same order.
 *
 * Scenario:
 *   - Order starts in CONFIRMED state
 *   - Thread A tries CONFIRMED → SHIPPED
 *   - Thread B tries CONFIRMED → CANCELLED
 *   - Both fire simultaneously via CountDownLatch
 *
 * Expected: exactly one succeeds, the other receives InvalidOrderStateException
 * because the winner's commit changes the status before the loser acquires the lock.
 */
@SpringBootTest(properties = "spring.kafka.bootstrap-servers=${spring.embedded.kafka.brokers}")
@EmbeddedKafka(partitions = 1, topics = {"orders.created", "payments.completed", "payments.failed"})
@Testcontainers
@ActiveProfiles("test")
class OrderConcurrencyTest {

    @Container
    static PostgreSQLContainer<?> postgres = new PostgreSQLContainer<>("postgres:16-alpine")
            .withDatabaseName("ecommerce_orders")
            .withUsername("postgres")
            .withPassword("postgres");

    @DynamicPropertySource
    static void configureDataSource(DynamicPropertyRegistry registry) {
        registry.add("spring.datasource.url",      postgres::getJdbcUrl);
        registry.add("spring.datasource.username", postgres::getUsername);
        registry.add("spring.datasource.password", postgres::getPassword);
    }

    // Stub out external dependencies — only the DB layer needs to be real
    @MockitoBean private ProductServiceClient productServiceClient;
    @MockitoBean private OrderEventProducer    orderEventProducer;

    @Autowired private OrderService                orderService;
    @Autowired private OrderRepository             orderRepository;
    @Autowired private OrderStatusHistoryRepository historyRepository;

    @BeforeEach
    void cleanUp() {
        historyRepository.deleteAll();
        orderRepository.deleteAll();
    }

    // ── Tests ─────────────────────────────────────────────────────────────────

    /**
     * Core pessimistic lock test:
     * Two threads race to transition the same CONFIRMED order.
     * The DB lock guarantees exactly one wins.
     */
    @Test
    void concurrent_stateTransitions_exactlyOneWins() throws Exception {
        Order order = createConfirmedOrder();

        CountDownLatch startGate = new CountDownLatch(1);

        Callable<String> shipTask = () -> {
            startGate.await();
            try {
                orderService.updateOrderStatus(order.getId(), OrderStatus.SHIPPED,
                        "Shipped by seller", "thread-ship");
                return "success";
            } catch (InvalidOrderStateException e) {
                return "failed:" + e.getMessage();
            }
        };

        Callable<String> cancelTask = () -> {
            startGate.await();
            try {
                orderService.updateOrderStatus(order.getId(), OrderStatus.CANCELLED,
                        "Customer cancelled", "thread-cancel");
                return "success";
            } catch (InvalidOrderStateException e) {
                return "failed:" + e.getMessage();
            }
        };

        ExecutorService executor = Executors.newFixedThreadPool(2);
        Future<String> f1 = executor.submit(shipTask);
        Future<String> f2 = executor.submit(cancelTask);

        startGate.countDown(); // release both threads simultaneously

        String result1 = f1.get(10, TimeUnit.SECONDS);
        String result2 = f2.get(10, TimeUnit.SECONDS);
        executor.shutdown();

        long successCount = Stream.of(result1, result2).filter("success"::equals).count();
        long failureCount = Stream.of(result1, result2).filter(r -> r.startsWith("failed")).count();

        assertThat(successCount)
                .as("Exactly one thread must win the state transition")
                .isEqualTo(1);
        assertThat(failureCount)
                .as("Exactly one thread must lose with InvalidOrderStateException")
                .isEqualTo(1);

        // Final DB state must be either SHIPPED or CANCELLED — never CONFIRMED
        Order finalOrder = orderRepository.findById(order.getId()).orElseThrow();
        assertThat(finalOrder.getStatus())
                .as("Order must have moved out of CONFIRMED regardless of which thread won")
                .isIn(OrderStatus.SHIPPED, OrderStatus.CANCELLED);
    }

    /**
     * Repeated-fire test: run 5 concurrent races on fresh orders to ensure
     * the lock holds consistently, not just by lucky serialization.
     */
    @Test
    void repeated_concurrent_transitions_alwaysExactlyOneWins() throws Exception {
        for (int i = 0; i < 5; i++) {
            Order order = createConfirmedOrder();

            CountDownLatch startGate = new CountDownLatch(1);
            Callable<String> ship   = raceTask(order.getId(), OrderStatus.SHIPPED,   startGate);
            Callable<String> cancel = raceTask(order.getId(), OrderStatus.CANCELLED, startGate);

            ExecutorService executor = Executors.newFixedThreadPool(2);
            Future<String> f1 = executor.submit(ship);
            Future<String> f2 = executor.submit(cancel);

            startGate.countDown();
            String r1 = f1.get(10, TimeUnit.SECONDS);
            String r2 = f2.get(10, TimeUnit.SECONDS);
            executor.shutdown();

            long wins = Stream.of(r1, r2).filter("success"::equals).count();
            assertThat(wins)
                    .as("Race %d: exactly one winner expected", i + 1)
                    .isEqualTo(1);
        }
    }

    // ── Helpers ───────────────────────────────────────────────────────────────

    private Order createConfirmedOrder() {
        Order order = Order.builder()
                .userId(UUID.randomUUID())
                .cartId(UUID.randomUUID())
                .status(OrderStatus.CONFIRMED)
                .totalAmount(BigDecimal.valueOf(100))
                .shippingAddress(new ShippingAddress("1 Main St", "HCMC", "HCM", "VN", "70000"))
                .build();
        return orderRepository.save(order);
    }

    private Callable<String> raceTask(UUID orderId, OrderStatus target, CountDownLatch gate) {
        return () -> {
            gate.await();
            try {
                orderService.updateOrderStatus(orderId, target, "race", "thread-" + target);
                return "success";
            } catch (InvalidOrderStateException e) {
                return "failed";
            }
        };
    }
}

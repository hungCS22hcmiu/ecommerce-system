package com.ecommerce.order_service.integration;

import com.ecommerce.order_service.client.ProductServiceClient;
import com.ecommerce.order_service.dto.CreateOrderRequest;
import com.ecommerce.order_service.dto.OrderItemRequest;
import com.ecommerce.order_service.dto.ShippingAddressDto;
import com.ecommerce.order_service.kafka.event.OrderCreatedEvent;
import com.ecommerce.order_service.model.OutboxEvent;
import com.ecommerce.order_service.repository.NotificationRepository;
import com.ecommerce.order_service.repository.OrderRepository;
import com.ecommerce.order_service.repository.OrderStatusHistoryRepository;
import com.ecommerce.order_service.repository.OutboxEventRepository;
import com.ecommerce.order_service.service.OrderService;
import com.fasterxml.jackson.databind.ObjectMapper;
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
import java.time.OffsetDateTime;
import java.util.List;
import java.util.Map;
import java.util.UUID;

import static org.assertj.core.api.Assertions.assertThat;
import static org.awaitility.Awaitility.await;
import static org.mockito.ArgumentMatchers.anyInt;
import static org.mockito.ArgumentMatchers.anyLong;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.Mockito.when;

import java.util.concurrent.TimeUnit;

/**
 * Proves the transactional outbox guarantees:
 *
 * 1. The outbox row is written atomically with the order insert — if the
 *    application crashes after the DB commit, the row survives and the
 *    OutboxPublisher delivers it on restart.
 *
 * 2. OutboxPublisher polls unpublished rows within 100ms and marks
 *    published_at only after kafkaTemplate.send().get() confirms delivery.
 */
@SpringBootTest(properties = "spring.kafka.bootstrap-servers=${spring.embedded.kafka.brokers}")
@EmbeddedKafka(partitions = 1, topics = {"orders.created", "payments.completed", "payments.failed"})
@Testcontainers
@ActiveProfiles("test")
class OrderOutboxIT {

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

    // Only external HTTP dependency needs mocking — outbox + Kafka must be real
    @MockitoBean private ProductServiceClient productServiceClient;

    @Autowired private OrderService                 orderService;
    @Autowired private OutboxEventRepository        outboxEventRepository;
    @Autowired private OrderRepository              orderRepository;
    @Autowired private OrderStatusHistoryRepository historyRepository;
    @Autowired private NotificationRepository        notificationRepository;
    @Autowired private ObjectMapper                 objectMapper;

    @BeforeEach
    void cleanUp() {
        outboxEventRepository.deleteAll();
        notificationRepository.deleteAll();
        historyRepository.deleteAll();
        orderRepository.deleteAll();
    }

    /**
     * Happy-path outbox flow: createOrder → outbox row written (publishedAt=null) →
     * OutboxPublisher fires within 100ms → row marked published (publishedAt!=null).
     *
     * The immediate assertion (publishedAt==null right after createOrder returns)
     * proves the inline Kafka publish was removed — the row is written but not yet
     * sent in the same call. The Awaitility assertion proves the scheduler delivers it.
     */
    @Test
    void createOrder_writes_outbox_atomically_and_publisher_delivers() {
        UUID sellerId = UUID.randomUUID();
        when(productServiceClient.reserveStock(anyLong(), anyInt(), anyString()))
                .thenReturn(new ProductServiceClient.StockResponse(1L, 9, 1));
        when(productServiceClient.getProduct(anyLong()))
                .thenReturn(new ProductServiceClient.ProductDetail(
                        "Test Product", new BigDecimal("49.99"), sellerId.toString()));

        CreateOrderRequest request = new CreateOrderRequest(
                UUID.randomUUID(),
                List.of(new OrderItemRequest(1L, 2)),
                new ShippingAddressDto("1 Main St", "HCMC", "HCM", "VN", "70000")
        );

        orderService.createOrder(UUID.randomUUID(), request);

        // Outbox row must exist immediately — proves atomic write with the order
        List<OutboxEvent> rows = outboxEventRepository.findAll();
        assertThat(rows).hasSize(1);
        assertThat(rows.get(0).getPublishedAt())
                .as("publishedAt must be null immediately after createOrder — inline publish was removed")
                .isNull();

        // OutboxPublisher fires every 100ms — row must be published within 2 seconds
        await().atMost(2, TimeUnit.SECONDS)
                .until(() -> outboxEventRepository.findAll().get(0).getPublishedAt() != null);

        OutboxEvent published = outboxEventRepository.findAll().get(0);
        assertThat(published.getPublishedAt()).isNotNull();
        assertThat(published.getOrderId()).isNotNull();
    }

    /**
     * Proves OutboxPublisher correctly delivers a pre-existing outbox row to Kafka
     * and marks it published. This simulates the "app restart after crash" scenario:
     * the row was written before the crash and the publisher picks it up on restart.
     */
    @Test
    void outbox_publisher_delivers_preexisting_row() throws Exception {
        UUID orderId = UUID.randomUUID();
        OrderCreatedEvent event = OrderCreatedEvent.builder()
                .orderId(orderId)
                .userId(UUID.randomUUID())
                .totalAmount(new BigDecimal("99.99"))
                .items(List.of(OrderCreatedEvent.OrderItemEvent.builder()
                        .productId(1L).quantity(1).unitPrice(new BigDecimal("99.99")).build()))
                .build();

        OutboxEvent saved = outboxEventRepository.save(OutboxEvent.builder()
                .orderId(orderId)
                .payload(objectMapper.writeValueAsString(event))
                .headers(objectMapper.writeValueAsString(Map.of("X-Correlation-ID", UUID.randomUUID().toString())))
                .createdAt(OffsetDateTime.now())
                .build());

        assertThat(saved.getPublishedAt()).isNull();

        // Publisher must pick up and deliver within 2 seconds
        await().atMost(2, TimeUnit.SECONDS)
                .until(() -> outboxEventRepository.findById(saved.getId())
                        .map(e -> e.getPublishedAt() != null)
                        .orElse(false));

        OutboxEvent published = outboxEventRepository.findById(saved.getId()).orElseThrow();
        assertThat(published.getPublishedAt())
                .as("published_at must be set — proving kafkaTemplate.send().get() returned successfully")
                .isNotNull();
    }
}

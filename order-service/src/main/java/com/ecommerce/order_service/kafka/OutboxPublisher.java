package com.ecommerce.order_service.kafka;

import com.ecommerce.order_service.kafka.event.OrderCreatedEvent;
import com.ecommerce.order_service.model.Order;
import com.ecommerce.order_service.model.OutboxEvent;
import com.ecommerce.order_service.repository.OrderRepository;
import com.ecommerce.order_service.repository.OutboxEventRepository;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.apache.kafka.clients.producer.ProducerRecord;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;
import org.springframework.transaction.annotation.Transactional;

import org.slf4j.MDC;

import java.nio.charset.StandardCharsets;
import java.time.OffsetDateTime;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.TimeUnit;
import java.util.stream.Collectors;

@Slf4j
@Component
@RequiredArgsConstructor
public class OutboxPublisher {

    private static final String TOPIC = "orders.created";

    private final OutboxEventRepository outboxRepo;
    private final OrderRepository orderRepository;
    private final KafkaTemplate<String, Object> kafkaTemplate;
    private final ObjectMapper objectMapper;

    @Scheduled(fixedDelay = 100)
    @Transactional
    public void publishPending() {
        List<OutboxEvent> batch = outboxRepo.findUnpublishedForUpdate();
        for (OutboxEvent ev : batch) {
            try {
                OrderCreatedEvent event = objectMapper.readValue(ev.getPayload(), OrderCreatedEvent.class);
                Map<String, String> hdrs = objectMapper.readValue(ev.getHeaders(),
                        new TypeReference<Map<String, String>>() {});

                String correlationId = hdrs.getOrDefault("X-Correlation-ID", UUID.randomUUID().toString());
                MDC.put("correlationId", correlationId);

                ProducerRecord<String, Object> record = new ProducerRecord<>(
                        TOPIC, ev.getOrderId().toString(), event);
                hdrs.forEach((k, v) -> record.headers().add(k, v.getBytes(StandardCharsets.UTF_8)));

                kafkaTemplate.send(record).get(5, TimeUnit.SECONDS);
                ev.setPublishedAt(OffsetDateTime.now());
                outboxRepo.save(ev);
                log.info("Outbox published orderId={}", ev.getOrderId());
            } catch (Exception e) {
                log.warn("Outbox publish failed for id={}", ev.getId(), e);
            } finally {
                MDC.clear();
            }
        }
    }

    @Scheduled(fixedDelay = 60_000)
    @Transactional
    public void reapStuckPendingOrders() {
        OffsetDateTime threshold = OffsetDateTime.now().minusMinutes(2);
        List<String> ids = orderRepository.findStuckPendingOrderIds(threshold);
        for (String idStr : ids) {
            UUID orderId = UUID.fromString(idStr);
            try {
                Order order = orderRepository.findById(orderId).orElse(null);
                if (order == null) continue;

                OrderCreatedEvent event = OrderCreatedEvent.builder()
                        .orderId(order.getId())
                        .userId(order.getUserId())
                        .totalAmount(order.getTotalAmount())
                        .items(order.getItems().stream()
                                .map(i -> OrderCreatedEvent.OrderItemEvent.builder()
                                        .productId(i.getProductId())
                                        .quantity(i.getQuantity())
                                        .unitPrice(i.getUnitPrice())
                                        .build())
                                .collect(Collectors.toList()))
                        .build();

                outboxRepo.save(OutboxEvent.builder()
                        .orderId(orderId)
                        .payload(objectMapper.writeValueAsString(event))
                        .headers(objectMapper.writeValueAsString(
                                Map.of("X-Correlation-ID", UUID.randomUUID().toString())))
                        .createdAt(OffsetDateTime.now())
                        .build());
                log.warn("Reaper re-queued stuck PENDING orderId={}", orderId);
            } catch (Exception e) {
                log.error("Reaper failed for orderId={}", orderId, e);
            }
        }
    }
}

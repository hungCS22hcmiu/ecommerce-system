package com.ecommerce.order_service.kafka;

import com.ecommerce.order_service.kafka.event.OrderCreatedEvent;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.apache.kafka.clients.producer.ProducerRecord;
import org.slf4j.MDC;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.stereotype.Component;

import java.nio.charset.StandardCharsets;
import java.util.Optional;
import java.util.UUID;

@Slf4j
@Component
@RequiredArgsConstructor
public class OrderEventProducer {

    private static final String ORDERS_CREATED_TOPIC = "orders.created";
    private static final String CORR_HEADER = "X-Correlation-ID";

    private final KafkaTemplate<String, Object> kafkaTemplate;

    public void publishOrderCreated(OrderCreatedEvent event) {
        String correlationId = Optional.ofNullable(MDC.get("correlationId"))
                .orElse(UUID.randomUUID().toString());

        ProducerRecord<String, Object> record = new ProducerRecord<>(
                ORDERS_CREATED_TOPIC, event.getOrderId().toString(), event);
        record.headers().add(CORR_HEADER, correlationId.getBytes(StandardCharsets.UTF_8));

        kafkaTemplate.send(record);
        log.info("Published order.created orderId={} correlationId={}", event.getOrderId(), correlationId);
    }
}

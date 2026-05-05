package com.ecommerce.order_service.kafka;

import com.ecommerce.order_service.kafka.event.OrderCreatedEvent;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.stereotype.Component;

@Slf4j
@Component
@RequiredArgsConstructor
public class OrderEventProducer {

    private static final String ORDERS_CREATED_TOPIC = "orders.created";

    private final KafkaTemplate<String, Object> kafkaTemplate;

    public void publishOrderCreated(OrderCreatedEvent event) {
        log.info("Publishing order.created event for orderId={}", event.getOrderId());
        kafkaTemplate.send(ORDERS_CREATED_TOPIC, event.getOrderId().toString(), event);
    }
}

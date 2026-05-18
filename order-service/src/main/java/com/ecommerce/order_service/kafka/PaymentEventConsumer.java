package com.ecommerce.order_service.kafka;

import com.ecommerce.order_service.kafka.event.PaymentCompletedEvent;
import com.ecommerce.order_service.kafka.event.PaymentFailedEvent;
import com.ecommerce.order_service.model.OrderStatus;
import com.ecommerce.order_service.repository.OrderRepository;
import com.ecommerce.order_service.service.NotificationService;
import com.ecommerce.order_service.service.OrderService;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.apache.kafka.common.header.Header;
import org.slf4j.MDC;
import org.springframework.kafka.annotation.KafkaListener;
import org.springframework.stereotype.Component;

import java.nio.charset.StandardCharsets;
import java.util.UUID;

@Slf4j
@Component
@RequiredArgsConstructor
public class PaymentEventConsumer {

    private final OrderService orderService;
    private final OrderRepository orderRepository;
    private final NotificationService notificationService;

    @KafkaListener(topics = "payments.completed", groupId = "order-service")
    public void onPaymentCompleted(ConsumerRecord<String, PaymentCompletedEvent> record) {
        MDC.put("correlationId", extractCorrelationId(record));
        try {
            PaymentCompletedEvent event = record.value();
            log.info("Received payment.completed orderId={}", event.getOrderId());
            orderService.updateOrderStatus(
                    event.getOrderId(),
                    OrderStatus.CONFIRMED,
                    "Payment completed (paymentId=" + event.getPaymentId() + ")",
                    "payment-service"
            );
            orderRepository.findById(event.getOrderId()).ifPresent(order -> {
                if (order.getSellerId() != null) {
                    try {
                        notificationService.notifySeller(order.getSellerId(), event.getOrderId(),
                                "Payment confirmed — ready to ship",
                                "Order #" + event.getOrderId().toString().substring(0, 8).toUpperCase()
                                        + " has been paid and is ready to ship.");
                    } catch (Exception ex) {
                        log.warn("Failed to notify seller of payment confirmation", ex);
                    }
                }
            });
        } catch (Exception e) {
            log.error("Failed to confirm order after payment completion", e);
        } finally {
            MDC.clear();
        }
    }

    @KafkaListener(topics = "payments.failed", groupId = "order-service")
    public void onPaymentFailed(ConsumerRecord<String, PaymentFailedEvent> record) {
        MDC.put("correlationId", extractCorrelationId(record));
        try {
            PaymentFailedEvent event = record.value();
            log.info("Received payment.failed orderId={}", event.getOrderId());
            orderService.updateOrderStatus(
                    event.getOrderId(),
                    OrderStatus.CANCELLED,
                    "Payment failed: " + event.getReason(),
                    "payment-service"
            );
            orderService.releaseStockForOrder(event.getOrderId());
        } catch (Exception e) {
            log.error("Failed to cancel order after payment failure", e);
        } finally {
            MDC.clear();
        }
    }

    private String extractCorrelationId(ConsumerRecord<?, ?> record) {
        Header h = record.headers().lastHeader("X-Correlation-ID");
        if (h != null) {
            return new String(h.value(), StandardCharsets.UTF_8);
        }
        return UUID.randomUUID().toString();
    }
}

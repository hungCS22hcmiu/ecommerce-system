package com.ecommerce.order_service.kafka;

import com.ecommerce.order_service.kafka.event.PaymentCompletedEvent;
import com.ecommerce.order_service.kafka.event.PaymentFailedEvent;
import com.ecommerce.order_service.model.OrderStatus;
import com.ecommerce.order_service.service.OrderService;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.kafka.annotation.KafkaListener;
import org.springframework.stereotype.Component;

@Slf4j
@Component
@RequiredArgsConstructor
public class PaymentEventConsumer {

    private final OrderService orderService;

    @KafkaListener(topics = "payments.completed", groupId = "order-service")
    public void onPaymentCompleted(PaymentCompletedEvent event) {
        log.info("Received payment.completed for orderId={}", event.getOrderId());
        try {
            orderService.updateOrderStatus(
                    event.getOrderId(),
                    OrderStatus.CONFIRMED,
                    "Payment completed (paymentId=" + event.getPaymentId() + ")",
                    "payment-service"
            );
        } catch (Exception e) {
            log.error("Failed to confirm order {} after payment completion", event.getOrderId(), e);
        }
    }

    @KafkaListener(topics = "payments.failed", groupId = "order-service")
    public void onPaymentFailed(PaymentFailedEvent event) {
        log.info("Received payment.failed for orderId={}", event.getOrderId());
        try {
            orderService.updateOrderStatus(
                    event.getOrderId(),
                    OrderStatus.CANCELLED,
                    "Payment failed: " + event.getReason(),
                    "payment-service"
            );
        } catch (Exception e) {
            log.error("Failed to cancel order {} after payment failure", event.getOrderId(), e);
        }
    }
}

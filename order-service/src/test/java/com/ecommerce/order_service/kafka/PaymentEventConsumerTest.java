package com.ecommerce.order_service.kafka;

import com.ecommerce.order_service.exception.InvalidOrderStateException;
import com.ecommerce.order_service.exception.OrderNotFoundException;
import com.ecommerce.order_service.kafka.event.PaymentCompletedEvent;
import com.ecommerce.order_service.kafka.event.PaymentFailedEvent;
import com.ecommerce.order_service.model.OrderStatus;
import com.ecommerce.order_service.service.OrderService;
import org.junit.jupiter.api.Nested;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.math.BigDecimal;
import java.util.UUID;

import static org.assertj.core.api.Assertions.assertThatNoException;
import static org.mockito.ArgumentMatchers.*;
import static org.mockito.Mockito.*;

@ExtendWith(MockitoExtension.class)
class PaymentEventConsumerTest {

    @Mock  private OrderService orderService;
    @InjectMocks private PaymentEventConsumer consumer;

    private final UUID orderId   = UUID.randomUUID();
    private final UUID paymentId = UUID.randomUUID();

    // ── OnPaymentCompleted ────────────────────────────────────────────────────

    @Nested
    class OnPaymentCompleted {

        @Test
        void validEvent_updatesOrderToConfirmed() {
            PaymentCompletedEvent event = new PaymentCompletedEvent(orderId, paymentId, BigDecimal.TEN);

            consumer.onPaymentCompleted(event);

            verify(orderService).updateOrderStatus(
                    eq(orderId),
                    eq(OrderStatus.CONFIRMED),
                    contains(paymentId.toString()),
                    eq("payment-service")
            );
        }

        @Test
        void orderNotFound_exceptionSwallowed_listenerDoesNotRethrow() {
            PaymentCompletedEvent event = new PaymentCompletedEvent(orderId, paymentId, BigDecimal.TEN);
            doThrow(new OrderNotFoundException(orderId))
                    .when(orderService).updateOrderStatus(any(), any(), any(), any());

            // Must not propagate — swallowed to avoid Kafka retry storm
            assertThatNoException().isThrownBy(() -> consumer.onPaymentCompleted(event));
        }

        @Test
        void invalidStateTransition_exceptionSwallowed() {
            PaymentCompletedEvent event = new PaymentCompletedEvent(orderId, paymentId, BigDecimal.TEN);
            doThrow(new InvalidOrderStateException("Already cancelled"))
                    .when(orderService).updateOrderStatus(any(), any(), any(), any());

            assertThatNoException().isThrownBy(() -> consumer.onPaymentCompleted(event));
        }
    }

    // ── OnPaymentFailed ───────────────────────────────────────────────────────

    @Nested
    class OnPaymentFailed {

        @Test
        void validEvent_updatesOrderToCancelled() {
            PaymentFailedEvent event = new PaymentFailedEvent(orderId, "Insufficient funds");

            consumer.onPaymentFailed(event);

            verify(orderService).updateOrderStatus(
                    eq(orderId),
                    eq(OrderStatus.CANCELLED),
                    contains("Insufficient funds"),
                    eq("payment-service")
            );
        }

        @Test
        void orderNotFound_exceptionSwallowed() {
            PaymentFailedEvent event = new PaymentFailedEvent(orderId, "Card declined");
            doThrow(new OrderNotFoundException(orderId))
                    .when(orderService).updateOrderStatus(any(), any(), any(), any());

            assertThatNoException().isThrownBy(() -> consumer.onPaymentFailed(event));
        }

        @Test
        void invalidStateTransition_exceptionSwallowed() {
            PaymentFailedEvent event = new PaymentFailedEvent(orderId, "Timeout");
            doThrow(new InvalidOrderStateException("Already delivered"))
                    .when(orderService).updateOrderStatus(any(), any(), any(), any());

            assertThatNoException().isThrownBy(() -> consumer.onPaymentFailed(event));
        }
    }
}

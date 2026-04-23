package com.ecommerce.order_service.service;

import com.ecommerce.order_service.exception.InvalidOrderStateException;
import com.ecommerce.order_service.model.OrderStatus;
import org.junit.jupiter.api.Nested;
import org.junit.jupiter.api.Test;

import static com.ecommerce.order_service.model.OrderStatus.*;
import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatNoException;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class OrderStateMachineTest {

    private final OrderStateMachine stateMachine = new OrderStateMachine();

    // ── FromPending ───────────────────────────────────────────────────────────

    @Nested
    class FromPending {

        @Test void toConfirmed_succeeds() {
            assertThatNoException().isThrownBy(() -> stateMachine.validateTransition(PENDING, CONFIRMED));
        }

        @Test void toCancelled_succeeds() {
            assertThatNoException().isThrownBy(() -> stateMachine.validateTransition(PENDING, CANCELLED));
        }

        @Test void toShipped_throws() {
            assertThatThrownBy(() -> stateMachine.validateTransition(PENDING, SHIPPED))
                    .isInstanceOf(InvalidOrderStateException.class)
                    .hasMessageContaining("PENDING")
                    .hasMessageContaining("SHIPPED");
        }

        @Test void toDelivered_throws() {
            assertThatThrownBy(() -> stateMachine.validateTransition(PENDING, DELIVERED))
                    .isInstanceOf(InvalidOrderStateException.class)
                    .hasMessageContaining("PENDING");
        }

        @Test void toPending_throws() {
            assertThatThrownBy(() -> stateMachine.validateTransition(PENDING, PENDING))
                    .isInstanceOf(InvalidOrderStateException.class);
        }

        @Test void canTransition_toConfirmed_returnsTrue() {
            assertThat(stateMachine.canTransition(PENDING, CONFIRMED)).isTrue();
        }

        @Test void canTransition_toCancelled_returnsTrue() {
            assertThat(stateMachine.canTransition(PENDING, CANCELLED)).isTrue();
        }

        @Test void canTransition_toShipped_returnsFalse() {
            assertThat(stateMachine.canTransition(PENDING, SHIPPED)).isFalse();
        }

        @Test void canTransition_toDelivered_returnsFalse() {
            assertThat(stateMachine.canTransition(PENDING, DELIVERED)).isFalse();
        }

        @Test void canTransition_toPending_returnsFalse() {
            assertThat(stateMachine.canTransition(PENDING, PENDING)).isFalse();
        }
    }

    // ── FromConfirmed ─────────────────────────────────────────────────────────

    @Nested
    class FromConfirmed {

        @Test void toShipped_succeeds() {
            assertThatNoException().isThrownBy(() -> stateMachine.validateTransition(CONFIRMED, SHIPPED));
        }

        @Test void toCancelled_succeeds() {
            assertThatNoException().isThrownBy(() -> stateMachine.validateTransition(CONFIRMED, CANCELLED));
        }

        @Test void toPending_throws() {
            assertThatThrownBy(() -> stateMachine.validateTransition(CONFIRMED, PENDING))
                    .isInstanceOf(InvalidOrderStateException.class)
                    .hasMessageContaining("CONFIRMED")
                    .hasMessageContaining("PENDING");
        }

        @Test void toConfirmed_throws() {
            assertThatThrownBy(() -> stateMachine.validateTransition(CONFIRMED, CONFIRMED))
                    .isInstanceOf(InvalidOrderStateException.class);
        }

        @Test void toDelivered_throws() {
            assertThatThrownBy(() -> stateMachine.validateTransition(CONFIRMED, DELIVERED))
                    .isInstanceOf(InvalidOrderStateException.class)
                    .hasMessageContaining("CONFIRMED")
                    .hasMessageContaining("DELIVERED");
        }

        @Test void canTransition_toShipped_returnsTrue() {
            assertThat(stateMachine.canTransition(CONFIRMED, SHIPPED)).isTrue();
        }

        @Test void canTransition_toCancelled_returnsTrue() {
            assertThat(stateMachine.canTransition(CONFIRMED, CANCELLED)).isTrue();
        }

        @Test void canTransition_toDelivered_returnsFalse() {
            assertThat(stateMachine.canTransition(CONFIRMED, DELIVERED)).isFalse();
        }
    }

    // ── FromShipped ───────────────────────────────────────────────────────────

    @Nested
    class FromShipped {

        @Test void toDelivered_succeeds() {
            assertThatNoException().isThrownBy(() -> stateMachine.validateTransition(SHIPPED, DELIVERED));
        }

        @Test void toPending_throws() {
            assertThatThrownBy(() -> stateMachine.validateTransition(SHIPPED, PENDING))
                    .isInstanceOf(InvalidOrderStateException.class)
                    .hasMessageContaining("SHIPPED");
        }

        @Test void toConfirmed_throws() {
            assertThatThrownBy(() -> stateMachine.validateTransition(SHIPPED, CONFIRMED))
                    .isInstanceOf(InvalidOrderStateException.class);
        }

        @Test void toCancelled_throws() {
            assertThatThrownBy(() -> stateMachine.validateTransition(SHIPPED, CANCELLED))
                    .isInstanceOf(InvalidOrderStateException.class)
                    .hasMessageContaining("SHIPPED")
                    .hasMessageContaining("CANCELLED");
        }

        @Test void toShipped_throws() {
            assertThatThrownBy(() -> stateMachine.validateTransition(SHIPPED, SHIPPED))
                    .isInstanceOf(InvalidOrderStateException.class);
        }

        @Test void canTransition_toDelivered_returnsTrue() {
            assertThat(stateMachine.canTransition(SHIPPED, DELIVERED)).isTrue();
        }

        @Test void canTransition_toCancelled_returnsFalse() {
            assertThat(stateMachine.canTransition(SHIPPED, CANCELLED)).isFalse();
        }
    }

    // ── FromDelivered (terminal) ──────────────────────────────────────────────

    @Nested
    class FromDelivered {

        @Test void toPending_throws() {
            assertThatThrownBy(() -> stateMachine.validateTransition(DELIVERED, PENDING))
                    .isInstanceOf(InvalidOrderStateException.class)
                    .hasMessageContaining("DELIVERED");
        }

        @Test void toConfirmed_throws() {
            assertThatThrownBy(() -> stateMachine.validateTransition(DELIVERED, CONFIRMED))
                    .isInstanceOf(InvalidOrderStateException.class);
        }

        @Test void toCancelled_throws() {
            assertThatThrownBy(() -> stateMachine.validateTransition(DELIVERED, CANCELLED))
                    .isInstanceOf(InvalidOrderStateException.class);
        }

        @Test void toShipped_throws() {
            assertThatThrownBy(() -> stateMachine.validateTransition(DELIVERED, SHIPPED))
                    .isInstanceOf(InvalidOrderStateException.class);
        }

        @Test void toDelivered_throws() {
            assertThatThrownBy(() -> stateMachine.validateTransition(DELIVERED, DELIVERED))
                    .isInstanceOf(InvalidOrderStateException.class)
                    .hasMessageContaining("DELIVERED");
        }

        @Test void canTransition_anyStatus_returnsFalse() {
            for (OrderStatus to : OrderStatus.values()) {
                assertThat(stateMachine.canTransition(DELIVERED, to)).isFalse();
            }
        }
    }

    // ── FromCancelled (terminal) ──────────────────────────────────────────────

    @Nested
    class FromCancelled {

        @Test void toPending_throws() {
            assertThatThrownBy(() -> stateMachine.validateTransition(CANCELLED, PENDING))
                    .isInstanceOf(InvalidOrderStateException.class)
                    .hasMessageContaining("CANCELLED");
        }

        @Test void toConfirmed_throws() {
            assertThatThrownBy(() -> stateMachine.validateTransition(CANCELLED, CONFIRMED))
                    .isInstanceOf(InvalidOrderStateException.class);
        }

        @Test void toCancelled_throws() {
            assertThatThrownBy(() -> stateMachine.validateTransition(CANCELLED, CANCELLED))
                    .isInstanceOf(InvalidOrderStateException.class);
        }

        @Test void toShipped_throws() {
            assertThatThrownBy(() -> stateMachine.validateTransition(CANCELLED, SHIPPED))
                    .isInstanceOf(InvalidOrderStateException.class);
        }

        @Test void toDelivered_throws() {
            assertThatThrownBy(() -> stateMachine.validateTransition(CANCELLED, DELIVERED))
                    .isInstanceOf(InvalidOrderStateException.class)
                    .hasMessageContaining("CANCELLED");
        }

        @Test void canTransition_anyStatus_returnsFalse() {
            for (OrderStatus to : OrderStatus.values()) {
                assertThat(stateMachine.canTransition(CANCELLED, to)).isFalse();
            }
        }
    }
}

package com.ecommerce.order_service.service;

import com.ecommerce.order_service.exception.InvalidOrderStateException;
import com.ecommerce.order_service.model.OrderStatus;
import org.springframework.stereotype.Component;

import java.util.Map;
import java.util.Set;

@Component
public class OrderStateMachine {

    private static final Map<OrderStatus, Set<OrderStatus>> VALID_TRANSITIONS = Map.of(
            OrderStatus.PENDING,   Set.of(OrderStatus.CONFIRMED, OrderStatus.CANCELLED),
            OrderStatus.CONFIRMED, Set.of(OrderStatus.SHIPPED,   OrderStatus.CANCELLED),
            OrderStatus.SHIPPED,   Set.of(OrderStatus.DELIVERED)
            // DELIVERED and CANCELLED are terminal states — no valid transitions out
    );

    public void validateTransition(OrderStatus from, OrderStatus to) {
        Set<OrderStatus> allowed = VALID_TRANSITIONS.getOrDefault(from, Set.of());
        if (!allowed.contains(to)) {
            throw new InvalidOrderStateException(
                    "Cannot transition order from " + from + " to " + to +
                    ". Allowed transitions: " + allowed);
        }
    }

    public boolean canTransition(OrderStatus from, OrderStatus to) {
        return VALID_TRANSITIONS.getOrDefault(from, Set.of()).contains(to);
    }
}

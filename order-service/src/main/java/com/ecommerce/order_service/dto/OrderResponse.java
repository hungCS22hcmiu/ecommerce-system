package com.ecommerce.order_service.dto;

import com.ecommerce.order_service.model.Order;
import com.ecommerce.order_service.model.OrderStatus;
import com.ecommerce.order_service.model.ShippingAddress;
import lombok.Builder;
import lombok.Getter;

import java.math.BigDecimal;
import java.time.OffsetDateTime;
import java.util.List;
import java.util.UUID;
import java.util.stream.Collectors;

@Getter
@Builder
public class OrderResponse {

    private UUID id;
    private UUID userId;
    private UUID cartId;
    private BigDecimal totalAmount;
    private OrderStatus status;
    private ShippingAddress shippingAddress;
    private List<OrderItemResponse> items;
    private OffsetDateTime createdAt;
    private OffsetDateTime updatedAt;

    public static OrderResponse from(Order order) {
        return OrderResponse.builder()
                .id(order.getId())
                .userId(order.getUserId())
                .cartId(order.getCartId())
                .totalAmount(order.getTotalAmount())
                .status(order.getStatus())
                .shippingAddress(order.getShippingAddress())
                .items(order.getItems().stream()
                        .map(OrderItemResponse::from)
                        .collect(Collectors.toList()))
                .createdAt(order.getCreatedAt())
                .updatedAt(order.getUpdatedAt())
                .build();
    }
}

package com.ecommerce.order_service.dto;

import com.ecommerce.order_service.model.Order;
import com.ecommerce.order_service.model.OrderStatus;
import lombok.Builder;
import lombok.Getter;

import java.math.BigDecimal;
import java.time.OffsetDateTime;
import java.util.UUID;

@Getter
@Builder
public class OrderSummaryResponse {

    private UUID id;
    private BigDecimal totalAmount;
    private OrderStatus status;
    private int itemCount;
    private OffsetDateTime createdAt;

    public static OrderSummaryResponse from(Order order) {
        return OrderSummaryResponse.builder()
                .id(order.getId())
                .totalAmount(order.getTotalAmount())
                .status(order.getStatus())
                .itemCount(order.getItems().size())
                .createdAt(order.getCreatedAt())
                .build();
    }
}

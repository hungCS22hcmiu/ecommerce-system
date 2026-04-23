package com.ecommerce.order_service.dto;

import com.ecommerce.order_service.model.OrderStatus;
import com.ecommerce.order_service.model.OrderStatusHistory;
import lombok.Builder;
import lombok.Getter;

import java.time.OffsetDateTime;

@Getter
@Builder
public class OrderStatusHistoryResponse {

    private Long id;
    private OrderStatus oldStatus;
    private OrderStatus newStatus;
    private String reason;
    private String changedBy;
    private OffsetDateTime changedAt;

    public static OrderStatusHistoryResponse from(OrderStatusHistory history) {
        return OrderStatusHistoryResponse.builder()
                .id(history.getId())
                .oldStatus(history.getOldStatus())
                .newStatus(history.getNewStatus())
                .reason(history.getReason())
                .changedBy(history.getChangedBy())
                .changedAt(history.getChangedAt())
                .build();
    }
}

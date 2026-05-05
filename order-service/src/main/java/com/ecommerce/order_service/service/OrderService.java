package com.ecommerce.order_service.service;

import com.ecommerce.order_service.dto.*;
import com.ecommerce.order_service.model.OrderStatus;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;

import java.util.List;
import java.util.UUID;

public interface OrderService {

    OrderResponse createOrder(UUID userId, CreateOrderRequest request);

    OrderResponse getOrder(UUID orderId, UUID userId);

    Page<OrderSummaryResponse> listOrders(UUID userId, Pageable pageable);

    OrderResponse cancelOrder(UUID orderId, UUID userId);

    OrderResponse updateOrderStatus(UUID orderId, OrderStatus newStatus, String reason, String changedBy);

    List<OrderStatusHistoryResponse> getOrderHistory(UUID orderId, UUID userId);
}

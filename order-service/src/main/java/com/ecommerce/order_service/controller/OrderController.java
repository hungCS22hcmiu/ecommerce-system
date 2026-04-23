package com.ecommerce.order_service.controller;

import com.ecommerce.order_service.dto.*;
import com.ecommerce.order_service.model.OrderStatus;
import com.ecommerce.order_service.service.OrderService;
import jakarta.validation.Valid;
import lombok.RequiredArgsConstructor;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;
import org.springframework.data.web.PageableDefault;
import org.springframework.http.HttpStatus;
import org.springframework.web.bind.annotation.*;

import java.util.List;
import java.util.UUID;

@RestController
@RequestMapping("/api/v1/orders")
@RequiredArgsConstructor
public class OrderController {

    private final OrderService orderService;

    @PostMapping
    @ResponseStatus(HttpStatus.CREATED)
    public ApiResponse<OrderResponse> createOrder(
            @RequestHeader("X-User-Id") UUID userId,
            @Valid @RequestBody CreateOrderRequest request) {
        return ApiResponse.ok(orderService.createOrder(userId, request));
    }

    @GetMapping("/{id}")
    public ApiResponse<OrderResponse> getOrder(
            @RequestHeader("X-User-Id") UUID userId,
            @PathVariable UUID id) {
        return ApiResponse.ok(orderService.getOrder(id, userId));
    }

    @GetMapping
    public ApiResponse<List<OrderSummaryResponse>> listOrders(
            @RequestHeader("X-User-Id") UUID userId,
            @PageableDefault(size = 20, sort = "createdAt") Pageable pageable) {
        Page<OrderSummaryResponse> page = orderService.listOrders(userId, pageable);
        return ApiResponse.ok(page);
    }

    @PutMapping("/{id}/cancel")
    public ApiResponse<OrderResponse> cancelOrder(
            @RequestHeader("X-User-Id") UUID userId,
            @PathVariable UUID id) {
        return ApiResponse.ok(orderService.cancelOrder(id, userId));
    }

    @PutMapping("/{id}/ship")
    public ApiResponse<OrderResponse> shipOrder(
            @RequestHeader("X-User-Id") UUID actorId,
            @PathVariable UUID id) {
        return ApiResponse.ok(orderService.updateOrderStatus(
                id, OrderStatus.SHIPPED, "Shipped by seller", actorId.toString()));
    }

    @PutMapping("/{id}/deliver")
    public ApiResponse<OrderResponse> deliverOrder(
            @RequestHeader("X-User-Id") UUID actorId,
            @PathVariable UUID id) {
        return ApiResponse.ok(orderService.updateOrderStatus(
                id, OrderStatus.DELIVERED, "Delivered", actorId.toString()));
    }

    @GetMapping("/{id}/history")
    public ApiResponse<List<OrderStatusHistoryResponse>> getOrderHistory(
            @RequestHeader("X-User-Id") UUID userId,
            @PathVariable UUID id) {
        return ApiResponse.ok(orderService.getOrderHistory(id, userId));
    }
}

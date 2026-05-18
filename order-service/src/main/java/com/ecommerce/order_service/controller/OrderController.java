package com.ecommerce.order_service.controller;

import com.ecommerce.order_service.dto.*;
import com.ecommerce.order_service.model.OrderStatus;
import com.ecommerce.order_service.service.NotificationService;
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
    private final NotificationService notificationService;

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
            @RequestHeader("X-User-Id") UUID sellerId,
            @PathVariable UUID id) {
        return ApiResponse.ok(orderService.shipOrder(id, sellerId));
    }

    @GetMapping("/seller")
    public ApiResponse<List<OrderSummaryResponse>> listSellerOrders(
            @RequestHeader("X-User-Id") UUID sellerId,
            @RequestParam(required = false) OrderStatus status,
            @PageableDefault(size = 20, sort = "createdAt") Pageable pageable) {
        Page<OrderSummaryResponse> page = orderService.listSellerOrders(sellerId, status, pageable);
        return ApiResponse.ok(page);
    }

    @GetMapping("/seller/{id}")
    public ApiResponse<OrderResponse> getSellerOrder(
            @RequestHeader("X-User-Id") UUID sellerId,
            @PathVariable UUID id) {
        return ApiResponse.ok(orderService.getOrderAsSeller(id, sellerId));
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

    @GetMapping("/purchase-verification")
    public ApiResponse<PurchaseVerificationResponse> verifyPurchase(
            @RequestHeader("X-User-Id") UUID userId,
            @RequestParam Long productId,
            @RequestParam UUID orderItemId) {
        return ApiResponse.ok(new PurchaseVerificationResponse(
                orderService.verifyPurchase(userId, productId, orderItemId)));
    }

    @GetMapping("/notifications")
    public ApiResponse<NotificationSummaryResponse> getNotifications(
            @RequestHeader("X-User-Id") UUID userId) {
        return ApiResponse.ok(notificationService.getSummary(userId));
    }

    @PutMapping("/notifications/mark-read")
    public ApiResponse<Void> markNotificationsRead(
            @RequestHeader("X-User-Id") UUID userId) {
        notificationService.markAllRead(userId);
        return ApiResponse.ok((Void) null);
    }

    @PostMapping("/notifications/internal/review")
    public ApiResponse<Void> createReviewNotification(
            @RequestBody ReviewNotificationRequest req) {
        notificationService.notifySellerReview(req.sellerId(), req.productId(), req.title(), req.body());
        return ApiResponse.ok((Void) null);
    }
}

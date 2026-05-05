package com.ecommerce.order_service.controller;

import com.ecommerce.order_service.dto.*;
import com.ecommerce.order_service.exception.*;
import com.ecommerce.order_service.model.OrderStatus;
import com.ecommerce.order_service.model.ShippingAddress;
import com.ecommerce.order_service.service.OrderService;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Nested;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.PageImpl;
import org.springframework.data.domain.Pageable;
import org.springframework.http.MediaType;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

import java.math.BigDecimal;
import java.time.OffsetDateTime;
import java.util.List;
import java.util.UUID;

import static org.mockito.ArgumentMatchers.*;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.*;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.*;

@WebMvcTest(OrderController.class)
class OrderControllerTest {

    @Autowired private MockMvc mockMvc;
    @Autowired private ObjectMapper objectMapper;
    @MockitoBean private OrderService orderService;

    private UUID userId;
    private UUID orderId;

    @BeforeEach
    void setUp() {
        userId  = UUID.randomUUID();
        orderId = UUID.randomUUID();
    }

    // ── Helpers ───────────────────────────────────────────────────────────────

    private OrderResponse buildOrderResponse() {
        return OrderResponse.builder()
                .id(orderId)
                .userId(userId)
                .cartId(UUID.randomUUID())
                .totalAmount(BigDecimal.ZERO)
                .status(OrderStatus.PENDING)
                .shippingAddress(new ShippingAddress("1 Main St", "HCMC", "HCM", "VN", "70000"))
                .items(List.of())
                .createdAt(OffsetDateTime.now())
                .updatedAt(OffsetDateTime.now())
                .build();
    }

    private CreateOrderRequest buildValidRequest() {
        return new CreateOrderRequest(
                UUID.randomUUID(),
                List.of(new OrderItemRequest(1L, 2)),
                new ShippingAddressDto("1 Main St", "HCMC", "HCM", "VN", "70000")
        );
    }

    // ── POST /api/v1/orders ───────────────────────────────────────────────────

    @Nested
    class CreateOrder {

        @Test
        void validRequest_returns201WithBody() throws Exception {
            when(orderService.createOrder(eq(userId), any())).thenReturn(buildOrderResponse());

            mockMvc.perform(post("/api/v1/orders")
                            .header("X-User-Id", userId.toString())
                            .contentType(MediaType.APPLICATION_JSON)
                            .content(objectMapper.writeValueAsString(buildValidRequest())))
                    .andExpect(status().isCreated())
                    .andExpect(jsonPath("$.success").value(true))
                    .andExpect(jsonPath("$.data.id").value(orderId.toString()))
                    .andExpect(jsonPath("$.data.status").value("PENDING"));
        }

        @Test
        void missingUserIdHeader_returns400() throws Exception {
            mockMvc.perform(post("/api/v1/orders")
                            .contentType(MediaType.APPLICATION_JSON)
                            .content(objectMapper.writeValueAsString(buildValidRequest())))
                    .andExpect(status().isBadRequest())
                    .andExpect(jsonPath("$.success").value(false))
                    .andExpect(jsonPath("$.error.code").value("BAD_REQUEST"));
        }

        @Test
        void emptyItemsList_returns400WithValidationError() throws Exception {
            CreateOrderRequest invalid = new CreateOrderRequest(
                    UUID.randomUUID(), List.of(),
                    new ShippingAddressDto("1 Main", "HCMC", "HCM", "VN", "70000"));

            mockMvc.perform(post("/api/v1/orders")
                            .header("X-User-Id", userId.toString())
                            .contentType(MediaType.APPLICATION_JSON)
                            .content(objectMapper.writeValueAsString(invalid)))
                    .andExpect(status().isBadRequest())
                    .andExpect(jsonPath("$.success").value(false))
                    .andExpect(jsonPath("$.error.code").value("VALIDATION_ERROR"));
        }

        @Test
        void insufficientStock_returns409() throws Exception {
            when(orderService.createOrder(any(), any()))
                    .thenThrow(new InsufficientStockException(1L));

            mockMvc.perform(post("/api/v1/orders")
                            .header("X-User-Id", userId.toString())
                            .contentType(MediaType.APPLICATION_JSON)
                            .content(objectMapper.writeValueAsString(buildValidRequest())))
                    .andExpect(status().isConflict())
                    .andExpect(jsonPath("$.error.code").value("INSUFFICIENT_STOCK"));
        }
    }

    // ── GET /api/v1/orders/{id} ───────────────────────────────────────────────

    @Nested
    class GetOrder {

        @Test
        void existingOwnOrder_returns200() throws Exception {
            when(orderService.getOrder(eq(orderId), eq(userId))).thenReturn(buildOrderResponse());

            mockMvc.perform(get("/api/v1/orders/{id}", orderId)
                            .header("X-User-Id", userId.toString()))
                    .andExpect(status().isOk())
                    .andExpect(jsonPath("$.success").value(true))
                    .andExpect(jsonPath("$.data.id").value(orderId.toString()));
        }

        @Test
        void orderNotFound_returns404() throws Exception {
            when(orderService.getOrder(any(), any()))
                    .thenThrow(new OrderNotFoundException(orderId));

            mockMvc.perform(get("/api/v1/orders/{id}", orderId)
                            .header("X-User-Id", userId.toString()))
                    .andExpect(status().isNotFound())
                    .andExpect(jsonPath("$.success").value(false))
                    .andExpect(jsonPath("$.error.code").value("ORDER_NOT_FOUND"));
        }

        @Test
        void accessDenied_returns403() throws Exception {
            when(orderService.getOrder(any(), any()))
                    .thenThrow(new OrderAccessDeniedException(orderId));

            mockMvc.perform(get("/api/v1/orders/{id}", orderId)
                            .header("X-User-Id", userId.toString()))
                    .andExpect(status().isForbidden())
                    .andExpect(jsonPath("$.error.code").value("ACCESS_DENIED"));
        }
    }

    // ── GET /api/v1/orders ────────────────────────────────────────────────────

    @Nested
    class ListOrders {

        @Test
        void returns200WithPageMeta() throws Exception {
            OrderSummaryResponse summary = OrderSummaryResponse.builder()
                    .id(orderId).totalAmount(BigDecimal.ZERO)
                    .status(OrderStatus.PENDING).itemCount(1)
                    .createdAt(OffsetDateTime.now()).build();

            Page<OrderSummaryResponse> page = new PageImpl<>(List.of(summary));
            when(orderService.listOrders(eq(userId), any(Pageable.class))).thenReturn(page);

            mockMvc.perform(get("/api/v1/orders")
                            .header("X-User-Id", userId.toString()))
                    .andExpect(status().isOk())
                    .andExpect(jsonPath("$.success").value(true))
                    .andExpect(jsonPath("$.data").isArray())
                    .andExpect(jsonPath("$.meta.totalElements").value(1));
        }
    }

    // ── PUT /api/v1/orders/{id}/cancel ────────────────────────────────────────

    @Nested
    class CancelOrder {

        @Test
        void validCancel_returns200() throws Exception {
            OrderResponse cancelled = buildOrderResponse();
            when(orderService.cancelOrder(eq(orderId), eq(userId))).thenReturn(cancelled);

            mockMvc.perform(put("/api/v1/orders/{id}/cancel", orderId)
                            .header("X-User-Id", userId.toString()))
                    .andExpect(status().isOk())
                    .andExpect(jsonPath("$.success").value(true));
        }

        @Test
        void invalidStateTransition_returns409() throws Exception {
            when(orderService.cancelOrder(any(), any()))
                    .thenThrow(new InvalidOrderStateException("Cannot cancel a DELIVERED order"));

            mockMvc.perform(put("/api/v1/orders/{id}/cancel", orderId)
                            .header("X-User-Id", userId.toString()))
                    .andExpect(status().isConflict())
                    .andExpect(jsonPath("$.error.code").value("INVALID_STATE_TRANSITION"));
        }
    }

    // ── PUT /api/v1/orders/{id}/ship ──────────────────────────────────────────

    @Nested
    class ShipOrder {

        @Test
        void validShip_returns200() throws Exception {
            when(orderService.updateOrderStatus(eq(orderId), eq(OrderStatus.SHIPPED), any(), any()))
                    .thenReturn(buildOrderResponse());

            mockMvc.perform(put("/api/v1/orders/{id}/ship", orderId)
                            .header("X-User-Id", userId.toString()))
                    .andExpect(status().isOk())
                    .andExpect(jsonPath("$.success").value(true));
        }

        @Test
        void invalidTransition_returns409() throws Exception {
            when(orderService.updateOrderStatus(any(), eq(OrderStatus.SHIPPED), any(), any()))
                    .thenThrow(new InvalidOrderStateException("Cannot ship from PENDING"));

            mockMvc.perform(put("/api/v1/orders/{id}/ship", orderId)
                            .header("X-User-Id", userId.toString()))
                    .andExpect(status().isConflict())
                    .andExpect(jsonPath("$.error.code").value("INVALID_STATE_TRANSITION"));
        }
    }

    // ── PUT /api/v1/orders/{id}/deliver ───────────────────────────────────────

    @Nested
    class DeliverOrder {

        @Test
        void validDeliver_returns200() throws Exception {
            when(orderService.updateOrderStatus(eq(orderId), eq(OrderStatus.DELIVERED), any(), any()))
                    .thenReturn(buildOrderResponse());

            mockMvc.perform(put("/api/v1/orders/{id}/deliver", orderId)
                            .header("X-User-Id", userId.toString()))
                    .andExpect(status().isOk())
                    .andExpect(jsonPath("$.success").value(true));
        }
    }

    // ── GET /api/v1/orders/{id}/history ──────────────────────────────────────

    @Nested
    class GetOrderHistory {

        @Test
        void returns200WithHistoryList() throws Exception {
            List<OrderStatusHistoryResponse> history = List.of(
                    OrderStatusHistoryResponse.builder()
                            .id(1L).oldStatus(null).newStatus(OrderStatus.PENDING)
                            .reason("Order created").changedBy(userId.toString())
                            .changedAt(OffsetDateTime.now()).build()
            );
            when(orderService.getOrderHistory(eq(orderId), eq(userId))).thenReturn(history);

            mockMvc.perform(get("/api/v1/orders/{id}/history", orderId)
                            .header("X-User-Id", userId.toString()))
                    .andExpect(status().isOk())
                    .andExpect(jsonPath("$.success").value(true))
                    .andExpect(jsonPath("$.data").isArray())
                    .andExpect(jsonPath("$.data[0].newStatus").value("PENDING"));
        }

        @Test
        void orderNotFound_returns404() throws Exception {
            when(orderService.getOrderHistory(any(), any()))
                    .thenThrow(new OrderNotFoundException(orderId));

            mockMvc.perform(get("/api/v1/orders/{id}/history", orderId)
                            .header("X-User-Id", userId.toString()))
                    .andExpect(status().isNotFound())
                    .andExpect(jsonPath("$.error.code").value("ORDER_NOT_FOUND"));
        }
    }
}

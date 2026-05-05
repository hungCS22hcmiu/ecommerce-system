package com.ecommerce.order_service.service;

import com.ecommerce.order_service.client.ProductServiceClient;
import com.ecommerce.order_service.dto.*;
import com.ecommerce.order_service.exception.InvalidOrderStateException;
import com.ecommerce.order_service.exception.InsufficientStockException;
import com.ecommerce.order_service.exception.OrderAccessDeniedException;
import com.ecommerce.order_service.exception.OrderNotFoundException;
import com.ecommerce.order_service.kafka.OrderEventProducer;
import com.ecommerce.order_service.kafka.event.OrderCreatedEvent;
import com.ecommerce.order_service.model.*;
import com.ecommerce.order_service.repository.OrderRepository;
import com.ecommerce.order_service.repository.OrderStatusHistoryRepository;
import com.ecommerce.order_service.service.impl.OrderServiceImpl;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Nested;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.PageImpl;
import org.springframework.data.domain.PageRequest;
import org.springframework.data.domain.Pageable;

import java.math.BigDecimal;
import java.time.OffsetDateTime;
import java.util.ArrayList;
import java.util.List;
import java.util.Optional;
import java.util.UUID;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatNoException;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.*;

@ExtendWith(MockitoExtension.class)
class OrderServiceImplTest {

    @Mock private OrderRepository orderRepository;
    @Mock private OrderStatusHistoryRepository historyRepository;
    @Mock private OrderStateMachine stateMachine;
    @Mock private ProductServiceClient productServiceClient;
    @Mock private OrderEventProducer eventProducer;

    @InjectMocks
    private OrderServiceImpl orderService;

    private UUID userId;
    private UUID orderId;

    @BeforeEach
    void setUp() {
        userId  = UUID.randomUUID();
        orderId = UUID.randomUUID();
    }

    // ── Helpers ───────────────────────────────────────────────────────────────

    private Order buildOrder(OrderStatus status) {
        return Order.builder()
                .id(orderId)
                .userId(userId)
                .cartId(UUID.randomUUID())
                .status(status)
                .totalAmount(BigDecimal.ZERO)
                .shippingAddress(new ShippingAddress("1 Main St", "HCMC", "HCM", "VN", "70000"))
                .items(new ArrayList<>())
                .createdAt(OffsetDateTime.now())
                .updatedAt(OffsetDateTime.now())
                .build();
    }

    private Order buildOrderWithItems(OrderStatus status) {
        Order order = buildOrder(status);
        OrderItem item = OrderItem.builder()
                .id(UUID.randomUUID())
                .order(order)
                .productId(1L)
                .productName("Product 1")
                .quantity(2)
                .unitPrice(BigDecimal.ZERO)
                .build();
        order.getItems().add(item);
        return order;
    }

    private CreateOrderRequest buildCreateRequest(int itemCount) {
        List<OrderItemRequest> items = new ArrayList<>();
        for (int i = 1; i <= itemCount; i++) {
            items.add(new OrderItemRequest((long) i, 2));
        }
        ShippingAddressDto addr = new ShippingAddressDto("1 Main St", "HCMC", "HCM", "VN", "70000");
        return new CreateOrderRequest(UUID.randomUUID(), items, addr);
    }

    private ProductServiceClient.StockResponse stockOk(Long productId) {
        return new ProductServiceClient.StockResponse(productId, 10, 0);
    }

    private ProductServiceClient.ProductDetail productDetail(Long productId) {
        return new ProductServiceClient.ProductDetail("Product " + productId, new BigDecimal("10.00"));
    }

    private void givenOrderExists(Order order) {
        when(orderRepository.findById(orderId)).thenReturn(Optional.of(order));
    }

    private void givenOrderExistsWithLock(Order order) {
        when(orderRepository.findByIdWithLock(orderId)).thenReturn(Optional.of(order));
    }

    private void givenOrderNotFound() {
        when(orderRepository.findById(orderId)).thenReturn(Optional.empty());
    }

    // ── CreateOrder ───────────────────────────────────────────────────────────

    @Nested
    class CreateOrder {

        @Test
        void singleItem_happyPath_orderSavedAndEventPublished() {
            CreateOrderRequest request = buildCreateRequest(1);
            Order saved = buildOrderWithItems(OrderStatus.PENDING);

            when(productServiceClient.reserveStock(eq(1L), eq(2), any())).thenReturn(stockOk(1L));
            when(productServiceClient.getProduct(1L)).thenReturn(productDetail(1L));
            when(orderRepository.save(any())).thenReturn(saved);
            when(historyRepository.save(any())).thenReturn(null);

            OrderResponse response = orderService.createOrder(userId, request);

            assertThat(response.getId()).isEqualTo(saved.getId());
            assertThat(response.getStatus()).isEqualTo(OrderStatus.PENDING);
            assertThat(response.getUserId()).isEqualTo(saved.getUserId());

            verify(orderRepository).save(any(Order.class));
            verify(eventProducer).publishOrderCreated(any(OrderCreatedEvent.class));
        }

        @Test
        void multipleItems_totalComputedFromProductServicePrices() {
            CreateOrderRequest request = buildCreateRequest(2);  // 2 items, qty=2 each
            Order saved = buildOrder(OrderStatus.PENDING);

            when(productServiceClient.reserveStock(eq(1L), eq(2), any())).thenReturn(stockOk(1L));
            when(productServiceClient.reserveStock(eq(2L), eq(2), any())).thenReturn(stockOk(2L));
            when(productServiceClient.getProduct(1L)).thenReturn(new ProductServiceClient.ProductDetail("Widget", new BigDecimal("10.00")));
            when(productServiceClient.getProduct(2L)).thenReturn(new ProductServiceClient.ProductDetail("Gadget", new BigDecimal("5.00")));
            when(orderRepository.save(any())).thenReturn(saved);

            orderService.createOrder(userId, request);

            // Capture the order passed to save and verify total = (10*2) + (5*2) = 30
            ArgumentCaptor<Order> orderCaptor = ArgumentCaptor.forClass(Order.class);
            verify(orderRepository).save(orderCaptor.capture());
            assertThat(orderCaptor.getValue().getTotalAmount()).isEqualByComparingTo(new BigDecimal("30.00"));
        }

        @Test
        void stockReservationFails_singleItem_exceptionPropagated_noReleaseNeeded() {
            CreateOrderRequest request = buildCreateRequest(1);

            when(productServiceClient.reserveStock(eq(1L), eq(2), any()))
                    .thenThrow(new InsufficientStockException(1L));

            assertThatThrownBy(() -> orderService.createOrder(userId, request))
                    .isInstanceOf(InsufficientStockException.class);

            // Nothing was reserved, so releaseStock must never be called
            verify(productServiceClient, never()).releaseStock(any(), anyInt(), any());
            verify(orderRepository, never()).save(any());
            verify(eventProducer, never()).publishOrderCreated(any());
        }

        @Test
        void initialHistoryEntry_recordsNullToPendingTransition() {
            CreateOrderRequest request = buildCreateRequest(1);
            Order saved = buildOrderWithItems(OrderStatus.PENDING);

            when(productServiceClient.reserveStock(any(), anyInt(), any())).thenReturn(stockOk(1L));
            when(productServiceClient.getProduct(1L)).thenReturn(productDetail(1L));
            when(orderRepository.save(any())).thenReturn(saved);

            orderService.createOrder(userId, request);

            ArgumentCaptor<OrderStatusHistory> historyCaptor =
                    ArgumentCaptor.forClass(OrderStatusHistory.class);
            verify(historyRepository).save(historyCaptor.capture());

            OrderStatusHistory recorded = historyCaptor.getValue();
            assertThat(recorded.getOldStatus()).isNull();
            assertThat(recorded.getNewStatus()).isEqualTo(OrderStatus.PENDING);
            assertThat(recorded.getReason()).isEqualTo("Order created");
        }

        @Test
        void publishedEvent_containsCorrectOrderIdAndUserId() {
            CreateOrderRequest request = buildCreateRequest(1);
            Order saved = buildOrderWithItems(OrderStatus.PENDING);

            when(productServiceClient.reserveStock(any(), anyInt(), any())).thenReturn(stockOk(1L));
            when(productServiceClient.getProduct(1L)).thenReturn(productDetail(1L));
            when(orderRepository.save(any())).thenReturn(saved);

            orderService.createOrder(userId, request);

            ArgumentCaptor<OrderCreatedEvent> eventCaptor =
                    ArgumentCaptor.forClass(OrderCreatedEvent.class);
            verify(eventProducer).publishOrderCreated(eventCaptor.capture());

            OrderCreatedEvent event = eventCaptor.getValue();
            assertThat(event.getOrderId()).isEqualTo(saved.getId());
            assertThat(event.getUserId()).isEqualTo(saved.getUserId());
        }

        @Test
        void shippingAddress_mappedCorrectlyToEntity() {
            CreateOrderRequest request = buildCreateRequest(1);
            Order saved = buildOrderWithItems(OrderStatus.PENDING);

            when(productServiceClient.reserveStock(any(), anyInt(), any())).thenReturn(stockOk(1L));
            when(productServiceClient.getProduct(1L)).thenReturn(productDetail(1L));
            when(orderRepository.save(any())).thenReturn(saved);

            orderService.createOrder(userId, request);

            ArgumentCaptor<Order> orderCaptor = ArgumentCaptor.forClass(Order.class);
            verify(orderRepository).save(orderCaptor.capture());

            ShippingAddress addr = orderCaptor.getValue().getShippingAddress();
            ShippingAddressDto dto = request.getShippingAddress();
            assertThat(addr.getStreet()).isEqualTo(dto.getStreet());
            assertThat(addr.getCity()).isEqualTo(dto.getCity());
            assertThat(addr.getCountry()).isEqualTo(dto.getCountry());
        }
    }

    // ── GetOrder ──────────────────────────────────────────────────────────────

    @Nested
    class GetOrder {

        @Test
        void ownOrder_returnsResponse() {
            Order order = buildOrderWithItems(OrderStatus.PENDING);
            givenOrderExists(order);

            OrderResponse response = orderService.getOrder(orderId, userId);

            assertThat(response.getId()).isEqualTo(orderId);
            assertThat(response.getStatus()).isEqualTo(OrderStatus.PENDING);
        }

        @Test
        void orderNotFound_throwsOrderNotFoundException() {
            givenOrderNotFound();

            assertThatThrownBy(() -> orderService.getOrder(orderId, userId))
                    .isInstanceOf(OrderNotFoundException.class)
                    .hasMessageContaining(orderId.toString());
        }

        @Test
        void differentUser_throwsOrderAccessDeniedException() {
            Order order = buildOrderWithItems(OrderStatus.PENDING);
            givenOrderExists(order);

            UUID otherUser = UUID.randomUUID();

            assertThatThrownBy(() -> orderService.getOrder(orderId, otherUser))
                    .isInstanceOf(OrderAccessDeniedException.class)
                    .hasMessageContaining(orderId.toString());
        }
    }

    // ── ListOrders ────────────────────────────────────────────────────────────

    @Nested
    class ListOrders {

        @Test
        void returnsPagedSummaries() {
            Pageable pageable = PageRequest.of(0, 20);
            List<Order> orders = List.of(
                    buildOrderWithItems(OrderStatus.PENDING),
                    buildOrderWithItems(OrderStatus.CONFIRMED)
            );
            Page<Order> page = new PageImpl<>(orders, pageable, 2);

            when(orderRepository.findByUserIdOrderByCreatedAtDesc(userId, pageable)).thenReturn(page);

            Page<OrderSummaryResponse> result = orderService.listOrders(userId, pageable);

            assertThat(result.getTotalElements()).isEqualTo(2);
            assertThat(result.getContent()).hasSize(2);
            assertThat(result.getContent().get(0).getStatus()).isEqualTo(OrderStatus.PENDING);
            assertThat(result.getContent().get(1).getStatus()).isEqualTo(OrderStatus.CONFIRMED);
        }

        @Test
        void emptyPage_whenNoOrders() {
            Pageable pageable = PageRequest.of(0, 20);
            when(orderRepository.findByUserIdOrderByCreatedAtDesc(userId, pageable))
                    .thenReturn(Page.empty());

            Page<OrderSummaryResponse> result = orderService.listOrders(userId, pageable);

            assertThat(result.isEmpty()).isTrue();
        }
    }

    // ── CancelOrder ───────────────────────────────────────────────────────────

    @Nested
    class CancelOrder {

        @Test
        void ownPendingOrder_cancelledAndStockReleased() {
            Order order = buildOrderWithItems(OrderStatus.PENDING);
            givenOrderExists(order);
            givenOrderExistsWithLock(order);
            when(orderRepository.save(any())).thenReturn(order);

            orderService.cancelOrder(orderId, userId);

            // updateOrderStatus called via cancellation path
            verify(orderRepository).findByIdWithLock(orderId);
            verify(stateMachine).validateTransition(OrderStatus.PENDING, OrderStatus.CANCELLED);

            // Stock released for each item
            verify(productServiceClient).releaseStock(eq(1L), eq(2), eq(orderId.toString()));
        }

        @Test
        void orderNotFound_throwsOrderNotFoundException() {
            givenOrderNotFound();

            assertThatThrownBy(() -> orderService.cancelOrder(orderId, userId))
                    .isInstanceOf(OrderNotFoundException.class);
        }

        @Test
        void differentUser_throwsOrderAccessDeniedException() {
            Order order = buildOrderWithItems(OrderStatus.PENDING);
            givenOrderExists(order);

            UUID otherUser = UUID.randomUUID();

            assertThatThrownBy(() -> orderService.cancelOrder(orderId, otherUser))
                    .isInstanceOf(OrderAccessDeniedException.class);

            verify(orderRepository, never()).findByIdWithLock(any());
        }

        @Test
        void stockReleaseFails_cancellationStillSucceeds() {
            Order order = buildOrderWithItems(OrderStatus.PENDING);
            givenOrderExists(order);
            givenOrderExistsWithLock(order);
            when(orderRepository.save(any())).thenReturn(order);
            doThrow(new RuntimeException("product-service down"))
                    .when(productServiceClient).releaseStock(any(), anyInt(), any());

            // Should not throw — release failure is swallowed and logged
            assertThatNoException().isThrownBy(() -> orderService.cancelOrder(orderId, userId));
        }
    }

    // ── UpdateOrderStatus ─────────────────────────────────────────────────────

    @Nested
    class UpdateOrderStatus {

        @Test
        void validTransition_updatesStatusAndRecordsHistory() {
            Order order = buildOrder(OrderStatus.CONFIRMED);
            givenOrderExistsWithLock(order);
            when(orderRepository.save(any())).thenReturn(order);

            orderService.updateOrderStatus(orderId, OrderStatus.SHIPPED, "Seller shipped", "seller-1");

            // Pessimistic lock acquired
            verify(orderRepository).findByIdWithLock(orderId);

            // Transition validated
            verify(stateMachine).validateTransition(OrderStatus.CONFIRMED, OrderStatus.SHIPPED);

            // Status set and saved
            ArgumentCaptor<Order> orderCaptor = ArgumentCaptor.forClass(Order.class);
            verify(orderRepository).save(orderCaptor.capture());
            assertThat(orderCaptor.getValue().getStatus()).isEqualTo(OrderStatus.SHIPPED);

            // History recorded
            ArgumentCaptor<OrderStatusHistory> histCaptor =
                    ArgumentCaptor.forClass(OrderStatusHistory.class);
            verify(historyRepository).save(histCaptor.capture());
            OrderStatusHistory hist = histCaptor.getValue();
            assertThat(hist.getOldStatus()).isEqualTo(OrderStatus.CONFIRMED);
            assertThat(hist.getNewStatus()).isEqualTo(OrderStatus.SHIPPED);
            assertThat(hist.getReason()).isEqualTo("Seller shipped");
            assertThat(hist.getChangedBy()).isEqualTo("seller-1");
            assertThat(hist.getOrderId()).isEqualTo(orderId);
        }

        @Test
        void orderNotFound_throwsOrderNotFoundException() {
            when(orderRepository.findByIdWithLock(orderId)).thenReturn(Optional.empty());

            assertThatThrownBy(() ->
                    orderService.updateOrderStatus(orderId, OrderStatus.SHIPPED, "ship", "actor"))
                    .isInstanceOf(OrderNotFoundException.class)
                    .hasMessageContaining(orderId.toString());
        }

        @Test
        void invalidTransition_stateMachineThrows_propagated() {
            Order order = buildOrder(OrderStatus.DELIVERED);
            givenOrderExistsWithLock(order);
            doThrow(new InvalidOrderStateException("Cannot transition from DELIVERED to CANCELLED"))
                    .when(stateMachine).validateTransition(OrderStatus.DELIVERED, OrderStatus.CANCELLED);

            assertThatThrownBy(() ->
                    orderService.updateOrderStatus(orderId, OrderStatus.CANCELLED, "cancel", "actor"))
                    .isInstanceOf(InvalidOrderStateException.class)
                    .hasMessageContaining("DELIVERED");

            // Nothing saved when transition is invalid
            verify(orderRepository, never()).save(any());
            verify(historyRepository, never()).save(any());
        }
    }

    // ── GetOrderHistory ───────────────────────────────────────────────────────

    @Nested
    class GetOrderHistory {

        @Test
        void ownOrder_returnsHistoryInAscendingOrder() {
            Order order = buildOrder(OrderStatus.CONFIRMED);
            givenOrderExists(order);

            List<OrderStatusHistory> history = List.of(
                    OrderStatusHistory.builder().id(1L).orderId(orderId)
                            .oldStatus(null).newStatus(OrderStatus.PENDING)
                            .reason("Order created").changedBy(userId.toString())
                            .changedAt(OffsetDateTime.now().minusMinutes(10)).build(),
                    OrderStatusHistory.builder().id(2L).orderId(orderId)
                            .oldStatus(OrderStatus.PENDING).newStatus(OrderStatus.CONFIRMED)
                            .reason("Payment completed").changedBy("payment-service")
                            .changedAt(OffsetDateTime.now()).build()
            );
            when(historyRepository.findByOrderIdOrderByChangedAtAsc(orderId)).thenReturn(history);

            List<OrderStatusHistoryResponse> result = orderService.getOrderHistory(orderId, userId);

            assertThat(result).hasSize(2);
            assertThat(result.get(0).getOldStatus()).isNull();
            assertThat(result.get(0).getNewStatus()).isEqualTo(OrderStatus.PENDING);
            assertThat(result.get(1).getOldStatus()).isEqualTo(OrderStatus.PENDING);
            assertThat(result.get(1).getNewStatus()).isEqualTo(OrderStatus.CONFIRMED);
            assertThat(result.get(1).getChangedBy()).isEqualTo("payment-service");
        }

        @Test
        void orderNotFound_throwsOrderNotFoundException() {
            givenOrderNotFound();

            assertThatThrownBy(() -> orderService.getOrderHistory(orderId, userId))
                    .isInstanceOf(OrderNotFoundException.class);
        }

        @Test
        void differentUser_throwsOrderAccessDeniedException() {
            Order order = buildOrder(OrderStatus.PENDING);
            givenOrderExists(order);

            UUID otherUser = UUID.randomUUID();

            assertThatThrownBy(() -> orderService.getOrderHistory(orderId, otherUser))
                    .isInstanceOf(OrderAccessDeniedException.class)
                    .hasMessageContaining(orderId.toString());

            verify(historyRepository, never()).findByOrderIdOrderByChangedAtAsc(any());
        }
    }
}

package com.ecommerce.order_service.service.impl;

import com.ecommerce.order_service.client.ProductServiceClient;
import com.ecommerce.order_service.dto.*;
import com.ecommerce.order_service.exception.OrderAccessDeniedException;
import com.ecommerce.order_service.exception.OrderNotFoundException;
import com.ecommerce.order_service.kafka.OrderEventProducer;
import com.ecommerce.order_service.kafka.event.OrderCreatedEvent;
import com.ecommerce.order_service.model.*;
import com.ecommerce.order_service.repository.OrderRepository;
import com.ecommerce.order_service.repository.OrderStatusHistoryRepository;
import com.ecommerce.order_service.service.OrderService;
import com.ecommerce.order_service.service.OrderStateMachine;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.math.BigDecimal;
import java.util.ArrayList;
import java.util.List;
import java.util.UUID;
import java.util.concurrent.CompletableFuture;
import java.util.stream.Collectors;

@Slf4j
@Service
@RequiredArgsConstructor
@Transactional(readOnly = true)
public class OrderServiceImpl implements OrderService {

    private final OrderRepository orderRepository;
    private final OrderStatusHistoryRepository historyRepository;
    private final OrderStateMachine stateMachine;
    private final ProductServiceClient productServiceClient;
    private final OrderEventProducer eventProducer;

    @Override
    @Transactional
    public OrderResponse createOrder(UUID userId, CreateOrderRequest request) {
        // 1. Reserve stock for each item in parallel; compensate on any failure
        List<OrderItemRequest> items = request.getItems();
        List<Long> reservedProductIds = new ArrayList<>();

        try {
            List<CompletableFuture<ProductServiceClient.StockResponse>> futures = items.stream()
                    .map(item -> CompletableFuture.supplyAsync(() -> {
                        ProductServiceClient.StockResponse resp = productServiceClient.reserveStock(
                                item.getProductId(),
                                item.getQuantity(),
                                "order-" + userId
                        );
                        synchronized (reservedProductIds) {
                            reservedProductIds.add(item.getProductId());
                        }
                        return resp;
                    }))
                    .collect(Collectors.toList());

            CompletableFuture.allOf(futures.toArray(new CompletableFuture[0])).join();
        } catch (Exception e) {
            // Compensate: release all successfully reserved items
            log.warn("Stock reservation failed, releasing {} reserved items", reservedProductIds.size());
            for (Long productId : reservedProductIds) {
                try {
                    items.stream()
                            .filter(i -> i.getProductId().equals(productId))
                            .findFirst()
                            .ifPresent(i -> productServiceClient.releaseStock(
                                    productId, i.getQuantity(), "order-" + userId));
                } catch (Exception releaseEx) {
                    log.error("Failed to release stock for productId={}", productId, releaseEx);
                }
            }
            Throwable cause = e.getCause() != null ? e.getCause() : e;
            if (cause instanceof RuntimeException re) throw re;
            throw new IllegalStateException("Order creation failed: " + cause.getMessage(), cause);
        }

        // 2. Build order entity
        ShippingAddressDto addrDto = request.getShippingAddress();
        ShippingAddress address = ShippingAddress.builder()
                .street(addrDto.getStreet())
                .city(addrDto.getCity())
                .state(addrDto.getState())
                .country(addrDto.getCountry())
                .zipCode(addrDto.getZipCode())
                .build();

        Order order = Order.builder()
                .userId(userId)
                .cartId(request.getCartId())
                .status(OrderStatus.PENDING)
                .shippingAddress(address)
                .totalAmount(BigDecimal.ZERO) // computed below
                .items(new ArrayList<>())
                .build();

        // 3. Build and attach order items, compute total
        BigDecimal total = BigDecimal.ZERO;
        for (OrderItemRequest itemReq : items) {
            ProductServiceClient.ProductDetail product = productServiceClient.getProduct(itemReq.getProductId());
            BigDecimal unitPrice = product.getPrice();
            OrderItem item = OrderItem.builder()
                    .order(order)
                    .productId(itemReq.getProductId())
                    .productName(product.getName())
                    .quantity(itemReq.getQuantity())
                    .unitPrice(unitPrice)
                    .build();
            order.getItems().add(item);
            total = total.add(unitPrice.multiply(BigDecimal.valueOf(itemReq.getQuantity())));
        }
        order.setTotalAmount(total);

        // 4. Persist
        Order saved = orderRepository.save(order);

        // 5. Record initial history: null → PENDING
        historyRepository.save(OrderStatusHistory.builder()
                .orderId(saved.getId())
                .oldStatus(null)
                .newStatus(OrderStatus.PENDING)
                .reason("Order created")
                .changedBy(userId.toString())
                .build());

        // 6. Publish Kafka event
        OrderCreatedEvent event = OrderCreatedEvent.builder()
                .orderId(saved.getId())
                .userId(saved.getUserId())
                .totalAmount(saved.getTotalAmount())
                .items(saved.getItems().stream()
                        .map(i -> OrderCreatedEvent.OrderItemEvent.builder()
                                .productId(i.getProductId())
                                .quantity(i.getQuantity())
                                .unitPrice(i.getUnitPrice())
                                .build())
                        .collect(Collectors.toList()))
                .build();
        eventProducer.publishOrderCreated(event);

        log.info("Order created: orderId={}, userId={}, total={}", saved.getId(), userId, total);
        return OrderResponse.from(saved);
    }

    @Override
    public OrderResponse getOrder(UUID orderId, UUID userId) {
        Order order = orderRepository.findById(orderId)
                .orElseThrow(() -> new OrderNotFoundException(orderId));
        if (!order.getUserId().equals(userId)) {
            throw new OrderAccessDeniedException(orderId);
        }
        return OrderResponse.from(order);
    }

    @Override
    public Page<OrderSummaryResponse> listOrders(UUID userId, Pageable pageable) {
        return orderRepository.findByUserIdOrderByCreatedAtDesc(userId, pageable)
                .map(OrderSummaryResponse::from);
    }

    @Override
    @Transactional
    public OrderResponse cancelOrder(UUID orderId, UUID userId) {
        Order order = orderRepository.findById(orderId)
                .orElseThrow(() -> new OrderNotFoundException(orderId));
        if (!order.getUserId().equals(userId)) {
            throw new OrderAccessDeniedException(orderId);
        }
        OrderResponse response = updateOrderStatus(orderId, OrderStatus.CANCELLED, "User requested cancellation", userId.toString());

        // Release reserved stock
        for (OrderItem item : order.getItems()) {
            try {
                productServiceClient.releaseStock(item.getProductId(), item.getQuantity(), orderId.toString());
            } catch (Exception e) {
                log.error("Failed to release stock for productId={} on cancel", item.getProductId(), e);
            }
        }
        return response;
    }

    @Override
    @Transactional
    public OrderResponse updateOrderStatus(UUID orderId, OrderStatus newStatus, String reason, String changedBy) {
        // Pessimistic lock: only one transaction wins concurrent transitions
        Order order = orderRepository.findByIdWithLock(orderId)
                .orElseThrow(() -> new OrderNotFoundException(orderId));

        OrderStatus oldStatus = order.getStatus();
        stateMachine.validateTransition(oldStatus, newStatus);

        order.setStatus(newStatus);
        orderRepository.save(order);

        historyRepository.save(OrderStatusHistory.builder()
                .orderId(orderId)
                .oldStatus(oldStatus)
                .newStatus(newStatus)
                .reason(reason)
                .changedBy(changedBy)
                .build());

        log.info("Order {} transitioned {} → {} by {}", orderId, oldStatus, newStatus, changedBy);
        return OrderResponse.from(order);
    }

    @Override
    public List<OrderStatusHistoryResponse> getOrderHistory(UUID orderId, UUID userId) {
        Order order = orderRepository.findById(orderId)
                .orElseThrow(() -> new OrderNotFoundException(orderId));
        if (!order.getUserId().equals(userId)) {
            throw new OrderAccessDeniedException(orderId);
        }
        return historyRepository.findByOrderIdOrderByChangedAtAsc(orderId).stream()
                .map(OrderStatusHistoryResponse::from)
                .collect(Collectors.toList());
    }

}

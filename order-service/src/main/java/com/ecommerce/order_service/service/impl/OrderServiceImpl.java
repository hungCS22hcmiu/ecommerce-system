package com.ecommerce.order_service.service.impl;

import com.ecommerce.order_service.client.ProductServiceClient;
import com.ecommerce.order_service.dto.*;
import com.ecommerce.order_service.exception.OrderAccessDeniedException;
import com.ecommerce.order_service.exception.OrderNotFoundException;
import com.ecommerce.order_service.kafka.OrderEventProducer;
import com.ecommerce.order_service.kafka.event.OrderCreatedEvent;
import com.ecommerce.order_service.model.*;
import com.ecommerce.order_service.repository.OrderItemRepository;
import com.ecommerce.order_service.repository.OrderRepository;
import com.ecommerce.order_service.repository.OrderStatusHistoryRepository;
import com.ecommerce.order_service.repository.OutboxEventRepository;
import com.ecommerce.order_service.service.NotificationService;
import com.ecommerce.order_service.service.OrderService;
import com.ecommerce.order_service.service.OrderStateMachine;
import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.slf4j.MDC;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;
import org.springframework.scheduling.annotation.Async;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.math.BigDecimal;
import java.time.OffsetDateTime;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;
import java.util.concurrent.CompletableFuture;
import java.util.stream.Collectors;

@Slf4j
@Service
@RequiredArgsConstructor
@Transactional(readOnly = true)
public class OrderServiceImpl implements OrderService {

    private final OrderRepository orderRepository;
    private final OrderItemRepository orderItemRepository;
    private final OrderStatusHistoryRepository historyRepository;
    private final OrderStateMachine stateMachine;
    private final ProductServiceClient productServiceClient;
    private final OrderEventProducer eventProducer;
    private final NotificationService notificationService;
    private final OutboxEventRepository outboxEventRepository;
    private final ObjectMapper objectMapper;

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

        // 3. Build and attach order items, compute total, validate same seller
        BigDecimal total = BigDecimal.ZERO;
        UUID orderSellerId = null;

        for (OrderItemRequest itemReq : items) {
            ProductServiceClient.ProductDetail product = productServiceClient.getProduct(itemReq.getProductId());

            UUID productSellerId = UUID.fromString(product.getSellerId());
            if (orderSellerId == null) {
                orderSellerId = productSellerId;
            } else if (!orderSellerId.equals(productSellerId)) {
                for (OrderItemRequest i : items) {
                    try {
                        productServiceClient.releaseStock(i.getProductId(), i.getQuantity(), "order-" + userId);
                    } catch (Exception ex) {
                        log.error("Failed to release stock for productId={} after mixed-seller rejection", i.getProductId(), ex);
                    }
                }
                throw new IllegalArgumentException("All items in an order must be from the same seller");
            }

            OrderItem item = OrderItem.builder()
                    .order(order)
                    .productId(itemReq.getProductId())
                    .productName(product.getName())
                    .quantity(itemReq.getQuantity())
                    .unitPrice(product.getPrice())
                    .build();
            order.getItems().add(item);
            total = total.add(product.getPrice().multiply(BigDecimal.valueOf(itemReq.getQuantity())));
        }
        order.setTotalAmount(total);
        order.setSellerId(orderSellerId);

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

        // 6. Write Kafka event to outbox (same TX — atomic with the order insert)
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
        String correlationId = Optional.ofNullable(MDC.get("correlationId"))
                .orElse(UUID.randomUUID().toString());
        try {
            outboxEventRepository.save(OutboxEvent.builder()
                    .orderId(saved.getId())
                    .payload(objectMapper.writeValueAsString(event))
                    .headers(objectMapper.writeValueAsString(Map.of("X-Correlation-ID", correlationId)))
                    .createdAt(OffsetDateTime.now())
                    .build());
        } catch (JsonProcessingException e) {
            throw new IllegalStateException("Failed to serialize outbox event for orderId=" + saved.getId(), e);
        }

        try {
            notificationService.notifySeller(orderSellerId, saved.getId(),
                    "New order #" + saved.getId().toString().substring(0, 8).toUpperCase(),
                    "A customer placed an order totalling $" + total);
        } catch (Exception e) {
            log.warn("Failed to notify seller of new order", e);
        }

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

        if (order.getSellerId() != null) {
            try {
                notificationService.notifySeller(order.getSellerId(), orderId,
                        "Order cancelled by customer",
                        "Order #" + orderId.toString().substring(0, 8).toUpperCase() + " was cancelled by the buyer.");
            } catch (Exception e) {
                log.warn("Failed to notify seller of order cancellation", e);
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

    @Override
    public boolean verifyPurchase(UUID userId, Long productId, UUID orderItemId) {
        return orderItemRepository.findVerifiedDeliveredItem(orderItemId, userId, productId).isPresent();
    }

    @Override
    @Async("taskExecutor")
    @Transactional
    public void releaseStockForOrder(UUID orderId) {
        Order order = orderRepository.findById(orderId)
                .orElseThrow(() -> new OrderNotFoundException(orderId));
        for (OrderItem item : order.getItems()) {
            try {
                productServiceClient.releaseStock(item.getProductId(), item.getQuantity(), orderId.toString());
            } catch (Exception e) {
                log.error("Failed to release stock for productId={} on payment failure", item.getProductId(), e);
            }
        }
    }

    @Override
    @Transactional
    public OrderResponse shipOrder(UUID orderId, UUID sellerId) {
        Order order = orderRepository.findByIdWithLock(orderId)
                .orElseThrow(() -> new OrderNotFoundException(orderId));
        if (!sellerId.equals(order.getSellerId())) {
            throw new OrderAccessDeniedException(orderId);
        }
        OrderResponse response = updateOrderStatus(orderId, OrderStatus.SHIPPED, "Shipped by seller", sellerId.toString());

        try {
            notificationService.notifyBuyer(order.getUserId(), orderId,
                    "Your order has been shipped",
                    "Order #" + orderId.toString().substring(0, 8).toUpperCase() + " is on its way.");
        } catch (Exception e) {
            log.warn("Failed to notify buyer of shipment", e);
        }

        return response;
    }

    @Override
    public Page<OrderSummaryResponse> listSellerOrders(UUID sellerId, OrderStatus status, Pageable pageable) {
        Page<Order> page = (status != null)
                ? orderRepository.findBySellerIdAndStatusOrderByCreatedAtDesc(sellerId, status, pageable)
                : orderRepository.findBySellerIdOrderByCreatedAtDesc(sellerId, pageable);
        return page.map(OrderSummaryResponse::from);
    }

    @Override
    public OrderResponse getOrderAsSeller(UUID orderId, UUID sellerId) {
        Order order = orderRepository.findById(orderId)
                .orElseThrow(() -> new OrderNotFoundException(orderId));
        if (!sellerId.equals(order.getSellerId())) {
            throw new OrderAccessDeniedException(orderId);
        }
        return OrderResponse.from(order);
    }

}

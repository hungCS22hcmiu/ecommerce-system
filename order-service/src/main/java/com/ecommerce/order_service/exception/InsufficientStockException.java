package com.ecommerce.order_service.exception;

public class InsufficientStockException extends RuntimeException {
    public InsufficientStockException(Long productId) {
        super("Insufficient stock for product: " + productId);
    }

    public InsufficientStockException(String message) {
        super(message);
    }
}

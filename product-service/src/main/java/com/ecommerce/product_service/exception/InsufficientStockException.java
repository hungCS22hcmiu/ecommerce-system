package com.ecommerce.product_service.exception;

public class InsufficientStockException extends RuntimeException {

    public InsufficientStockException(Long productId, int requested, int available) {
        super("Insufficient stock for product " + productId +
              ": requested=" + requested + ", available=" + available);
    }
}

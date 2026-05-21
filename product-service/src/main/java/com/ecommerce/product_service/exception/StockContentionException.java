package com.ecommerce.product_service.exception;

public class StockContentionException extends RuntimeException {
    public StockContentionException(Long productId) {
        super("Stock reservation for product " + productId + " is under heavy contention. Please retry.");
    }
}

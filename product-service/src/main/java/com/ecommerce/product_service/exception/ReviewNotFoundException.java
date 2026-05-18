package com.ecommerce.product_service.exception;

public class ReviewNotFoundException extends RuntimeException {
    public ReviewNotFoundException(Long id) {
        super("Review not found: " + id);
    }
}

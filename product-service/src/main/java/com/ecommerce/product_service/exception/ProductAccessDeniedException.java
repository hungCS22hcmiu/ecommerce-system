package com.ecommerce.product_service.exception;

public class ProductAccessDeniedException extends RuntimeException {

    public ProductAccessDeniedException(Long id) {
        super("Access denied for product: " + id);
    }
}

package com.ecommerce.product_service.exception;

import lombok.Getter;

@Getter
public class AIServiceException extends RuntimeException {

    private final String query;
    private final int limit;

    public AIServiceException(String message, Throwable cause, String query, int limit) {
        super(message, cause);
        this.query = query;
        this.limit = limit;
    }
}

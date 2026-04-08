package com.ecommerce.product_service.dto;

public record StockResponse(
        Long productId,
        int stockQuantity,
        int stockReserved,
        int availableStock
) {}

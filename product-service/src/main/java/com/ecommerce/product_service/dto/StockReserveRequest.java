package com.ecommerce.product_service.dto;

import jakarta.validation.constraints.Min;
import jakarta.validation.constraints.NotNull;

public record StockReserveRequest(
        @NotNull @Min(1) Integer quantity,
        String referenceId
) {}

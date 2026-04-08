package com.ecommerce.product_service.dto;

import jakarta.validation.constraints.Min;
import jakarta.validation.constraints.NotNull;

public record StockReleaseRequest(
        @NotNull @Min(1) Integer quantity,
        String referenceId
) {}

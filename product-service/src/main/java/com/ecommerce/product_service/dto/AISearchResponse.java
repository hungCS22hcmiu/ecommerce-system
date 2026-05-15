package com.ecommerce.product_service.dto;

import java.util.List;

public record AISearchResponse(
        String query,
        List<ProductSummaryResponse> results,
        List<Double> scores,
        String mode   // "ai" | "fallback-keyword"
) {}

package com.ecommerce.product_service.service;

import com.ecommerce.product_service.dto.AISearchResponse;

import java.util.UUID;

public interface AISearchService {
    AISearchResponse search(String query, int limit, Long categoryId, UUID sellerId);
}

package com.ecommerce.product_service.service;

import com.ecommerce.product_service.dto.StockResponse;

public interface InventoryService {
    StockResponse reserveStock(Long productId, int quantity, String referenceId);
    StockResponse releaseStock(Long productId, int quantity, String referenceId);
    StockResponse getStockLevel(Long productId);
}

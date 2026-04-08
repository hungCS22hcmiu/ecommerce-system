package com.ecommerce.product_service.controller;

import com.ecommerce.product_service.dto.ApiResponse;
import com.ecommerce.product_service.dto.StockReleaseRequest;
import com.ecommerce.product_service.dto.StockReserveRequest;
import com.ecommerce.product_service.dto.StockResponse;
import com.ecommerce.product_service.service.InventoryService;
import jakarta.validation.Valid;
import lombok.RequiredArgsConstructor;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/api/v1/inventory")
@RequiredArgsConstructor
public class InventoryController {

    private final InventoryService inventoryService;

    @PostMapping("/{productId}/reserve")
    public ApiResponse<StockResponse> reserve(@PathVariable Long productId,
                                              @Valid @RequestBody StockReserveRequest request) {
        return ApiResponse.ok(inventoryService.reserveStock(productId, request.quantity(), request.referenceId()));
    }

    @PostMapping("/{productId}/release")
    public ApiResponse<StockResponse> release(@PathVariable Long productId,
                                              @Valid @RequestBody StockReleaseRequest request) {
        return ApiResponse.ok(inventoryService.releaseStock(productId, request.quantity(), request.referenceId()));
    }

    @GetMapping("/{productId}")
    public ApiResponse<StockResponse> getStockLevel(@PathVariable Long productId) {
        return ApiResponse.ok(inventoryService.getStockLevel(productId));
    }
}

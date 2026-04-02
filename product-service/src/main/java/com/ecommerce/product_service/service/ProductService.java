package com.ecommerce.product_service.service;

import com.ecommerce.product_service.dto.CreateProductRequest;
import com.ecommerce.product_service.dto.ProductResponse;
import com.ecommerce.product_service.dto.ProductSummaryResponse;
import com.ecommerce.product_service.dto.UpdateProductRequest;
import com.ecommerce.product_service.model.ProductStatus;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;

import java.util.UUID;

public interface ProductService {

    ProductResponse createProduct(UUID sellerId, CreateProductRequest request);

    ProductResponse getProduct(Long id);

    Page<ProductSummaryResponse> listProducts(Long categoryId, ProductStatus status, Pageable pageable);

    Page<ProductSummaryResponse> searchProducts(String query, Pageable pageable);

    ProductResponse updateProduct(Long id, UUID sellerId, UpdateProductRequest request);

    void deleteProduct(Long id, UUID sellerId);
}

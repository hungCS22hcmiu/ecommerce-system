package com.ecommerce.product_service.dto;

import com.ecommerce.product_service.model.ProductStatus;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.math.BigDecimal;
import java.time.OffsetDateTime;
import java.util.List;
import java.util.UUID;

@Data
@NoArgsConstructor
@AllArgsConstructor
@Builder
public class ProductResponse {

    private Long id;
    private String name;
    private String description;
    private BigDecimal price;

    private Long categoryId;
    private String categoryName;

    private UUID sellerId;
    private ProductStatus status;

    private int stockQuantity;
    private int stockReserved;
    private int stockAvailable;

    private Long version;

    private List<ProductImageResponse> images;

    private OffsetDateTime createdAt;
    private OffsetDateTime updatedAt;
}

package com.ecommerce.product_service.dto;

import com.ecommerce.product_service.model.ProductStatus;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.math.BigDecimal;
import java.time.OffsetDateTime;
import java.util.UUID;

@Data
@NoArgsConstructor
@AllArgsConstructor
@Builder
public class ProductSummaryResponse {

    private Long id;
    private String name;
    private BigDecimal price;

    private Long categoryId;
    private String categoryName;

    private UUID sellerId;
    private ProductStatus status;

    private int stockAvailable;
    private String thumbnailUrl;

    private OffsetDateTime createdAt;
}

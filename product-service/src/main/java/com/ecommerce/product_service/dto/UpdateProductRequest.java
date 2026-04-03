package com.ecommerce.product_service.dto;

import com.ecommerce.product_service.model.ProductStatus;
import jakarta.validation.Valid;
import jakarta.validation.constraints.DecimalMin;
import jakarta.validation.constraints.Min;
import jakarta.validation.constraints.Size;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.math.BigDecimal;
import java.util.List;

@Data
@NoArgsConstructor
@AllArgsConstructor
@Builder
public class UpdateProductRequest {

    @Size(max = 200)
    private String name;

    private String description;

    @DecimalMin("0.00")
    private BigDecimal price;

    private Long categoryId;

    private ProductStatus status;

    @Min(0)
    private Integer stockQuantity;

    @Valid
    private List<ProductImageRequest> images;
}

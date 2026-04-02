package com.ecommerce.product_service.dto;

import jakarta.validation.Valid;
import jakarta.validation.constraints.DecimalMin;
import jakarta.validation.constraints.Min;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
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
public class CreateProductRequest {

    @NotBlank
    @Size(max = 200)
    private String name;

    private String description;

    @NotNull
    @DecimalMin("0.00")
    private BigDecimal price;

    private Long categoryId;

    @Min(0)
    private int stockQuantity;

    @Valid
    private List<ProductImageRequest> images;
}

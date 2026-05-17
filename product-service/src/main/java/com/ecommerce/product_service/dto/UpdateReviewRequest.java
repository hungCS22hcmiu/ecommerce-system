package com.ecommerce.product_service.dto;

import jakarta.validation.constraints.Max;
import jakarta.validation.constraints.Min;
import lombok.Data;

@Data
public class UpdateReviewRequest {

    @Min(1) @Max(5)
    private int rating;

    private String comment;
}

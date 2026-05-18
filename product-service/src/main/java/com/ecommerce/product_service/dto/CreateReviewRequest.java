package com.ecommerce.product_service.dto;

import jakarta.validation.constraints.Max;
import jakarta.validation.constraints.Min;
import jakarta.validation.constraints.NotNull;
import lombok.Data;
import java.util.UUID;

@Data
public class CreateReviewRequest {

    @NotNull
    private UUID orderItemId;

    @Min(1) @Max(5)
    private int rating;

    private String comment;
}

package com.ecommerce.product_service.dto;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;
import java.time.OffsetDateTime;
import java.util.UUID;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class ReviewResponse {
    private Long id;
    private Long productId;
    private UUID customerId;
    private UUID orderItemId;
    private int rating;
    private String comment;
    private OffsetDateTime createdAt;
    private OffsetDateTime updatedAt;
}

package com.ecommerce.product_service.service;

import com.ecommerce.product_service.dto.CreateReviewRequest;
import com.ecommerce.product_service.dto.ReviewResponse;
import com.ecommerce.product_service.dto.UpdateReviewRequest;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;

import java.util.Optional;
import java.util.UUID;

public interface ReviewService {
    ReviewResponse createReview(Long productId, UUID customerId, CreateReviewRequest req);
    ReviewResponse updateReview(Long reviewId, UUID customerId, UpdateReviewRequest req);
    Optional<ReviewResponse> getMyReviewByOrderItem(UUID orderItemId, UUID customerId);
    Page<ReviewResponse> getReviews(Long productId, Pageable pageable);
}

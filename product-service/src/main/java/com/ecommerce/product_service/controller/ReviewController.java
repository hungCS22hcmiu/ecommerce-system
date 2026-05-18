package com.ecommerce.product_service.controller;

import com.ecommerce.product_service.dto.*;
import com.ecommerce.product_service.service.ReviewService;
import jakarta.validation.Valid;
import lombok.RequiredArgsConstructor;
import org.springframework.data.domain.Pageable;
import org.springframework.data.domain.Sort;
import org.springframework.data.web.PageableDefault;
import org.springframework.http.HttpStatus;
import org.springframework.web.bind.annotation.*;

import java.util.List;
import java.util.UUID;

@RestController
@RequestMapping("/api/v1/products/{productId}/reviews")
@RequiredArgsConstructor
public class ReviewController {

    private final ReviewService reviewService;

    @PostMapping
    @ResponseStatus(HttpStatus.CREATED)
    public ApiResponse<ReviewResponse> create(
            @PathVariable Long productId,
            @RequestHeader("X-User-Id") UUID customerId,
            @Valid @RequestBody CreateReviewRequest req) {
        return ApiResponse.ok(reviewService.createReview(productId, customerId, req));
    }

    @PutMapping("/{reviewId}")
    public ApiResponse<ReviewResponse> update(
            @PathVariable Long productId,
            @PathVariable Long reviewId,
            @RequestHeader("X-User-Id") UUID customerId,
            @Valid @RequestBody UpdateReviewRequest req) {
        return ApiResponse.ok(reviewService.updateReview(reviewId, customerId, req));
    }

    @GetMapping("/by-order-item/{orderItemId}")
    public ApiResponse<ReviewResponse> getMyReviewByOrderItem(
            @PathVariable Long productId,
            @PathVariable UUID orderItemId,
            @RequestHeader("X-User-Id") UUID customerId) {
        return ApiResponse.ok(reviewService.getMyReviewByOrderItem(orderItemId, customerId).orElse(null));
    }

    @GetMapping
    public ApiResponse<List<ReviewResponse>> list(
            @PathVariable Long productId,
            @PageableDefault(size = 10, sort = "createdAt", direction = Sort.Direction.DESC) Pageable pageable) {
        return ApiResponse.ok(reviewService.getReviews(productId, pageable));
    }
}

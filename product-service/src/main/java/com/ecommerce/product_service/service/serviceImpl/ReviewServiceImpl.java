package com.ecommerce.product_service.service.serviceImpl;

import com.ecommerce.product_service.client.OrderServiceClient;
import com.ecommerce.product_service.dto.CreateReviewRequest;
import com.ecommerce.product_service.dto.ReviewResponse;
import com.ecommerce.product_service.dto.UpdateReviewRequest;
import com.ecommerce.product_service.exception.*;
import com.ecommerce.product_service.model.Product;
import com.ecommerce.product_service.model.ProductReview;
import com.ecommerce.product_service.model.ProductStatus;
import com.ecommerce.product_service.repository.ProductRepository;
import com.ecommerce.product_service.repository.ProductReviewRepository;
import com.ecommerce.product_service.service.ReviewService;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.cache.CacheManager;
import org.springframework.dao.DataIntegrityViolationException;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.math.BigDecimal;
import java.math.RoundingMode;
import java.time.OffsetDateTime;
import java.util.Objects;
import java.util.Optional;
import java.util.UUID;

@Slf4j
@Service
@Transactional(readOnly = true)
@RequiredArgsConstructor
public class ReviewServiceImpl implements ReviewService {

    private final ProductRepository productRepository;
    private final ProductReviewRepository reviewRepository;
    private final OrderServiceClient orderServiceClient;
    private final CacheManager cacheManager;

    @Override
    @Transactional
    public ReviewResponse createReview(Long productId, UUID customerId, CreateReviewRequest req) {
        Product product = productRepository.findById(productId)
                .filter(p -> p.getStatus() == ProductStatus.ACTIVE)
                .orElseThrow(() -> new ProductNotFoundException(productId));

        if (!orderServiceClient.verifyPurchase(customerId, productId, req.getOrderItemId())) {
            throw new PurchaseNotVerifiedException();
        }

        ProductReview review = ProductReview.builder()
                .product(product)
                .customerId(customerId)
                .orderItemId(req.getOrderItemId())
                .rating(req.getRating())
                .comment(req.getComment())
                .createdAt(OffsetDateTime.now())
                .updatedAt(OffsetDateTime.now())
                .build();
        try {
            reviewRepository.save(review);
        } catch (DataIntegrityViolationException e) {
            throw new AlreadyReviewedException();
        }

        recalculateAndEvict(product);

        String notifBody = req.getRating() + "/5 stars"
                + (req.getComment() != null && !req.getComment().isBlank()
                   ? " — " + req.getComment().substring(0, Math.min(req.getComment().length(), 80)) : "");
        orderServiceClient.notifySellerReview(
                product.getSellerId(), product.getId(),
                "New review on " + product.getName(), notifBody);

        return toReviewResponse(review);
    }

    @Override
    @Transactional
    public ReviewResponse updateReview(Long reviewId, UUID customerId, UpdateReviewRequest req) {
        ProductReview review = reviewRepository.findById(reviewId)
                .orElseThrow(() -> new ReviewNotFoundException(reviewId));
        if (!review.getCustomerId().equals(customerId)) {
            throw new ReviewAccessDeniedException();
        }

        review.setRating(req.getRating());
        review.setComment(req.getComment());
        review.setUpdatedAt(OffsetDateTime.now());
        reviewRepository.save(review);

        recalculateAndEvict(review.getProduct());
        return toReviewResponse(review);
    }

    @Override
    public Optional<ReviewResponse> getMyReviewByOrderItem(UUID orderItemId, UUID customerId) {
        return reviewRepository.findByOrderItemIdAndCustomerId(orderItemId, customerId)
                .map(this::toReviewResponse);
    }

    @Override
    public Page<ReviewResponse> getReviews(Long productId, Pageable pageable) {
        return reviewRepository.findByProductIdOrderByCreatedAtDesc(productId, pageable)
                .map(this::toReviewResponse);
    }

    private void recalculateAndEvict(Product product) {
        double avg = reviewRepository.findAvgRating(product.getId()).orElse(0.0);
        long count = reviewRepository.countByProductId(product.getId());
        product.setAvgRating(BigDecimal.valueOf(avg).setScale(2, RoundingMode.HALF_UP));
        product.setRatingCount((int) count);
        productRepository.save(product);
        Objects.requireNonNull(cacheManager.getCache("productList")).clear();
        var productCache = cacheManager.getCache("product");
        if (productCache != null) productCache.evict(product.getId());
    }

    private ReviewResponse toReviewResponse(ProductReview r) {
        return ReviewResponse.builder()
                .id(r.getId())
                .productId(r.getProduct().getId())
                .customerId(r.getCustomerId())
                .orderItemId(r.getOrderItemId())
                .rating(r.getRating())
                .comment(r.getComment())
                .createdAt(r.getCreatedAt())
                .updatedAt(r.getUpdatedAt())
                .build();
    }
}

package com.ecommerce.product_service.repository;

import com.ecommerce.product_service.model.ProductReview;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;
import org.springframework.stereotype.Repository;

import java.util.Optional;
import java.util.UUID;

@Repository
public interface ProductReviewRepository extends JpaRepository<ProductReview, Long> {

    Page<ProductReview> findByProductIdOrderByCreatedAtDesc(Long productId, Pageable pageable);

    boolean existsByOrderItemId(UUID orderItemId);

    Optional<ProductReview> findByOrderItemIdAndCustomerId(UUID orderItemId, UUID customerId);

    @Query("SELECT AVG(CAST(r.rating AS double)) FROM ProductReview r WHERE r.product.id = :productId")
    Optional<Double> findAvgRating(@Param("productId") Long productId);

    @Query("SELECT COUNT(r) FROM ProductReview r WHERE r.product.id = :productId")
    long countByProductId(@Param("productId") Long productId);
}

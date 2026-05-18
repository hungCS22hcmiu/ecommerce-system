package com.ecommerce.order_service.repository;

import com.ecommerce.order_service.model.OrderItem;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;
import org.springframework.stereotype.Repository;

import java.util.Optional;
import java.util.UUID;

@Repository
public interface OrderItemRepository extends JpaRepository<OrderItem, UUID> {
    @Query("""
        SELECT oi FROM OrderItem oi JOIN oi.order o
        WHERE oi.id = :itemId
          AND o.userId = :userId
          AND o.status = 'DELIVERED'
          AND oi.productId = :productId
        """)
    Optional<OrderItem> findVerifiedDeliveredItem(
        @Param("itemId") UUID itemId,
        @Param("userId") UUID userId,
        @Param("productId") Long productId);
}

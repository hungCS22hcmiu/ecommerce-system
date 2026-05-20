package com.ecommerce.order_service.repository;

import com.ecommerce.order_service.model.Order;
import com.ecommerce.order_service.model.OrderStatus;
import jakarta.persistence.LockModeType;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Lock;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.time.OffsetDateTime;
import java.util.List;
import java.util.Optional;
import java.util.UUID;

public interface OrderRepository extends JpaRepository<Order, UUID> {

    @Query("SELECT o FROM Order o WHERE o.id = :id")
    @Lock(LockModeType.PESSIMISTIC_WRITE)
    Optional<Order> findByIdWithLock(@Param("id") UUID id);

    Page<Order> findByUserIdOrderByCreatedAtDesc(UUID userId, Pageable pageable);

    Page<Order> findBySellerIdOrderByCreatedAtDesc(UUID sellerId, Pageable pageable);

    Page<Order> findBySellerIdAndStatusOrderByCreatedAtDesc(UUID sellerId, OrderStatus status, Pageable pageable);

    // Reaper query: PENDING orders older than threshold with no unpublished outbox row
    @Query(value = """
            SELECT o.id::text FROM orders o
            WHERE o.status = 'PENDING'
            AND o.created_at < :threshold
            AND NOT EXISTS (
                SELECT 1 FROM orders_outbox ob
                WHERE ob.order_id = o.id
                AND ob.published_at IS NULL
            )
            """, nativeQuery = true)
    List<String> findStuckPendingOrderIds(@Param("threshold") OffsetDateTime threshold);
}

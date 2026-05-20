package com.ecommerce.order_service.repository;

import com.ecommerce.order_service.model.OutboxEvent;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;

import java.util.List;

public interface OutboxEventRepository extends JpaRepository<OutboxEvent, Long> {

    @Query(value = "SELECT * FROM orders_outbox WHERE published_at IS NULL ORDER BY id LIMIT 100 FOR UPDATE SKIP LOCKED",
           nativeQuery = true)
    List<OutboxEvent> findUnpublishedForUpdate();
}

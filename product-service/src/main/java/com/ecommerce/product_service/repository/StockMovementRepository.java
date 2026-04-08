package com.ecommerce.product_service.repository;

import com.ecommerce.product_service.model.StockMovement;
import org.springframework.data.jpa.repository.JpaRepository;

public interface StockMovementRepository extends JpaRepository<StockMovement, Long> {
}

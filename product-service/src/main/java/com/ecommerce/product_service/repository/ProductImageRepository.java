package com.ecommerce.product_service.repository;

import com.ecommerce.product_service.model.ProductImage;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;

public interface ProductImageRepository extends JpaRepository<ProductImage, Long> {

    // Ordered image list for a product (used when loading images separately)
    List<ProductImage> findByProductIdOrderBySortOrderAsc(Long productId);
}

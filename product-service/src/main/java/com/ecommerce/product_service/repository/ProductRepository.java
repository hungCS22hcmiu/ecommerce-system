package com.ecommerce.product_service.repository;

import com.ecommerce.product_service.model.Product;
import com.ecommerce.product_service.model.ProductStatus;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.JpaSpecificationExecutor;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.util.List;
import java.util.Optional;
import java.util.UUID;

public interface ProductRepository extends JpaRepository<Product, Long>, JpaSpecificationExecutor<Product> {

    // Fetch a single active product (used for public GET /products/:id)
    Optional<Product> findByIdAndStatus(Long id, ProductStatus status);

    // Seller's own product listing
    Page<Product> findBySellerId(UUID sellerId, Pageable pageable);

    // Products by category
    Page<Product> findByCategoryIdAndStatus(Long categoryId, ProductStatus status, Pageable pageable);

    // Ownership check before update/delete
    boolean existsByIdAndSellerId(Long id, UUID sellerId);

    // Top 100 most recently active products — used for cache warming on startup
    List<Product> findTop100ByStatusOrderByUpdatedAtDesc(ProductStatus status);

    // Full-text search using the GIN index: idx_products_fts
    // to_tsvector('english', name || ' ' || COALESCE(description, ''))
    @Query(value = """
            SELECT * FROM products
            WHERE to_tsvector('english', name || ' ' || COALESCE(description, ''))
                  @@ plainto_tsquery('english', :query)
              AND status = 'ACTIVE'
            """,
            countQuery = """
            SELECT COUNT(*) FROM products
            WHERE to_tsvector('english', name || ' ' || COALESCE(description, ''))
                  @@ plainto_tsquery('english', :query)
              AND status = 'ACTIVE'
            """,
            nativeQuery = true)
    Page<Product> searchActive(@Param("query") String query, Pageable pageable);
}

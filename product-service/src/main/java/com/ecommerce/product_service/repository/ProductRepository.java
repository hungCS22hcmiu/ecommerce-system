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

    // Seller's own products filtered by status
    Page<Product> findBySellerIdAndStatus(UUID sellerId, ProductStatus status, Pageable pageable);

    // Seller's rated products only (ratingCount > 0)
    Page<Product> findBySellerIdAndRatingCountGreaterThan(UUID sellerId, int minCount, Pageable pageable);
    Page<Product> findBySellerIdAndStatusAndRatingCountGreaterThan(UUID sellerId, ProductStatus status, int minCount, Pageable pageable);

    // Ownership check before update/delete
    boolean existsByIdAndSellerId(Long id, UUID sellerId);

    // Top 100 most recently active products — used for cache warming on startup
    List<Product> findTop100ByStatusOrderByUpdatedAtDesc(ProductStatus status);

    // Full-text search using the GIN index: idx_products_fts
    // Results ranked by ts_rank boosted by avg_rating and stock_reserved (popularity).
    @Query(value = """
            SELECT * FROM products
            WHERE to_tsvector('english', name || ' ' || COALESCE(description, ''))
                  @@ plainto_tsquery('english', :query)
              AND status = 'ACTIVE'
              AND (:categoryId IS NULL OR category_id = :categoryId)
            ORDER BY
              ts_rank(to_tsvector('english', name || ' ' || COALESCE(description, '')),
                      plainto_tsquery('english', :query))
              * (1.0 + COALESCE(avg_rating, 0)::float / 5.0 * 0.2
                     + LEAST(stock_reserved, 50)::float / 50.0 * 0.1) DESC
            """,
            countQuery = """
            SELECT COUNT(*) FROM products
            WHERE to_tsvector('english', name || ' ' || COALESCE(description, ''))
                  @@ plainto_tsquery('english', :query)
              AND status = 'ACTIVE'
              AND (:categoryId IS NULL OR category_id = :categoryId)
            """,
            nativeQuery = true)
    Page<Product> searchActive(@Param("query") String query,
                               @Param("categoryId") Long categoryId,
                               Pageable pageable);

    // Full-text search scoped to a single seller
    @Query(value = """
            SELECT * FROM products
            WHERE to_tsvector('english', name || ' ' || COALESCE(description, ''))
                  @@ plainto_tsquery('english', :query)
              AND status = 'ACTIVE'
              AND seller_id = :sellerId
              AND (:categoryId IS NULL OR category_id = :categoryId)
            ORDER BY
              ts_rank(to_tsvector('english', name || ' ' || COALESCE(description, '')),
                      plainto_tsquery('english', :query))
              * (1.0 + COALESCE(avg_rating, 0)::float / 5.0 * 0.2
                     + LEAST(stock_reserved, 50)::float / 50.0 * 0.1) DESC
            """,
            countQuery = """
            SELECT COUNT(*) FROM products
            WHERE to_tsvector('english', name || ' ' || COALESCE(description, ''))
                  @@ plainto_tsquery('english', :query)
              AND status = 'ACTIVE'
              AND seller_id = :sellerId
              AND (:categoryId IS NULL OR category_id = :categoryId)
            """,
            nativeQuery = true)
    Page<Product> searchActiveBySeller(@Param("query") String query,
                                       @Param("sellerId") UUID sellerId,
                                       @Param("categoryId") Long categoryId,
                                       Pageable pageable);

    // Returns [id (Long), score (Double)] pairs ordered by cosine similarity descending.
    // Uses id + score only to avoid mapping the unmapped vector(384) column.
    // Caller must SET LOCAL ivfflat.probes before this query (pgvector reads the
    // setting at access-method init, so a CTE or WHERE trick won't work).
    @Query(value = """
            SELECT id, (1 - (embedding <=> CAST(:queryVec AS vector))) AS score
            FROM products
            WHERE status = 'ACTIVE' AND embedding IS NOT NULL
              AND (:categoryId IS NULL OR category_id = :categoryId)
            ORDER BY embedding <=> CAST(:queryVec AS vector)
            LIMIT :limit
            """, nativeQuery = true)
    List<Object[]> findIdsBySemanticSimilarity(@Param("queryVec") String queryVec,
                                               @Param("categoryId") Long categoryId,
                                               @Param("limit") int limit);

    // AI/vector search scoped to a single seller
    @Query(value = """
            SELECT id, (1 - (embedding <=> CAST(:queryVec AS vector))) AS score
            FROM products
            WHERE status = 'ACTIVE' AND embedding IS NOT NULL
              AND seller_id = :sellerId
              AND (:categoryId IS NULL OR category_id = :categoryId)
            ORDER BY embedding <=> CAST(:queryVec AS vector)
            LIMIT :limit
            """, nativeQuery = true)
    List<Object[]> findIdsBySemanticSimilarityBySeller(@Param("queryVec") String queryVec,
                                                       @Param("sellerId") UUID sellerId,
                                                       @Param("categoryId") Long categoryId,
                                                       @Param("limit") int limit);
}

package com.ecommerce.product_service.repository;

import com.ecommerce.product_service.model.Category;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;
import java.util.Optional;

public interface CategoryRepository extends JpaRepository<Category, Long> {

    Optional<Category> findBySlug(String slug);

    boolean existsBySlug(String slug);

    // Root categories (top-level, no parent)
    List<Category> findByParentIsNullOrderBySortOrderAsc();

    // Direct children of a parent
    List<Category> findByParentIdOrderBySortOrderAsc(Long parentId);
}

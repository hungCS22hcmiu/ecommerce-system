package com.ecommerce.product_service.service.serviceImpl;

import com.ecommerce.product_service.dto.CategoryResponse;
import com.ecommerce.product_service.dto.CreateCategoryRequest;
import com.ecommerce.product_service.model.Category;
import com.ecommerce.product_service.repository.CategoryRepository;
import com.ecommerce.product_service.service.CategoryService;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.text.Normalizer;
import java.util.List;

@Service
@Transactional(readOnly = true)
@RequiredArgsConstructor
public class CategoryServiceImpl implements CategoryService {

    private final CategoryRepository categoryRepository;

    @Override
    public List<CategoryResponse> listCategories(String q) {
        List<Category> categories = (q != null && !q.isBlank())
                ? categoryRepository.findByNameContainingIgnoreCaseOrderBySortOrderAsc(q)
                : categoryRepository.findAll();
        return categories.stream().map(this::toResponse).toList();
    }

    @Override
    @Transactional
    public CategoryResponse createCategory(CreateCategoryRequest request) {
        String slug = uniqueSlug(request.getName());
        Category category = Category.builder()
                .name(request.getName())
                .slug(slug)
                .sortOrder(0)
                .build();
        return toResponse(categoryRepository.save(category));
    }

    private CategoryResponse toResponse(Category c) {
        return CategoryResponse.builder()
                .id(c.getId())
                .name(c.getName())
                .slug(c.getSlug())
                .parentId(c.getParent() != null ? c.getParent().getId() : null)
                .sortOrder(c.getSortOrder())
                .build();
    }

    private String uniqueSlug(String name) {
        String base = Normalizer.normalize(name, Normalizer.Form.NFD)
                .replaceAll("\\p{M}", "")
                .toLowerCase()
                .replaceAll("[^a-z0-9]+", "-")
                .replaceAll("^-|-$", "");

        if (!categoryRepository.existsBySlug(base)) return base;

        int n = 2;
        while (categoryRepository.existsBySlug(base + "-" + n)) n++;
        return base + "-" + n;
    }
}

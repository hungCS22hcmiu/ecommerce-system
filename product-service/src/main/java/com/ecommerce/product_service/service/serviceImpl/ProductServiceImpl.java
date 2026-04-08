package com.ecommerce.product_service.service.serviceImpl;

import com.ecommerce.product_service.dto.CreateProductRequest;
import com.ecommerce.product_service.dto.ProductImageRequest;
import com.ecommerce.product_service.dto.ProductImageResponse;
import com.ecommerce.product_service.dto.ProductResponse;
import com.ecommerce.product_service.dto.ProductSummaryResponse;
import com.ecommerce.product_service.dto.UpdateProductRequest;
import com.ecommerce.product_service.exception.ProductAccessDeniedException;
import com.ecommerce.product_service.exception.ProductNotFoundException;
import com.ecommerce.product_service.model.Category;
import com.ecommerce.product_service.model.Product;
import com.ecommerce.product_service.model.ProductImage;
import com.ecommerce.product_service.model.ProductStatus;
import com.ecommerce.product_service.repository.CategoryRepository;
import com.ecommerce.product_service.repository.ProductRepository;
import com.ecommerce.product_service.service.ProductService;
import jakarta.persistence.criteria.Predicate;
import lombok.RequiredArgsConstructor;
import org.springframework.cache.annotation.CacheEvict;
import org.springframework.cache.annotation.CachePut;
import org.springframework.cache.annotation.Cacheable;
import org.springframework.cache.annotation.Caching;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;
import org.springframework.data.jpa.domain.Specification;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.ArrayList;
import java.util.List;
import java.util.UUID;

@Service
@Transactional(readOnly = true)
@RequiredArgsConstructor
public class ProductServiceImpl implements ProductService {

    private final ProductRepository productRepository;
    private final CategoryRepository categoryRepository;

    @Override
    @Transactional
    @CacheEvict(value = "productList", allEntries = true)
    public ProductResponse createProduct(UUID sellerId, CreateProductRequest request) {
        Category category = resolveCategory(request.getCategoryId());

        Product product = Product.builder()
                .name(request.getName())
                .description(request.getDescription())
                .price(request.getPrice())
                .category(category)
                .sellerId(sellerId)
                .status(ProductStatus.ACTIVE)
                .stockQuantity(request.getStockQuantity())
                .stockReserved(0)
                .images(new ArrayList<>())
                .build();

        if (request.getImages() != null) {
            request.getImages().forEach(img -> product.getImages().add(toImageEntity(img, product)));
        }

        return toProductResponse(productRepository.save(product));
    }

    @Override
    @Cacheable(value = "product", key = "#id")
    public ProductResponse getProduct(Long id) {
        Product product = productRepository.findByIdAndStatus(id, ProductStatus.ACTIVE)
                .orElseThrow(() -> new ProductNotFoundException(id));
        return toProductResponse(product);
    }

    @Override
    @Cacheable(value = "productList", key = "{#categoryId, #status, #pageable.pageNumber, #pageable.pageSize, #pageable.sort}")
    public Page<ProductSummaryResponse> listProducts(Long categoryId, ProductStatus status, Pageable pageable) {
        ProductStatus effectiveStatus = status != null ? status : ProductStatus.ACTIVE;

        if (categoryId != null) {
            return productRepository
                    .findByCategoryIdAndStatus(categoryId, effectiveStatus, pageable)
                    .map(this::toSummaryResponse);
        }

        Specification<Product> spec = (root, query, cb) -> {
            List<Predicate> predicates = new ArrayList<>();
            predicates.add(cb.equal(root.get("status"), effectiveStatus));
            return cb.and(predicates.toArray(new Predicate[0]));
        };

        return productRepository.findAll(spec, pageable).map(this::toSummaryResponse);
    }

    @Override
    @Cacheable(value = "productList", key = "{'search', #query, #pageable.pageNumber, #pageable.pageSize}")
    public Page<ProductSummaryResponse> searchProducts(String query, Pageable pageable) {
        return productRepository.searchActive(query, pageable).map(this::toSummaryResponse);
    }

    @Override
    @Transactional
    @Caching(
        put  = @CachePut(value = "product", key = "#id"),
        evict = @CacheEvict(value = "productList", allEntries = true)
    )
    public ProductResponse updateProduct(Long id, UUID sellerId, UpdateProductRequest request) {
        Product product = productRepository.findById(id)
                .orElseThrow(() -> new ProductNotFoundException(id));

        if (!productRepository.existsByIdAndSellerId(id, sellerId)) {
            throw new ProductAccessDeniedException(id);
        }

        if (request.getName() != null) product.setName(request.getName());
        if (request.getDescription() != null) product.setDescription(request.getDescription());
        if (request.getPrice() != null) product.setPrice(request.getPrice());
        if (request.getStatus() != null) product.setStatus(request.getStatus());
        if (request.getStockQuantity() != null) product.setStockQuantity(request.getStockQuantity());

        if (request.getCategoryId() != null) {
            product.setCategory(resolveCategory(request.getCategoryId()));
        }

        if (request.getImages() != null) {
            product.getImages().clear();
            request.getImages().forEach(img -> product.getImages().add(toImageEntity(img, product)));
        }

        return toProductResponse(product);
    }

    @Override
    @Transactional
    @Caching(evict = {
        @CacheEvict(value = "product", key = "#id"),
        @CacheEvict(value = "productList", allEntries = true)
    })
    public void deleteProduct(Long id, UUID sellerId) {
        Product product = productRepository.findById(id)
                .orElseThrow(() -> new ProductNotFoundException(id));

        if (!productRepository.existsByIdAndSellerId(id, sellerId)) {
            throw new ProductAccessDeniedException(id);
        }

        product.setStatus(ProductStatus.DELETED);
    }

    // ── Mapping helpers ──────────────────────────────────────────────────────

    private ProductResponse toProductResponse(Product p) {
        return ProductResponse.builder()
                .id(p.getId())
                .name(p.getName())
                .description(p.getDescription())
                .price(p.getPrice())
                .categoryId(p.getCategory() != null ? p.getCategory().getId() : null)
                .categoryName(p.getCategory() != null ? p.getCategory().getName() : null)
                .sellerId(p.getSellerId())
                .status(p.getStatus())
                .stockQuantity(p.getStockQuantity())
                .stockReserved(p.getStockReserved())
                .stockAvailable(p.getStockQuantity() - p.getStockReserved())
                .version(p.getVersion())
                .images(p.getImages().stream().map(this::toImageResponse).toList())
                .createdAt(p.getCreatedAt())
                .updatedAt(p.getUpdatedAt())
                .build();
    }

    private ProductSummaryResponse toSummaryResponse(Product p) {
        String thumbnail = p.getImages().isEmpty() ? null : p.getImages().get(0).getUrl();
        return ProductSummaryResponse.builder()
                .id(p.getId())
                .name(p.getName())
                .price(p.getPrice())
                .categoryId(p.getCategory() != null ? p.getCategory().getId() : null)
                .categoryName(p.getCategory() != null ? p.getCategory().getName() : null)
                .sellerId(p.getSellerId())
                .status(p.getStatus())
                .stockAvailable(p.getStockQuantity() - p.getStockReserved())
                .thumbnailUrl(thumbnail)
                .createdAt(p.getCreatedAt())
                .build();
    }

    private ProductImageResponse toImageResponse(ProductImage i) {
        return ProductImageResponse.builder()
                .id(i.getId())
                .url(i.getUrl())
                .altText(i.getAltText())
                .sortOrder(i.getSortOrder())
                .build();
    }

    private ProductImage toImageEntity(ProductImageRequest r, Product product) {
        return ProductImage.builder()
                .product(product)
                .url(r.getUrl())
                .altText(r.getAltText())
                .sortOrder(r.getSortOrder())
                .build();
    }

    private Category resolveCategory(Long categoryId) {
        if (categoryId == null) return null;
        return categoryRepository.findById(categoryId)
                .orElse(null);
    }
}

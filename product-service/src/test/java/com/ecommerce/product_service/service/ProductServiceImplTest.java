package com.ecommerce.product_service.service;

import com.ecommerce.product_service.dto.CreateProductRequest;
import com.ecommerce.product_service.dto.ProductImageRequest;
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
import com.ecommerce.product_service.service.serviceImpl.ProductServiceImpl;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Nested;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.PageImpl;
import org.springframework.data.domain.PageRequest;
import org.springframework.data.domain.Pageable;
import org.springframework.data.jpa.domain.Specification;

import java.math.BigDecimal;
import java.time.OffsetDateTime;
import java.util.ArrayList;
import java.util.List;
import java.util.Optional;
import java.util.UUID;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class ProductServiceImplTest {

    @Mock
    private ProductRepository productRepository;

    @Mock
    private CategoryRepository categoryRepository;

    @InjectMocks
    private ProductServiceImpl productService;

    private UUID sellerId;
    private Category category;

    @BeforeEach
    void setUp() {
        sellerId = UUID.randomUUID();
        category = Category.builder()
                .id(1L)
                .name("Electronics")
                .slug("electronics")
                .sortOrder(1)
                .build();
    }

    // ── Helpers ──────────────────────────────────────────────────────────────

    private Product buildProduct(Long id, UUID seller, ProductStatus status) {
        return Product.builder()
                .id(id)
                .name("Test Product")
                .description("A description")
                .price(new BigDecimal("99.99"))
                .category(category)
                .sellerId(seller)
                .status(status)
                .stockQuantity(10)
                .stockReserved(2)
                .version(0L)
                .images(new ArrayList<>())
                .createdAt(OffsetDateTime.now())
                .updatedAt(OffsetDateTime.now())
                .build();
    }

    private CreateProductRequest buildCreateRequest() {
        return CreateProductRequest.builder()
                .name("Test Product")
                .description("A description")
                .price(new BigDecimal("99.99"))
                .stockQuantity(10)
                .build();
    }

    // ── createProduct ─────────────────────────────────────────────────────────

    @Nested
    class CreateProduct {

        @Test
        void savesActiveProductWithStockReservedZero() {
            CreateProductRequest request = buildCreateRequest();
            Product saved = buildProduct(1L, sellerId, ProductStatus.ACTIVE);
            when(productRepository.save(any(Product.class))).thenReturn(saved);

            ProductResponse response = productService.createProduct(sellerId, request);

            ArgumentCaptor<Product> captor = ArgumentCaptor.forClass(Product.class);
            verify(productRepository).save(captor.capture());
            Product persisted = captor.getValue();
            assertThat(persisted.getStatus()).isEqualTo(ProductStatus.ACTIVE);
            assertThat(persisted.getStockReserved()).isZero();
            assertThat(persisted.getSellerId()).isEqualTo(sellerId);
            assertThat(response.getId()).isEqualTo(1L);
        }

        @Test
        void resolvesAndSetsCategory() {
            CreateProductRequest request = CreateProductRequest.builder()
                    .name("Test Product")
                    .description("A description")
                    .price(new BigDecimal("99.99"))
                    .stockQuantity(10)
                    .categoryId(1L)
                    .build();
            when(categoryRepository.findById(1L)).thenReturn(Optional.of(category));
            Product saved = buildProduct(1L, sellerId, ProductStatus.ACTIVE);
            when(productRepository.save(any(Product.class))).thenReturn(saved);

            productService.createProduct(sellerId, request);

            ArgumentCaptor<Product> captor = ArgumentCaptor.forClass(Product.class);
            verify(productRepository).save(captor.capture());
            assertThat(captor.getValue().getCategory()).isEqualTo(category);
        }

        @Test
        void nullCategoryIdResultsInNoCategory() {
            CreateProductRequest request = buildCreateRequest(); // categoryId = null
            Product saved = buildProduct(1L, sellerId, ProductStatus.ACTIVE);
            saved.setCategory(null);
            when(productRepository.save(any(Product.class))).thenReturn(saved);

            ProductResponse response = productService.createProduct(sellerId, request);

            verify(categoryRepository, never()).findById(any());
            assertThat(response.getCategoryId()).isNull();
        }

        @Test
        void addsImagesToProduct() {
            List<ProductImageRequest> imgRequests = List.of(
                    ProductImageRequest.builder().url("http://img.example.com/1.jpg").altText("front").sortOrder(0).build(),
                    ProductImageRequest.builder().url("http://img.example.com/2.jpg").altText("back").sortOrder(1).build()
            );
            CreateProductRequest request = CreateProductRequest.builder()
                    .name("Test Product")
                    .description("A description")
                    .price(new BigDecimal("99.99"))
                    .stockQuantity(10)
                    .images(imgRequests)
                    .build();

            Product saved = buildProduct(1L, sellerId, ProductStatus.ACTIVE);
            saved.setImages(new ArrayList<>(List.of(
                    ProductImage.builder().id(1L).url("http://img.example.com/1.jpg").altText("front").sortOrder(0).build(),
                    ProductImage.builder().id(2L).url("http://img.example.com/2.jpg").altText("back").sortOrder(1).build()
            )));
            when(productRepository.save(any(Product.class))).thenReturn(saved);

            ProductResponse response = productService.createProduct(sellerId, request);

            ArgumentCaptor<Product> captor = ArgumentCaptor.forClass(Product.class);
            verify(productRepository).save(captor.capture());
            assertThat(captor.getValue().getImages()).hasSize(2);
            assertThat(response.getImages()).hasSize(2);
        }
    }

    // ── getProduct ────────────────────────────────────────────────────────────

    @Nested
    class GetProduct {

        @Test
        void returnsProductWhenActiveAndFound() {
            Product product = buildProduct(1L, sellerId, ProductStatus.ACTIVE);
            when(productRepository.findByIdAndStatus(1L, ProductStatus.ACTIVE))
                    .thenReturn(Optional.of(product));

            ProductResponse response = productService.getProduct(1L);

            assertThat(response.getId()).isEqualTo(1L);
            assertThat(response.getName()).isEqualTo("Test Product");
            assertThat(response.getStockAvailable()).isEqualTo(8); // 10 - 2
        }

        @Test
        void throwsProductNotFoundWhenMissing() {
            when(productRepository.findByIdAndStatus(99L, ProductStatus.ACTIVE))
                    .thenReturn(Optional.empty());

            assertThatThrownBy(() -> productService.getProduct(99L))
                    .isInstanceOf(ProductNotFoundException.class);
        }

        @Test
        void throwsProductNotFoundForDeletedProduct() {
            // DELETED products are not returned by findByIdAndStatus(..., ACTIVE)
            when(productRepository.findByIdAndStatus(1L, ProductStatus.ACTIVE))
                    .thenReturn(Optional.empty());

            assertThatThrownBy(() -> productService.getProduct(1L))
                    .isInstanceOf(ProductNotFoundException.class);
        }
    }

    // ── listProducts ──────────────────────────────────────────────────────────

    @Nested
    class ListProducts {

        private final Pageable pageable = PageRequest.of(0, 20);

        @Test
        void usesCategoryFilterWhenCategoryIdProvided() {
            Product product = buildProduct(1L, sellerId, ProductStatus.ACTIVE);
            Page<Product> page = new PageImpl<>(List.of(product));
            when(productRepository.findByCategoryIdAndStatus(1L, ProductStatus.ACTIVE, pageable))
                    .thenReturn(page);

            Page<ProductSummaryResponse> result = productService.listProducts(1L, null, pageable);

            assertThat(result.getContent()).hasSize(1);
            verify(productRepository).findByCategoryIdAndStatus(1L, ProductStatus.ACTIVE, pageable);
            verify(productRepository, never()).findAll(any(Specification.class), eq(pageable));
        }

        @Test
        void usesSpecificationWhenNoCategoryId() {
            Product product = buildProduct(1L, sellerId, ProductStatus.ACTIVE);
            Page<Product> page = new PageImpl<>(List.of(product));
            when(productRepository.findAll(any(Specification.class), eq(pageable))).thenReturn(page);

            Page<ProductSummaryResponse> result = productService.listProducts(null, null, pageable);

            assertThat(result.getContent()).hasSize(1);
            verify(productRepository).findAll(any(Specification.class), eq(pageable));
        }

        @Test
        void defaultsStatusToActiveWhenNull() {
            Page<Product> emptyPage = Page.empty();
            when(productRepository.findAll(any(Specification.class), eq(pageable))).thenReturn(emptyPage);

            productService.listProducts(null, null, pageable);

            // The spec is built with ACTIVE internally — verified indirectly by checking
            // no exception thrown and findAll was called (not findByCategoryIdAndStatus with null)
            verify(productRepository).findAll(any(Specification.class), eq(pageable));
        }

        @Test
        void respectsExplicitStatusFilter() {
            Page<Product> emptyPage = Page.empty();
            when(productRepository.findAll(any(Specification.class), eq(pageable))).thenReturn(emptyPage);

            productService.listProducts(null, ProductStatus.INACTIVE, pageable);

            verify(productRepository).findAll(any(Specification.class), eq(pageable));
        }

        @Test
        void summaryIncludesThumbnailFromFirstImage() {
            Product product = buildProduct(1L, sellerId, ProductStatus.ACTIVE);
            product.getImages().add(ProductImage.builder()
                    .id(1L).url("http://img.example.com/thumb.jpg").sortOrder(0).build());
            Page<Product> page = new PageImpl<>(List.of(product));
            when(productRepository.findAll(any(Specification.class), eq(pageable))).thenReturn(page);

            Page<ProductSummaryResponse> result = productService.listProducts(null, null, pageable);

            assertThat(result.getContent().get(0).getThumbnailUrl()).isEqualTo("http://img.example.com/thumb.jpg");
        }

        @Test
        void thumbnailIsNullWhenNoImages() {
            Product product = buildProduct(1L, sellerId, ProductStatus.ACTIVE);
            Page<Product> page = new PageImpl<>(List.of(product));
            when(productRepository.findAll(any(Specification.class), eq(pageable))).thenReturn(page);

            Page<ProductSummaryResponse> result = productService.listProducts(null, null, pageable);

            assertThat(result.getContent().get(0).getThumbnailUrl()).isNull();
        }

        @Test
        void emptyPageHasZeroTotals() {
            Page<Product> emptyPage = new PageImpl<>(List.of(), pageable, 0);
            when(productRepository.findAll(any(Specification.class), eq(pageable))).thenReturn(emptyPage);

            Page<ProductSummaryResponse> result = productService.listProducts(null, null, pageable);

            assertThat(result.getContent()).isEmpty();
            assertThat(result.getTotalElements()).isZero();
            assertThat(result.getTotalPages()).isZero();
        }

        @Test
        void lastPageIsEmptyButTotalElementsIsPreserved() {
            // 85 total products, page size 20 → last page (index 4) has 5 items;
            // requesting page 5 (index 5) is beyond last → empty content, totalElements still 85
            Pageable lastPage = PageRequest.of(5, 20);
            Page<Product> beyondLastPage = new PageImpl<>(List.of(), lastPage, 85);
            when(productRepository.findAll(any(Specification.class), eq(lastPage))).thenReturn(beyondLastPage);

            Page<ProductSummaryResponse> result = productService.listProducts(null, null, lastPage);

            assertThat(result.getContent()).isEmpty();
            assertThat(result.getTotalElements()).isEqualTo(85);
            assertThat(result.getTotalPages()).isEqualTo(5); // ceil(85/20)
            assertThat(result.getNumber()).isEqualTo(5);
        }

        @Test
        void paginationMetadataPassesThroughCorrectly() {
            Pageable page2 = PageRequest.of(1, 5);
            List<Product> products = List.of(
                    buildProduct(6L, sellerId, ProductStatus.ACTIVE),
                    buildProduct(7L, sellerId, ProductStatus.ACTIVE),
                    buildProduct(8L, sellerId, ProductStatus.ACTIVE),
                    buildProduct(9L, sellerId, ProductStatus.ACTIVE),
                    buildProduct(10L, sellerId, ProductStatus.ACTIVE)
            );
            Page<Product> repoPage = new PageImpl<>(products, page2, 23);
            when(productRepository.findAll(any(Specification.class), eq(page2))).thenReturn(repoPage);

            Page<ProductSummaryResponse> result = productService.listProducts(null, null, page2);

            assertThat(result.getContent()).hasSize(5);
            assertThat(result.getNumber()).isEqualTo(1);
            assertThat(result.getSize()).isEqualTo(5);
            assertThat(result.getTotalElements()).isEqualTo(23);
            assertThat(result.getTotalPages()).isEqualTo(5); // ceil(23/5)
        }
    }

    // ── searchProducts ────────────────────────────────────────────────────────

    @Nested
    class SearchProducts {

        @Test
        void delegatesToRepositorySearchActive() {
            Pageable pageable = PageRequest.of(0, 20);
            Product product = buildProduct(1L, sellerId, ProductStatus.ACTIVE);
            Page<Product> page = new PageImpl<>(List.of(product));
            when(productRepository.searchActive("laptop", pageable)).thenReturn(page);

            Page<ProductSummaryResponse> result = productService.searchProducts("laptop", pageable);

            assertThat(result.getContent()).hasSize(1);
            verify(productRepository).searchActive("laptop", pageable);
        }

        @Test
        void returnsEmptyPageWhenNoMatches() {
            Pageable pageable = PageRequest.of(0, 20);
            Page<Product> emptyPage = new PageImpl<>(List.of(), pageable, 0);
            when(productRepository.searchActive("xyz_no_match", pageable)).thenReturn(emptyPage);

            Page<ProductSummaryResponse> result = productService.searchProducts("xyz_no_match", pageable);

            assertThat(result.getContent()).isEmpty();
            assertThat(result.getTotalElements()).isZero();
            assertThat(result.getTotalPages()).isZero();
        }

        @Test
        void searchLastPageIsEmptyButTotalElementsPreserved() {
            Pageable lastPage = PageRequest.of(3, 10);
            Page<Product> beyondLast = new PageImpl<>(List.of(), lastPage, 27);
            when(productRepository.searchActive("shoe", lastPage)).thenReturn(beyondLast);

            Page<ProductSummaryResponse> result = productService.searchProducts("shoe", lastPage);

            assertThat(result.getContent()).isEmpty();
            assertThat(result.getTotalElements()).isEqualTo(27);
            assertThat(result.getTotalPages()).isEqualTo(3); // ceil(27/10)
            assertThat(result.getNumber()).isEqualTo(3);
        }

        @Test
        void searchPaginationMetadataPassesThroughCorrectly() {
            Pageable page1 = PageRequest.of(1, 10);
            List<Product> products = List.of(
                    buildProduct(11L, sellerId, ProductStatus.ACTIVE),
                    buildProduct(12L, sellerId, ProductStatus.ACTIVE)
            );
            Page<Product> repoPage = new PageImpl<>(products, page1, 12);
            when(productRepository.searchActive("laptop", page1)).thenReturn(repoPage);

            Page<ProductSummaryResponse> result = productService.searchProducts("laptop", page1);

            assertThat(result.getContent()).hasSize(2);
            assertThat(result.getNumber()).isEqualTo(1);
            assertThat(result.getSize()).isEqualTo(10);
            assertThat(result.getTotalElements()).isEqualTo(12);
            assertThat(result.getTotalPages()).isEqualTo(2);
        }
    }

    // ── updateProduct ─────────────────────────────────────────────────────────

    @Nested
    class UpdateProduct {

        @Test
        void throwsProductNotFoundWhenProductMissing() {
            when(productRepository.findById(99L)).thenReturn(Optional.empty());

            assertThatThrownBy(() -> productService.updateProduct(99L, sellerId,
                    UpdateProductRequest.builder().name("New").build()))
                    .isInstanceOf(ProductNotFoundException.class);
        }

        @Test
        void throwsAccessDeniedWhenSellerDoesNotOwnProduct() {
            Product product = buildProduct(1L, sellerId, ProductStatus.ACTIVE);
            UUID otherSeller = UUID.randomUUID();
            when(productRepository.findById(1L)).thenReturn(Optional.of(product));
            when(productRepository.existsByIdAndSellerId(1L, otherSeller)).thenReturn(false);

            assertThatThrownBy(() -> productService.updateProduct(1L, otherSeller,
                    UpdateProductRequest.builder().name("New").build()))
                    .isInstanceOf(ProductAccessDeniedException.class);
        }

        @Test
        void updatesOnlyProvidedFields() {
            Product product = buildProduct(1L, sellerId, ProductStatus.ACTIVE);
            when(productRepository.findById(1L)).thenReturn(Optional.of(product));
            when(productRepository.existsByIdAndSellerId(1L, sellerId)).thenReturn(true);

            UpdateProductRequest request = UpdateProductRequest.builder()
                    .name("Updated Name")
                    .build();

            ProductResponse response = productService.updateProduct(1L, sellerId, request);

            assertThat(product.getName()).isEqualTo("Updated Name");
            assertThat(product.getDescription()).isEqualTo("A description"); // unchanged
            assertThat(product.getPrice()).isEqualByComparingTo("99.99");    // unchanged
        }

        @Test
        void updatesAllFieldsWhenAllProvided() {
            Product product = buildProduct(1L, sellerId, ProductStatus.ACTIVE);
            Category newCategory = Category.builder().id(2L).name("Clothing").slug("clothing").sortOrder(2).build();
            when(productRepository.findById(1L)).thenReturn(Optional.of(product));
            when(productRepository.existsByIdAndSellerId(1L, sellerId)).thenReturn(true);
            when(categoryRepository.findById(2L)).thenReturn(Optional.of(newCategory));

            UpdateProductRequest request = UpdateProductRequest.builder()
                    .name("New Name")
                    .description("New desc")
                    .price(new BigDecimal("49.99"))
                    .categoryId(2L)
                    .status(ProductStatus.INACTIVE)
                    .stockQuantity(5)
                    .build();

            productService.updateProduct(1L, sellerId, request);

            assertThat(product.getName()).isEqualTo("New Name");
            assertThat(product.getDescription()).isEqualTo("New desc");
            assertThat(product.getPrice()).isEqualByComparingTo("49.99");
            assertThat(product.getCategory()).isEqualTo(newCategory);
            assertThat(product.getStatus()).isEqualTo(ProductStatus.INACTIVE);
            assertThat(product.getStockQuantity()).isEqualTo(5);
        }

        @Test
        void replacesImagesWhenImagesProvided() {
            Product product = buildProduct(1L, sellerId, ProductStatus.ACTIVE);
            product.getImages().add(ProductImage.builder().id(1L).url("http://old.jpg").sortOrder(0).build());
            when(productRepository.findById(1L)).thenReturn(Optional.of(product));
            when(productRepository.existsByIdAndSellerId(1L, sellerId)).thenReturn(true);

            UpdateProductRequest request = UpdateProductRequest.builder()
                    .images(List.of(
                            ProductImageRequest.builder().url("http://new.jpg").sortOrder(0).build()
                    ))
                    .build();

            productService.updateProduct(1L, sellerId, request);

            assertThat(product.getImages()).hasSize(1);
            assertThat(product.getImages().get(0).getUrl()).isEqualTo("http://new.jpg");
        }

        @Test
        void keepsImagesUnchangedWhenImagesNotProvided() {
            Product product = buildProduct(1L, sellerId, ProductStatus.ACTIVE);
            product.getImages().add(ProductImage.builder().id(1L).url("http://existing.jpg").sortOrder(0).build());
            when(productRepository.findById(1L)).thenReturn(Optional.of(product));
            when(productRepository.existsByIdAndSellerId(1L, sellerId)).thenReturn(true);

            UpdateProductRequest request = UpdateProductRequest.builder()
                    .name("Only name update")
                    .build();

            productService.updateProduct(1L, sellerId, request);

            assertThat(product.getImages()).hasSize(1);
            assertThat(product.getImages().get(0).getUrl()).isEqualTo("http://existing.jpg");
        }
    }

    // ── deleteProduct ─────────────────────────────────────────────────────────

    @Nested
    class DeleteProduct {

        @Test
        void throwsProductNotFoundWhenProductMissing() {
            when(productRepository.findById(99L)).thenReturn(Optional.empty());

            assertThatThrownBy(() -> productService.deleteProduct(99L, sellerId))
                    .isInstanceOf(ProductNotFoundException.class);
        }

        @Test
        void throwsAccessDeniedWhenSellerDoesNotOwnProduct() {
            Product product = buildProduct(1L, sellerId, ProductStatus.ACTIVE);
            UUID otherSeller = UUID.randomUUID();
            when(productRepository.findById(1L)).thenReturn(Optional.of(product));
            when(productRepository.existsByIdAndSellerId(1L, otherSeller)).thenReturn(false);

            assertThatThrownBy(() -> productService.deleteProduct(1L, otherSeller))
                    .isInstanceOf(ProductAccessDeniedException.class);
        }

        @Test
        void setsStatusToDeletedOnSoftDelete() {
            Product product = buildProduct(1L, sellerId, ProductStatus.ACTIVE);
            when(productRepository.findById(1L)).thenReturn(Optional.of(product));
            when(productRepository.existsByIdAndSellerId(1L, sellerId)).thenReturn(true);

            productService.deleteProduct(1L, sellerId);

            assertThat(product.getStatus()).isEqualTo(ProductStatus.DELETED);
        }

        @Test
        void doesNotHardDeleteProduct() {
            Product product = buildProduct(1L, sellerId, ProductStatus.ACTIVE);
            when(productRepository.findById(1L)).thenReturn(Optional.of(product));
            when(productRepository.existsByIdAndSellerId(1L, sellerId)).thenReturn(true);

            productService.deleteProduct(1L, sellerId);

            verify(productRepository, never()).delete(any(Product.class));
            verify(productRepository, never()).deleteById(any());
        }
    }
}

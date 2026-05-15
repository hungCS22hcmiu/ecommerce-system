package com.ecommerce.product_service.service;

import com.ecommerce.product_service.client.EmbeddingClient;
import com.ecommerce.product_service.dto.AISearchResponse;
import com.ecommerce.product_service.exception.AIServiceException;
import com.ecommerce.product_service.model.Category;
import com.ecommerce.product_service.model.Product;
import com.ecommerce.product_service.model.ProductStatus;
import com.ecommerce.product_service.repository.ProductRepository;
import com.ecommerce.product_service.service.serviceImpl.AISearchServiceImpl;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Nested;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.springframework.jdbc.core.JdbcTemplate;

import java.math.BigDecimal;
import java.time.OffsetDateTime;
import java.util.ArrayList;
import java.util.List;
import java.util.UUID;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.*;
import static org.mockito.Mockito.*;

@ExtendWith(MockitoExtension.class)
class AISearchServiceTest {

    @Mock
    EmbeddingClient embeddingClient;

    @Mock
    ProductRepository productRepository;

    @Mock
    JdbcTemplate jdbcTemplate;

    @InjectMocks
    AISearchServiceImpl service;

    private static final float[] UNIT_VEC = buildUnitVec(0);
    private static final String QUERY = "comfortable running shoes";
    private static final int LIMIT = 5;

    @BeforeEach
    void setUp() {
        lenient().when(embeddingClient.embed(eq(QUERY), eq(QUERY), eq(LIMIT))).thenReturn(UNIT_VEC);
        // jdbcTemplate.execute(String) returns void — no stub needed; Mockito ignores void calls by default
    }

    @Nested
    class HappyPath {

        @Test
        void returnsAiModeOnSuccess() {
            List<Object[]> rows = new ArrayList<>();
            rows.add(new Object[]{1L, 0.95});
            when(productRepository.findIdsBySemanticSimilarity(anyString(), eq(LIMIT)))
                    .thenReturn(rows);
            when(productRepository.findAllById(List.of(1L)))
                    .thenReturn(List.of(buildProduct(1L)));

            AISearchResponse resp = service.search(QUERY, LIMIT);

            assertThat(resp.mode()).isEqualTo("ai");
            assertThat(resp.query()).isEqualTo(QUERY);
            assertThat(resp.results()).hasSize(1);
            assertThat(resp.scores()).containsExactly(0.95);
        }

        @Test
        void preservesRankingOrder() {
            List<Object[]> rows = new ArrayList<>();
            rows.add(new Object[]{10L, 0.9});
            rows.add(new Object[]{20L, 0.7});
            when(productRepository.findIdsBySemanticSimilarity(anyString(), eq(LIMIT)))
                    .thenReturn(rows);
            when(productRepository.findAllById(List.of(10L, 20L)))
                    .thenReturn(List.of(buildProduct(20L), buildProduct(10L)));

            AISearchResponse resp = service.search(QUERY, LIMIT);

            assertThat(resp.results().get(0).getId()).isEqualTo(10L);
            assertThat(resp.results().get(1).getId()).isEqualTo(20L);
        }

        @Test
        void emptyResultsWhenNothingEmbedded() {
            when(productRepository.findIdsBySemanticSimilarity(anyString(), anyInt()))
                    .thenReturn(new ArrayList<>());
            when(productRepository.findAllById(anyList())).thenReturn(List.of());

            AISearchResponse resp = service.search(QUERY, LIMIT);

            assertThat(resp.results()).isEmpty();
            assertThat(resp.scores()).isEmpty();
        }
    }

    @Nested
    class Validation {

        @Test
        void throwsOnNullQuery() {
            assertThatThrownBy(() -> service.search(null, LIMIT))
                    .isInstanceOf(IllegalArgumentException.class)
                    .hasMessageContaining("2 characters");
        }

        @Test
        void throwsOnShortQuery() {
            assertThatThrownBy(() -> service.search("x", LIMIT))
                    .isInstanceOf(IllegalArgumentException.class);
        }

        @Test
        void acceptsTwoCharQuery() {
            when(productRepository.findIdsBySemanticSimilarity(anyString(), anyInt()))
                    .thenReturn(new ArrayList<>());
            when(productRepository.findAllById(anyList())).thenReturn(List.of());
            when(embeddingClient.embed(eq("ok"), eq("ok"), eq(LIMIT))).thenReturn(UNIT_VEC);

            AISearchResponse resp = service.search("ok", LIMIT);
            assertThat(resp.mode()).isEqualTo("ai");
        }
    }

    @Nested
    class Fallback {

        @Test
        void propagatesAIServiceExceptionForFallback() {
            when(embeddingClient.embed(eq(QUERY), eq(QUERY), eq(LIMIT)))
                    .thenThrow(new AIServiceException("timeout", null, QUERY, LIMIT));

            assertThatThrownBy(() -> service.search(QUERY, LIMIT))
                    .isInstanceOf(AIServiceException.class)
                    .extracting("query").isEqualTo(QUERY);
        }
    }

    // ── helpers ────────────────────────────────────────────────────────────────

    private static float[] buildUnitVec(int hotDim) {
        float[] v = new float[384];
        v[hotDim] = 1.0f;
        return v;
    }

    private Product buildProduct(Long id) {
        Category cat = new Category();
        cat.setId(1L);
        cat.setName("Footwear");

        Product p = Product.builder()
                .id(id)
                .name("Product " + id)
                .price(BigDecimal.valueOf(99.99))
                .category(cat)
                .sellerId(UUID.randomUUID())
                .status(ProductStatus.ACTIVE)
                .stockQuantity(10)
                .stockReserved(0)
                .images(new ArrayList<>())
                .createdAt(OffsetDateTime.now())
                .updatedAt(OffsetDateTime.now())
                .build();
        return p;
    }
}

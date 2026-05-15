package com.ecommerce.product_service.service.serviceImpl;

import com.ecommerce.product_service.client.EmbeddingClient;
import com.ecommerce.product_service.dto.AISearchResponse;
import com.ecommerce.product_service.dto.ProductSummaryResponse;
import com.ecommerce.product_service.model.Product;
import com.ecommerce.product_service.repository.ProductRepository;
import com.ecommerce.product_service.service.AISearchService;
import lombok.RequiredArgsConstructor;
import org.springframework.cache.annotation.Cacheable;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.StringJoiner;
import java.util.stream.Collectors;

@Service
@Transactional(readOnly = true)
@RequiredArgsConstructor
public class AISearchServiceImpl implements AISearchService {

    private final EmbeddingClient embeddingClient;
    private final ProductRepository productRepository;
    private final JdbcTemplate jdbcTemplate;

    @Override
    @Cacheable(value = "aiSearch", key = "{#query, #limit}")
    public AISearchResponse search(String query, int limit) {
        if (query == null || query.trim().length() < 2) {
            throw new IllegalArgumentException("Query must be at least 2 characters");
        }

        // Throws AIServiceException on failure — exception propagates before cache write,
        // so fallback results are never stored in "aiSearch" cache.
        float[] vector = embeddingClient.embed(query, query, limit);
        String vectorLiteral = toPgvectorLiteral(vector);

        // Set nprobe before the IVFFLAT scan; must be a separate statement — pgvector
        // reads the setting at access-method init, before any CTE or WHERE eval.
        jdbcTemplate.execute("SET LOCAL ivfflat.probes = 10");

        List<Object[]> rows = productRepository.findIdsBySemanticSimilarity(vectorLiteral, limit);
        List<Long> ids = rows.stream().map(r -> ((Number) r[0]).longValue()).toList();
        List<Double> scores = rows.stream().map(r -> ((Number) r[1]).doubleValue()).toList();

        Map<Long, Product> byId = productRepository.findAllById(ids).stream()
                .collect(Collectors.toMap(Product::getId, p -> p));

        List<ProductSummaryResponse> results = ids.stream()
                .map(byId::get)
                .filter(Objects::nonNull)
                .map(this::toSummaryResponse)
                .toList();

        return new AISearchResponse(query, results, scores, "ai");
    }

    private String toPgvectorLiteral(float[] vector) {
        StringJoiner sj = new StringJoiner(",", "[", "]");
        for (float v : vector) sj.add(Float.toString(v));
        return sj.toString();
    }

    private ProductSummaryResponse toSummaryResponse(Product p) {
        String thumbnail = (p.getImages() == null || p.getImages().isEmpty())
                ? null : p.getImages().get(0).getUrl();
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
}

package com.ecommerce.product_service.service.serviceImpl;

import com.ecommerce.product_service.client.EmbeddingClient;
import com.ecommerce.product_service.dto.AISearchResponse;
import com.ecommerce.product_service.dto.ProductSummaryResponse;
import com.ecommerce.product_service.model.Product;
import com.ecommerce.product_service.repository.ProductRepository;
import com.ecommerce.product_service.service.AISearchService;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.cache.annotation.Cacheable;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.Comparator;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.StringJoiner;
import java.util.UUID;
import java.util.stream.Collectors;
import java.util.stream.IntStream;

@Service
@Slf4j
@Transactional(readOnly = true)
@RequiredArgsConstructor
public class AISearchServiceImpl implements AISearchService {

    private final EmbeddingClient embeddingClient;
    private final ProductRepository productRepository;
    private final JdbcTemplate jdbcTemplate;

    @Override
    @Cacheable(value = "aiSearch", key = "{#query, #limit, #categoryId, #sellerId}",
               unless = "#result.results().isEmpty()")
    public AISearchResponse search(String query, int limit, Long categoryId, UUID sellerId) {
        if (query == null || query.trim().length() < 2) {
            throw new IllegalArgumentException("Query must be at least 2 characters");
        }

        // Phase 2 instrumentation — measure embed / vector / rerank in nanoseconds
        long t0 = System.nanoTime();

        // Throws AIServiceException on failure — exception propagates before cache write,
        // so fallback results are never stored in "aiSearch" cache.
        float[] vector = embeddingClient.embed(query, query, limit);
        String vectorLiteral = toPgvectorLiteral(vector);
        long tEmbed = System.nanoTime();

        // Set nprobe before the IVFFLAT scan; must be a separate statement — pgvector
        // reads the setting at access-method init, before any CTE or WHERE eval.
        jdbcTemplate.execute("SET LOCAL ivfflat.probes = 10");

        List<Object[]> rows = (sellerId != null)
                ? productRepository.findIdsBySemanticSimilarityBySeller(vectorLiteral, sellerId, categoryId, limit)
                : productRepository.findIdsBySemanticSimilarity(vectorLiteral, categoryId, limit);
        List<Long> ids = rows.stream().map(r -> ((Number) r[0]).longValue()).toList();
        List<Double> scores = rows.stream().map(r -> ((Number) r[1]).doubleValue()).toList();
        long tVector = System.nanoTime();

        Map<Long, Product> byId = productRepository.findAllById(ids).stream()
                .collect(Collectors.toMap(Product::getId, p -> p));

        // Build similarity-score lookup by product id
        Map<Long, Double> simScoreMap = new HashMap<>();
        IntStream.range(0, ids.size()).forEach(i -> simScoreMap.put(ids.get(i), scores.get(i)));

        double maxReserved = Math.max(1.0, byId.values().stream()
                .mapToInt(Product::getStockReserved).max().orElse(1));

        // Re-rank: similarity 75% + rating boost 15% + sales boost 10%
        List<ProductSummaryResponse> results = byId.values().stream()
                .filter(p -> simScoreMap.containsKey(p.getId()))
                .sorted(Comparator.comparingDouble((Product p) -> {
                    double sim = simScoreMap.getOrDefault(p.getId(), 0.0);
                    double ratingBoost = p.getAvgRating() != null
                            ? p.getAvgRating().doubleValue() / 5.0 * 0.15 : 0.0;
                    double salesBoost = p.getStockReserved() / maxReserved * 0.10;
                    return -(sim * 0.75 + ratingBoost + salesBoost);
                }))
                .map(this::toSummaryResponse)
                .toList();
        long tRerank = System.nanoTime();

        log.info("ai.search.layer query='{}' embed_ms={} vector_ms={} rerank_ms={} results={}",
                query,
                (tEmbed - t0) / 1_000_000,
                (tVector - tEmbed) / 1_000_000,
                (tRerank - tVector) / 1_000_000,
                results.size());

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
                .stockReserved(p.getStockReserved())
                .thumbnailUrl(thumbnail)
                .avgRating(p.getAvgRating() != null ? p.getAvgRating().doubleValue() : null)
                .ratingCount(p.getRatingCount())
                .createdAt(p.getCreatedAt())
                .build();
    }
}

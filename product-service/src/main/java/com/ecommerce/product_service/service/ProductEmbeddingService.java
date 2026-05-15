package com.ecommerce.product_service.service;

import com.ecommerce.product_service.client.EmbeddingClient;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.scheduling.annotation.Async;
import org.springframework.stereotype.Service;

import java.util.StringJoiner;

@Slf4j
@Service
@RequiredArgsConstructor
public class ProductEmbeddingService {

    private final EmbeddingClient embeddingClient;
    private final JdbcTemplate jdbcTemplate;

    @Value("${ai-service.write-through-enabled:true}")
    private boolean writeThroughEnabled;

    @Async("taskExecutor")
    public void scheduleEmbedding(Long productId, String name, String description, String categoryName) {
        if (!writeThroughEnabled) return;
        try {
            String text = buildText(name, description, categoryName);
            float[] vector = embeddingClient.embed(text, text, 0);
            String literal = toPgvectorLiteral(vector);
            jdbcTemplate.update(
                    "UPDATE products SET embedding = CAST(? AS vector) WHERE id = ?",
                    literal, productId
            );
            log.debug("Write-through embedding updated for product {}", productId);
        } catch (Exception e) {
            log.warn("Write-through embedding failed for product {}: {}", productId, e.getMessage());
        }
    }

    private String buildText(String name, String description, String categoryName) {
        StringBuilder sb = new StringBuilder(name);
        if (description != null && !description.isBlank()) sb.append(' ').append(description);
        if (categoryName != null && !categoryName.isBlank()) sb.append(' ').append(categoryName);
        return sb.toString();
    }

    private String toPgvectorLiteral(float[] vector) {
        StringJoiner sj = new StringJoiner(",", "[", "]");
        for (float v : vector) sj.add(Float.toString(v));
        return sj.toString();
    }
}

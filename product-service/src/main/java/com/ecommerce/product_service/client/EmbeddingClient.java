package com.ecommerce.product_service.client;

import com.ecommerce.product_service.exception.AIServiceException;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Component;
import org.springframework.web.reactive.function.client.WebClient;

import java.util.List;
import java.util.Map;

@Component
@RequiredArgsConstructor
public class EmbeddingClient {

    private final WebClient aiServiceWebClient;

    public float[] embed(String text, String query, int limit) {
        try {
            EmbedResponse response = aiServiceWebClient.post()
                    .uri("/embed")
                    .bodyValue(Map.of("text", text))
                    .retrieve()
                    .bodyToMono(EmbedResponse.class)
                    .block();
            if (response == null || response.embedding() == null) {
                throw new AIServiceException("Empty embedding response", null, query, limit);
            }
            List<Float> vec = response.embedding();
            float[] result = new float[vec.size()];
            for (int i = 0; i < vec.size(); i++) result[i] = vec.get(i);
            return result;
        } catch (AIServiceException e) {
            throw e;
        } catch (Exception e) {
            throw new AIServiceException("ai-service unavailable: " + e.getMessage(), e, query, limit);
        }
    }

    private record EmbedResponse(List<Float> embedding) {}
}

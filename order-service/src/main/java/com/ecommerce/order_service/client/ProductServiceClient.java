package com.ecommerce.order_service.client;

import com.ecommerce.order_service.exception.InsufficientStockException;
import lombok.*;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Component;
import org.springframework.web.client.HttpClientErrorException;
import org.springframework.web.client.RestTemplate;

import java.math.BigDecimal;

@Slf4j
@Component
@RequiredArgsConstructor
public class ProductServiceClient {

    private final RestTemplate restTemplate;

    @Value("${product-service.url}")
    private String productServiceUrl;

    public StockResponse reserveStock(Long productId, int quantity, String referenceId) {
        String url = productServiceUrl + "/api/v1/inventory/" + productId + "/reserve";
        StockRequest request = new StockRequest(quantity, referenceId);
        try {
            return restTemplate.postForObject(url, request, StockResponse.class);
        } catch (HttpClientErrorException e) {
            if (e.getStatusCode() == HttpStatus.CONFLICT) {
                throw new InsufficientStockException(productId);
            }
            log.error("Failed to reserve stock for productId={}, status={}", productId, e.getStatusCode());
            throw new IllegalStateException("Failed to reserve stock for product " + productId + ": " + e.getMessage());
        }
    }

    public StockResponse releaseStock(Long productId, int quantity, String referenceId) {
        String url = productServiceUrl + "/api/v1/inventory/" + productId + "/release";
        StockRequest request = new StockRequest(quantity, referenceId);
        try {
            return restTemplate.postForObject(url, request, StockResponse.class);
        } catch (HttpClientErrorException e) {
            log.error("Failed to release stock for productId={}, status={}", productId, e.getStatusCode());
            throw new IllegalStateException("Failed to release stock for product " + productId + ": " + e.getMessage());
        }
    }

    public ProductDetail getProduct(Long productId) {
        String url = productServiceUrl + "/api/v1/products/" + productId;
        try {
            ProductApiResponse resp = restTemplate.getForObject(url, ProductApiResponse.class);
            if (resp == null || !resp.isSuccess() || resp.getData() == null) {
                throw new IllegalStateException("Product not found: " + productId);
            }
            return resp.getData();
        } catch (HttpClientErrorException e) {
            if (e.getStatusCode() == HttpStatus.NOT_FOUND) {
                throw new IllegalStateException("Product not found: " + productId);
            }
            log.error("Failed to fetch product productId={}, status={}", productId, e.getStatusCode());
            throw new IllegalStateException("Failed to fetch product " + productId + ": " + e.getMessage());
        }
    }

    @Data
    @NoArgsConstructor
    @AllArgsConstructor
    public static class ProductDetail {
        private String name;
        private BigDecimal price;
    }

    @Data
    @NoArgsConstructor
    @AllArgsConstructor
    public static class ProductApiResponse {
        private boolean success;
        private ProductDetail data;
    }

    @Data
    @NoArgsConstructor
    @AllArgsConstructor
    public static class StockRequest {
        private int quantity;
        private String referenceId;
    }

    @Data
    @NoArgsConstructor
    @AllArgsConstructor
    public static class StockResponse {
        private Long productId;
        private int available;
        private int reserved;
    }
}

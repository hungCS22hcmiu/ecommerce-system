package com.ecommerce.product_service.client;

import com.ecommerce.product_service.exception.PurchaseVerificationException;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.*;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestTemplate;

import java.util.Map;
import java.util.UUID;

@Slf4j
@Component
@RequiredArgsConstructor
public class OrderServiceClient {

    @Value("${ORDER_SERVICE_URL:http://order-service:8082}")
    private String orderServiceUrl;

    private final RestTemplate restTemplate;

    @SuppressWarnings("unchecked")
    public boolean verifyPurchase(UUID userId, Long productId, UUID orderItemId) {
        String url = orderServiceUrl + "/api/v1/orders/purchase-verification"
                + "?productId=" + productId + "&orderItemId=" + orderItemId;
        HttpHeaders headers = new HttpHeaders();
        headers.set("X-User-Id", userId.toString());
        try {
            ResponseEntity<Map> resp = restTemplate.exchange(
                    url, HttpMethod.GET, new HttpEntity<>(headers), Map.class);
            if (resp.getBody() == null) return false;
            Map<?, ?> data = (Map<?, ?>) resp.getBody().get("data");
            if (data == null) return false;
            return Boolean.TRUE.equals(data.get("verified"));
        } catch (Exception e) {
            log.error("Purchase verification failed for userId={} productId={} orderItemId={}: {}",
                    userId, productId, orderItemId, e.getMessage());
            throw new PurchaseVerificationException("Could not verify purchase with order service");
        }
    }
}

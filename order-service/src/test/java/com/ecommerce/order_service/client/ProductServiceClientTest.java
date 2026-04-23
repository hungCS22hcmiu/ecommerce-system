package com.ecommerce.order_service.client;

import com.ecommerce.order_service.exception.InsufficientStockException;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Nested;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.springframework.http.HttpStatus;
import org.springframework.test.util.ReflectionTestUtils;
import org.springframework.web.client.HttpClientErrorException;
import org.springframework.web.client.RestTemplate;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.*;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class ProductServiceClientTest {

    @Mock  private RestTemplate restTemplate;
    @InjectMocks private ProductServiceClient client;

    @BeforeEach
    void setUp() {
        ReflectionTestUtils.setField(client, "productServiceUrl", "http://product-service:8081");
    }

    // ── ReserveStock ──────────────────────────────────────────────────────────

    @Nested
    class ReserveStock {

        @Test
        void happyPath_returnsStockResponse() {
            ProductServiceClient.StockResponse expected =
                    new ProductServiceClient.StockResponse(1L, 8, 2);

            when(restTemplate.postForObject(
                    eq("http://product-service:8081/api/v1/inventory/1/reserve"),
                    any(ProductServiceClient.StockRequest.class),
                    eq(ProductServiceClient.StockResponse.class)))
                    .thenReturn(expected);

            ProductServiceClient.StockResponse result = client.reserveStock(1L, 2, "order-ref");

            assertThat(result.getProductId()).isEqualTo(1L);
            assertThat(result.getAvailable()).isEqualTo(8);
            assertThat(result.getReserved()).isEqualTo(2);
        }

        @Test
        void conflict409_throwsInsufficientStockException() {
            when(restTemplate.postForObject(anyString(), any(), eq(ProductServiceClient.StockResponse.class)))
                    .thenThrow(HttpClientErrorException.create(
                            HttpStatus.CONFLICT, "Conflict", null, null, null));

            assertThatThrownBy(() -> client.reserveStock(1L, 5, "order-ref"))
                    .isInstanceOf(InsufficientStockException.class)
                    .hasMessageContaining("1");
        }

        @Test
        void otherClientError_throwsIllegalStateException() {
            when(restTemplate.postForObject(anyString(), any(), eq(ProductServiceClient.StockResponse.class)))
                    .thenThrow(HttpClientErrorException.create(
                            HttpStatus.BAD_REQUEST, "Bad Request", null, null, null));

            assertThatThrownBy(() -> client.reserveStock(1L, 5, "order-ref"))
                    .isInstanceOf(IllegalStateException.class)
                    .hasMessageContaining("Failed to reserve stock");
        }

        @Test
        void callsCorrectUrl() {
            when(restTemplate.postForObject(anyString(), any(), eq(ProductServiceClient.StockResponse.class)))
                    .thenReturn(new ProductServiceClient.StockResponse(2L, 5, 0));

            client.reserveStock(2L, 1, "ref");

            verify(restTemplate).postForObject(
                    eq("http://product-service:8081/api/v1/inventory/2/reserve"),
                    any(),
                    eq(ProductServiceClient.StockResponse.class));
        }
    }

    // ── ReleaseStock ──────────────────────────────────────────────────────────

    @Nested
    class ReleaseStock {

        @Test
        void happyPath_returnsStockResponse() {
            ProductServiceClient.StockResponse expected =
                    new ProductServiceClient.StockResponse(1L, 10, 0);

            when(restTemplate.postForObject(
                    eq("http://product-service:8081/api/v1/inventory/1/release"),
                    any(ProductServiceClient.StockRequest.class),
                    eq(ProductServiceClient.StockResponse.class)))
                    .thenReturn(expected);

            ProductServiceClient.StockResponse result = client.releaseStock(1L, 2, "order-ref");

            assertThat(result.getAvailable()).isEqualTo(10);
        }

        @Test
        void clientError_throwsIllegalStateException() {
            when(restTemplate.postForObject(anyString(), any(), eq(ProductServiceClient.StockResponse.class)))
                    .thenThrow(HttpClientErrorException.create(
                            HttpStatus.INTERNAL_SERVER_ERROR, "Server Error", null, null, null));

            assertThatThrownBy(() -> client.releaseStock(1L, 2, "order-ref"))
                    .isInstanceOf(IllegalStateException.class)
                    .hasMessageContaining("Failed to release stock");
        }
    }
}

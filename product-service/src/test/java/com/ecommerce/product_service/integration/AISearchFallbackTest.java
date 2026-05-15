package com.ecommerce.product_service.integration;

import com.ecommerce.product_service.dto.AISearchResponse;
import com.ecommerce.product_service.dto.ApiResponse;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.github.tomakehurst.wiremock.WireMockServer;
import com.github.tomakehurst.wiremock.core.WireMockConfiguration;
import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.client.TestRestTemplate;
import org.springframework.http.ResponseEntity;
import org.springframework.test.context.ActiveProfiles;
import org.springframework.test.context.DynamicPropertyRegistry;
import org.springframework.test.context.DynamicPropertySource;
import org.testcontainers.containers.GenericContainer;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;
import org.testcontainers.utility.DockerImageName;

import static com.github.tomakehurst.wiremock.client.WireMock.*;
import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
@Testcontainers
@ActiveProfiles("test")
class AISearchFallbackTest {

    @Container
    @SuppressWarnings("resource")
    static PostgreSQLContainer<?> postgres = new PostgreSQLContainer<>(
            DockerImageName.parse("pgvector/pgvector:pg15").asCompatibleSubstituteFor("postgres"))
            .withDatabaseName("ecommerce_products")
            .withUsername("postgres")
            .withPassword("postgres")
            .withInitScript("test-pgvector-init.sql");

    @Container
    @SuppressWarnings("resource")
    static GenericContainer<?> redis = new GenericContainer<>(
            DockerImageName.parse("redis:7-alpine"))
            .withExposedPorts(6379);

    static WireMockServer wireMock;

    @BeforeAll
    static void startWireMock() {
        wireMock = new WireMockServer(WireMockConfiguration.options().dynamicPort());
        wireMock.start();
    }

    @AfterAll
    static void stopWireMock() {
        if (wireMock != null) wireMock.stop();
    }

    @DynamicPropertySource
    static void overrideProperties(DynamicPropertyRegistry registry) {
        registry.add("spring.datasource.url", postgres::getJdbcUrl);
        registry.add("spring.datasource.username", postgres::getUsername);
        registry.add("spring.datasource.password", postgres::getPassword);
        registry.add("spring.data.redis.host", redis::getHost);
        registry.add("spring.data.redis.port", () -> redis.getMappedPort(6379));
        registry.add("spring.cache.type", () -> "redis");
        registry.add("ai-service.url", wireMock::baseUrl);
        registry.add("ai-service.timeout-ms", () -> "500");
    }

    @Autowired TestRestTemplate restTemplate;
    @Autowired ObjectMapper objectMapper;

    @BeforeEach
    void resetWireMock() {
        wireMock.resetAll();
    }

    @Test
    void returns200WithFallbackKeywordModeWhenAiServiceReturns500() {
        wireMock.stubFor(post(urlEqualTo("/embed"))
                .willReturn(aResponse().withStatus(500)));

        ResponseEntity<String> raw = restTemplate.getForEntity(
                "/api/v1/products/ai-search?q=laptop&limit=5", String.class);

        assertThat(raw.getStatusCode().is2xxSuccessful()).isTrue();

        ApiResponse<AISearchResponse> resp = parseResponse(raw.getBody());
        assertThat(resp.isSuccess()).isTrue();
        assertThat(resp.getData().mode()).isEqualTo("fallback-keyword");
    }

    @Test
    void returns200WithFallbackKeywordModeOnTimeout() {
        // Delay longer than the 500ms timeout configured above
        wireMock.stubFor(post(urlEqualTo("/embed"))
                .willReturn(aResponse()
                        .withStatus(200)
                        .withFixedDelay(2000)));

        ResponseEntity<String> raw = restTemplate.getForEntity(
                "/api/v1/products/ai-search?q=shoes&limit=5", String.class);

        assertThat(raw.getStatusCode().is2xxSuccessful()).isTrue();

        ApiResponse<AISearchResponse> resp = parseResponse(raw.getBody());
        assertThat(resp.isSuccess()).isTrue();
        assertThat(resp.getData().mode()).isEqualTo("fallback-keyword");
    }

    private ApiResponse<AISearchResponse> parseResponse(String body) {
        try {
            return objectMapper.readValue(body, new TypeReference<>() {});
        } catch (Exception e) {
            throw new RuntimeException("Failed to parse response: " + body, e);
        }
    }
}

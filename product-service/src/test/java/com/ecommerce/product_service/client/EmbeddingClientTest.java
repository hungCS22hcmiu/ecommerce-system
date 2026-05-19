package com.ecommerce.product_service.client;

import com.ecommerce.product_service.exception.AIServiceException;
import com.github.tomakehurst.wiremock.WireMockServer;
import com.github.tomakehurst.wiremock.core.WireMockConfiguration;
import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.client.reactive.ReactorClientHttpConnector;
import org.springframework.web.reactive.function.client.WebClient;
import reactor.netty.http.client.HttpClient;

import java.time.Duration;

import static com.github.tomakehurst.wiremock.client.WireMock.aResponse;
import static com.github.tomakehurst.wiremock.client.WireMock.post;
import static com.github.tomakehurst.wiremock.client.WireMock.urlEqualTo;
import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

/**
 * Phase 4 — Unit tests for EmbeddingClient (testing_plan.md §9.B).
 *
 * Verifies the three outcomes of a call to /embed:
 *   200 + body                → returns float[]
 *   500                       → throws AIServiceException with cause
 *   response timeout exceeded → throws AIServiceException with cause
 *
 * Uses WireMock to stand in for ai-service; constructs the WebClient directly with
 * a 500ms response timeout so the timeout case runs fast.
 */
class EmbeddingClientTest {

    private static WireMockServer wireMock;
    private EmbeddingClient client;

    @BeforeAll
    static void startWireMock() {
        wireMock = new WireMockServer(WireMockConfiguration.options().dynamicPort());
        wireMock.start();
    }

    @AfterAll
    static void stopWireMock() {
        if (wireMock != null) wireMock.stop();
    }

    @BeforeEach
    void setUp() {
        wireMock.resetAll();
        HttpClient httpClient = HttpClient.create().responseTimeout(Duration.ofMillis(500));
        WebClient webClient = WebClient.builder()
                .baseUrl(wireMock.baseUrl())
                .clientConnector(new ReactorClientHttpConnector(httpClient))
                .build();
        client = new EmbeddingClient(webClient);
    }

    @Test
    void embed_returnsVector_onSuccess() {
        // Body: a small embedding (the schema expects List<Float>).
        wireMock.stubFor(post(urlEqualTo("/embed"))
                .willReturn(aResponse()
                        .withHeader("Content-Type", "application/json")
                        .withBody("{\"embedding\":[0.1,0.2,0.3,0.4]}")));

        float[] vec = client.embed("running shoes", "running shoes", 10);

        assertThat(vec).hasSize(4);
        assertThat(vec[0]).isEqualTo(0.1f);
        assertThat(vec[3]).isEqualTo(0.4f);
    }

    @Test
    void embed_throwsAIServiceException_on500() {
        wireMock.stubFor(post(urlEqualTo("/embed"))
                .willReturn(aResponse().withStatus(500)));

        assertThatThrownBy(() -> client.embed("query", "query", 10))
                .isInstanceOf(AIServiceException.class)
                .hasMessageContaining("ai-service unavailable");
    }

    @Test
    void embed_throwsAIServiceException_onTimeout() {
        // WireMock waits 2s before responding; WebClient response timeout is 500ms.
        wireMock.stubFor(post(urlEqualTo("/embed"))
                .willReturn(aResponse()
                        .withHeader("Content-Type", "application/json")
                        .withBody("{\"embedding\":[1.0]}")
                        .withFixedDelay(2000)));

        long t0 = System.currentTimeMillis();
        assertThatThrownBy(() -> client.embed("slow", "slow", 1))
                .isInstanceOf(AIServiceException.class);
        long elapsed = System.currentTimeMillis() - t0;

        // Must fail within ~1s (timeout 500ms + a little overhead), not wait the full 2s.
        assertThat(elapsed).as("timeout must surface within 1s").isLessThan(1500);
    }

    @Test
    void embed_throwsAIServiceException_whenEmbeddingFieldNull() {
        wireMock.stubFor(post(urlEqualTo("/embed"))
                .willReturn(aResponse()
                        .withHeader("Content-Type", "application/json")
                        .withBody("{\"embedding\":null}")));

        assertThatThrownBy(() -> client.embed("q", "q", 1))
                .isInstanceOf(AIServiceException.class)
                .hasMessageContaining("Empty embedding response");
    }
}

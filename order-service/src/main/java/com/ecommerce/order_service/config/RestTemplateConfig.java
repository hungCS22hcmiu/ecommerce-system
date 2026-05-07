package com.ecommerce.order_service.config;

import org.slf4j.MDC;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.web.client.RestTemplate;

import java.util.List;

@Configuration
public class RestTemplateConfig {

    private static final String HEADER = "X-Correlation-ID";

    @Bean
    public RestTemplate restTemplate() {
        RestTemplate rt = new RestTemplate();
        // Forward the active correlation ID from MDC to every outbound HTTP request.
        rt.setInterceptors(List.of((request, body, execution) -> {
            String correlationId = MDC.get("correlationId");
            if (correlationId != null) {
                request.getHeaders().add(HEADER, correlationId);
            }
            return execution.execute(request, body);
        }));
        return rt;
    }
}

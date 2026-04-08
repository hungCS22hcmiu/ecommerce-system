package com.ecommerce.product_service.service;

import com.ecommerce.product_service.model.ProductStatus;
import com.ecommerce.product_service.repository.ProductRepository;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.boot.context.event.ApplicationReadyEvent;
import org.springframework.context.event.EventListener;
import org.springframework.scheduling.annotation.Async;
import org.springframework.stereotype.Component;

@Slf4j
@Component
@RequiredArgsConstructor
public class CacheWarmupService {

    private final ProductRepository productRepository;
    private final ProductService productService;

    /**
     * Warms the "product" cache on startup by pre-loading the 100 most recently
     * updated active products. Runs asynchronously so it does not delay startup.
     *
     * Each call to productService.getProduct() goes through the Spring proxy,
     * which means @Cacheable fires and the result is stored in Redis.
     */
    @Async("taskExecutor")
    @EventListener(ApplicationReadyEvent.class)
    public void warmCache() {
        log.info("[CacheWarmup] Starting cache warm-up for top 100 active products...");
        long start = System.currentTimeMillis();

        var products = productRepository.findTop100ByStatusOrderByUpdatedAtDesc(ProductStatus.ACTIVE);

        int warmed = 0;
        for (var product : products) {
            try {
                productService.getProduct(product.getId());
                warmed++;
            } catch (Exception e) {
                // Product may have been deleted between query and cache load — skip silently
                log.debug("[CacheWarmup] Skipped product id={}: {}", product.getId(), e.getMessage());
            }
        }

        long elapsed = System.currentTimeMillis() - start;
        log.info("[CacheWarmup] Completed: {} products cached in {}ms", warmed, elapsed);
    }
}

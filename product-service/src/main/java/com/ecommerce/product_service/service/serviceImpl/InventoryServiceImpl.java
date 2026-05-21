package com.ecommerce.product_service.service.serviceImpl;

import com.ecommerce.product_service.dto.StockProjection;
import com.ecommerce.product_service.dto.StockResponse;
import com.ecommerce.product_service.exception.InsufficientStockException;
import com.ecommerce.product_service.exception.ProductNotFoundException;
import com.ecommerce.product_service.exception.StockContentionException;
import com.ecommerce.product_service.model.MovementType;
import com.ecommerce.product_service.model.Product;
import com.ecommerce.product_service.model.StockMovement;
import com.ecommerce.product_service.repository.ProductRepository;
import com.ecommerce.product_service.repository.StockMovementRepository;
import com.ecommerce.product_service.service.InventoryService;
import lombok.RequiredArgsConstructor;
import org.springframework.cache.annotation.CacheEvict;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

@Service
@RequiredArgsConstructor
public class InventoryServiceImpl implements InventoryService {

    private final ProductRepository productRepository;
    private final StockMovementRepository stockMovementRepository;

    @Override
    @Transactional
    @CacheEvict(value = "product", key = "#productId")
    public StockResponse reserveStock(Long productId, int quantity, String referenceId) {
        int updated = productRepository.reserveStockConditional(productId, quantity);

        if (updated == 0) {
            Product product = productRepository.findById(productId)
                    .orElseThrow(() -> new ProductNotFoundException(productId));
            int available = product.getStockQuantity() - product.getStockReserved();
            if (available < quantity) {
                throw new InsufficientStockException(productId, quantity, available);
            }
            // Safety belt: theoretically unreachable — conditional UPDATE is atomic
            throw new StockContentionException(productId);
        }

        stockMovementRepository.save(StockMovement.builder()
                .productId(productId)
                .type(MovementType.RESERVE)
                .quantity(quantity)
                .referenceId(referenceId)
                .build());

        // Use a projection instead of findById to avoid loading a managed Product entity
        // into this writable transaction. Loading the full entity triggers a spurious
        // Hibernate dirty-check UPDATE (incrementing @Version) even though no application
        // code modifies the entity, causing ObjectOptimisticLockingFailureException under
        // concurrent load and false INSUFFICIENT_STOCK errors.
        StockProjection sp = productRepository.findStockById(productId)
                .orElseThrow(() -> new ProductNotFoundException(productId));
        return new StockResponse(productId, sp.getStockQuantity(), sp.getStockReserved(),
                sp.getStockQuantity() - sp.getStockReserved());
    }

    @Override
    @Transactional
    @CacheEvict(value = "product", key = "#productId")
    public StockResponse releaseStock(Long productId, int quantity, String referenceId) {
        int updated = productRepository.releaseStockConditional(productId, quantity);

        if (updated == 0) {
            Product product = productRepository.findById(productId)
                    .orElseThrow(() -> new ProductNotFoundException(productId));
            throw new IllegalArgumentException(
                    "Cannot release " + quantity + " units: only " + product.getStockReserved() + " reserved");
        }

        stockMovementRepository.save(StockMovement.builder()
                .productId(productId)
                .type(MovementType.RELEASE)
                .quantity(quantity)
                .referenceId(referenceId)
                .build());

        StockProjection sp = productRepository.findStockById(productId)
                .orElseThrow(() -> new ProductNotFoundException(productId));
        return new StockResponse(productId, sp.getStockQuantity(), sp.getStockReserved(),
                sp.getStockQuantity() - sp.getStockReserved());
    }

    @Override
    @Transactional(readOnly = true)
    public StockResponse getStockLevel(Long productId) {
        Product product = productRepository.findById(productId)
                .orElseThrow(() -> new ProductNotFoundException(productId));
        return toStockResponse(product);
    }

    private StockResponse toStockResponse(Product product) {
        return new StockResponse(
                product.getId(),
                product.getStockQuantity(),
                product.getStockReserved(),
                product.getStockQuantity() - product.getStockReserved()
        );
    }
}

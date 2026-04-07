package com.ecommerce.product_service.service.serviceImpl;

import com.ecommerce.product_service.dto.StockResponse;
import com.ecommerce.product_service.exception.InsufficientStockException;
import com.ecommerce.product_service.exception.ProductNotFoundException;
import com.ecommerce.product_service.model.MovementType;
import com.ecommerce.product_service.model.Product;
import com.ecommerce.product_service.model.StockMovement;
import com.ecommerce.product_service.repository.ProductRepository;
import com.ecommerce.product_service.repository.StockMovementRepository;
import com.ecommerce.product_service.service.InventoryService;
import lombok.RequiredArgsConstructor;
import org.springframework.orm.ObjectOptimisticLockingFailureException;
import org.springframework.retry.annotation.Backoff;
import org.springframework.retry.annotation.Retryable;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

@Service
@RequiredArgsConstructor
public class InventoryServiceImpl implements InventoryService {

    private final ProductRepository productRepository;
    private final StockMovementRepository stockMovementRepository;

    @Override
    @Transactional
    @Retryable(retryFor = ObjectOptimisticLockingFailureException.class,
               maxAttempts = 3,
               backoff = @Backoff(delay = 100))
    public StockResponse reserveStock(Long productId, int quantity, String referenceId) {
        Product product = productRepository.findById(productId)
                .orElseThrow(() -> new ProductNotFoundException(productId));

        int available = product.getStockQuantity() - product.getStockReserved();
        if (available < quantity) {
            throw new InsufficientStockException(productId, quantity, available);
        }

        product.setStockReserved(product.getStockReserved() + quantity);
        productRepository.save(product);

        stockMovementRepository.save(StockMovement.builder()
                .productId(productId)
                .type(MovementType.RESERVE)
                .quantity(quantity)
                .referenceId(referenceId)
                .build());

        return toStockResponse(product);
    }

    @Override
    @Transactional
    @Retryable(retryFor = ObjectOptimisticLockingFailureException.class,
               maxAttempts = 3,
               backoff = @Backoff(delay = 100))
    public StockResponse releaseStock(Long productId, int quantity, String referenceId) {
        Product product = productRepository.findById(productId)
                .orElseThrow(() -> new ProductNotFoundException(productId));

        if (product.getStockReserved() < quantity) {
            throw new IllegalArgumentException(
                    "Cannot release " + quantity + " units: only " + product.getStockReserved() + " reserved");
        }

        product.setStockReserved(product.getStockReserved() - quantity);
        productRepository.save(product);

        stockMovementRepository.save(StockMovement.builder()
                .productId(productId)
                .type(MovementType.RELEASE)
                .quantity(quantity)
                .referenceId(referenceId)
                .build());

        return toStockResponse(product);
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

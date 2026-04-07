package com.ecommerce.product_service.service;

import com.ecommerce.product_service.dto.StockResponse;
import com.ecommerce.product_service.exception.InsufficientStockException;
import com.ecommerce.product_service.exception.ProductNotFoundException;
import com.ecommerce.product_service.model.MovementType;
import com.ecommerce.product_service.model.Product;
import com.ecommerce.product_service.model.StockMovement;
import com.ecommerce.product_service.repository.ProductRepository;
import com.ecommerce.product_service.repository.StockMovementRepository;
import com.ecommerce.product_service.service.serviceImpl.InventoryServiceImpl;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Nested;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.math.BigDecimal;
import java.util.Optional;
import java.util.UUID;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.*;

@ExtendWith(MockitoExtension.class)
class InventoryServiceImplTest {

    @Mock
    private ProductRepository productRepository;

    @Mock
    private StockMovementRepository stockMovementRepository;

    @InjectMocks
    private InventoryServiceImpl inventoryService;

    private Product product;

    @BeforeEach
    void setUp() {
        product = Product.builder()
                .id(1L)
                .name("Test Product")
                .price(new BigDecimal("99.99"))
                .sellerId(UUID.randomUUID())
                .stockQuantity(10)
                .stockReserved(0)
                .build();
    }

    // ── Helpers ──────────────────────────────────────────────────────────────

    private void givenProductExists() {
        when(productRepository.findById(1L)).thenReturn(Optional.of(product));
    }

    private void givenProductNotFound() {
        when(productRepository.findById(1L)).thenReturn(Optional.empty());
    }

    // ── ReserveStock ─────────────────────────────────────────────────────────

    @Nested
    class ReserveStock {

        @Test
        void happyPath_reservesStockAndRecordsMovement() {
            givenProductExists();
            when(productRepository.save(any())).thenReturn(product);

            StockResponse response = inventoryService.reserveStock(1L, 5, "order-123");

            assertThat(response.productId()).isEqualTo(1L);
            assertThat(response.stockQuantity()).isEqualTo(10);
            assertThat(response.stockReserved()).isEqualTo(5);
            assertThat(response.availableStock()).isEqualTo(5);

            ArgumentCaptor<Product> productCaptor = ArgumentCaptor.forClass(Product.class);
            verify(productRepository).save(productCaptor.capture());
            assertThat(productCaptor.getValue().getStockReserved()).isEqualTo(5);

            ArgumentCaptor<StockMovement> movementCaptor = ArgumentCaptor.forClass(StockMovement.class);
            verify(stockMovementRepository).save(movementCaptor.capture());
            StockMovement movement = movementCaptor.getValue();
            assertThat(movement.getType()).isEqualTo(MovementType.RESERVE);
            assertThat(movement.getQuantity()).isEqualTo(5);
            assertThat(movement.getReferenceId()).isEqualTo("order-123");
        }

        @Test
        void insufficientStock_throwsException() {
            product.setStockReserved(8); // available = 10 - 8 = 2
            givenProductExists();

            assertThatThrownBy(() -> inventoryService.reserveStock(1L, 5, "order-456"))
                    .isInstanceOf(InsufficientStockException.class)
                    .hasMessageContaining("requested=5")
                    .hasMessageContaining("available=2");

            verify(productRepository, never()).save(any());
            verify(stockMovementRepository, never()).save(any());
        }

        @Test
        void productNotFound_throwsException() {
            givenProductNotFound();

            assertThatThrownBy(() -> inventoryService.reserveStock(1L, 3, "order-789"))
                    .isInstanceOf(ProductNotFoundException.class);
        }

        @Test
        void exactAvailableStock_succeeds() {
            givenProductExists();
            when(productRepository.save(any())).thenReturn(product);

            StockResponse response = inventoryService.reserveStock(1L, 10, "order-exact");

            assertThat(response.availableStock()).isEqualTo(0);
            assertThat(response.stockReserved()).isEqualTo(10);
        }
    }

    // ── ReleaseStock ─────────────────────────────────────────────────────────

    @Nested
    class ReleaseStock {

        @BeforeEach
        void setReserved() {
            product.setStockReserved(5);
        }

        @Test
        void happyPath_releasesStockAndRecordsMovement() {
            givenProductExists();
            when(productRepository.save(any())).thenReturn(product);

            StockResponse response = inventoryService.releaseStock(1L, 3, "order-123");

            assertThat(response.stockReserved()).isEqualTo(2);
            assertThat(response.availableStock()).isEqualTo(8);

            ArgumentCaptor<Product> productCaptor = ArgumentCaptor.forClass(Product.class);
            verify(productRepository).save(productCaptor.capture());
            assertThat(productCaptor.getValue().getStockReserved()).isEqualTo(2);

            ArgumentCaptor<StockMovement> movementCaptor = ArgumentCaptor.forClass(StockMovement.class);
            verify(stockMovementRepository).save(movementCaptor.capture());
            assertThat(movementCaptor.getValue().getType()).isEqualTo(MovementType.RELEASE);
        }

        @Test
        void releaseMoreThanReserved_throwsException() {
            givenProductExists();

            assertThatThrownBy(() -> inventoryService.releaseStock(1L, 10, "order-over"))
                    .isInstanceOf(IllegalArgumentException.class)
                    .hasMessageContaining("Cannot release 10");

            verify(productRepository, never()).save(any());
        }

        @Test
        void productNotFound_throwsException() {
            givenProductNotFound();

            assertThatThrownBy(() -> inventoryService.releaseStock(1L, 3, "order-789"))
                    .isInstanceOf(ProductNotFoundException.class);
        }
    }

    // ── GetStockLevel ─────────────────────────────────────────────────────────

    @Nested
    class GetStockLevel {

        @Test
        void returnsCorrectAvailableStock() {
            product.setStockReserved(3);
            givenProductExists();

            StockResponse response = inventoryService.getStockLevel(1L);

            assertThat(response.productId()).isEqualTo(1L);
            assertThat(response.stockQuantity()).isEqualTo(10);
            assertThat(response.stockReserved()).isEqualTo(3);
            assertThat(response.availableStock()).isEqualTo(7);
        }

        @Test
        void productNotFound_throwsException() {
            givenProductNotFound();

            assertThatThrownBy(() -> inventoryService.getStockLevel(1L))
                    .isInstanceOf(ProductNotFoundException.class);
        }
    }
}

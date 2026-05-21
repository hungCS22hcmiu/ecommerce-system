package com.ecommerce.product_service.exception;

import com.ecommerce.product_service.dto.AISearchResponse;
import com.ecommerce.product_service.dto.ApiResponse;
import com.ecommerce.product_service.dto.ProductSummaryResponse;
import com.ecommerce.product_service.service.ProductService;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.PageRequest;
import org.springframework.data.domain.Pageable;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.orm.ObjectOptimisticLockingFailureException;
import org.springframework.web.bind.MethodArgumentNotValidException;
import org.springframework.web.bind.MissingRequestHeaderException;
import org.springframework.web.bind.MissingServletRequestParameterException;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.ResponseStatus;
import org.springframework.web.bind.annotation.RestControllerAdvice;
import org.springframework.web.method.annotation.MethodArgumentTypeMismatchException;
import org.springframework.web.servlet.resource.NoResourceFoundException;

@Slf4j
@RestControllerAdvice
@RequiredArgsConstructor
public class GlobalExceptionHandler {

    private final ProductService productService;

    @ExceptionHandler(AIServiceException.class)
    public ResponseEntity<ApiResponse<AISearchResponse>> handleAIService(AIServiceException ex) {
        log.warn("AI search fallback triggered for query='{}': {}", ex.getQuery(), ex.getMessage());
        Pageable pageable = PageRequest.of(0, ex.getLimit());
        Page<ProductSummaryResponse> page = productService.searchProducts(ex.getQuery(), null, null, pageable);
        AISearchResponse fallback = new AISearchResponse(ex.getQuery(), page.getContent(), null, "fallback-keyword");
        return ResponseEntity.ok(ApiResponse.ok(fallback));
    }

    @ExceptionHandler(ProductNotFoundException.class)
    @ResponseStatus(HttpStatus.NOT_FOUND)
    public ApiResponse<?> handleNotFound(ProductNotFoundException ex) {
        return ApiResponse.error("PRODUCT_NOT_FOUND", ex.getMessage());
    }

    @ExceptionHandler(InsufficientStockException.class)
    @ResponseStatus(HttpStatus.CONFLICT)
    public ApiResponse<?> handleInsufficientStock(InsufficientStockException ex) {
        return ApiResponse.error("INSUFFICIENT_STOCK", ex.getMessage());
    }

    @ExceptionHandler(StockContentionException.class)
    @ResponseStatus(HttpStatus.SERVICE_UNAVAILABLE)
    public ApiResponse<?> handleStockContention(StockContentionException ex) {
        return ApiResponse.error("STOCK_CONTENTION", ex.getMessage());
    }

    @ExceptionHandler(ObjectOptimisticLockingFailureException.class)
    @ResponseStatus(HttpStatus.CONFLICT)
    public ApiResponse<?> handleOptimisticLock(ObjectOptimisticLockingFailureException ex) {
        return ApiResponse.error("CONCURRENT_MODIFICATION", "Resource was modified concurrently, please retry");
    }

    @ExceptionHandler(IllegalArgumentException.class)
    @ResponseStatus(HttpStatus.BAD_REQUEST)
    public ApiResponse<?> handleIllegalArgument(IllegalArgumentException ex) {
        return ApiResponse.error("BAD_REQUEST", ex.getMessage());
    }

    @ExceptionHandler(ProductAccessDeniedException.class)
    @ResponseStatus(HttpStatus.FORBIDDEN)
    public ApiResponse<?> handleAccessDenied(ProductAccessDeniedException ex) {
        return ApiResponse.error("ACCESS_DENIED", ex.getMessage());
    }

    @ExceptionHandler(ReviewNotFoundException.class)
    @ResponseStatus(HttpStatus.NOT_FOUND)
    public ApiResponse<?> handleReviewNotFound(ReviewNotFoundException ex) {
        return ApiResponse.error("REVIEW_NOT_FOUND", ex.getMessage());
    }

    @ExceptionHandler(ReviewAccessDeniedException.class)
    @ResponseStatus(HttpStatus.FORBIDDEN)
    public ApiResponse<?> handleReviewAccessDenied(ReviewAccessDeniedException ex) {
        return ApiResponse.error("ACCESS_DENIED", ex.getMessage());
    }

    @ExceptionHandler(AlreadyReviewedException.class)
    @ResponseStatus(HttpStatus.CONFLICT)
    public ApiResponse<?> handleAlreadyReviewed(AlreadyReviewedException ex) {
        return ApiResponse.error("ALREADY_REVIEWED", ex.getMessage());
    }

    @ExceptionHandler(PurchaseNotVerifiedException.class)
    @ResponseStatus(HttpStatus.FORBIDDEN)
    public ApiResponse<?> handlePurchaseNotVerified(PurchaseNotVerifiedException ex) {
        return ApiResponse.error("PURCHASE_NOT_VERIFIED", ex.getMessage());
    }

    @ExceptionHandler(PurchaseVerificationException.class)
    @ResponseStatus(HttpStatus.BAD_GATEWAY)
    public ApiResponse<?> handlePurchaseVerificationException(PurchaseVerificationException ex) {
        return ApiResponse.error("PURCHASE_VERIFICATION_ERROR", ex.getMessage());
    }

    @ExceptionHandler(MethodArgumentNotValidException.class)
    @ResponseStatus(HttpStatus.BAD_REQUEST)
    public ApiResponse<?> handleValidation(MethodArgumentNotValidException ex) {
        String message = ex.getBindingResult().getFieldErrors().stream()
                .map(fe -> fe.getField() + ": " + fe.getDefaultMessage())
                .findFirst()
                .orElse("Validation failed");
        return ApiResponse.error("VALIDATION_ERROR", message);
    }

    @ExceptionHandler(MissingRequestHeaderException.class)
    @ResponseStatus(HttpStatus.BAD_REQUEST)
    public ApiResponse<?> handleMissingHeader(MissingRequestHeaderException ex) {
        return ApiResponse.error("BAD_REQUEST", ex.getMessage());
    }

    @ExceptionHandler(MethodArgumentTypeMismatchException.class)
    @ResponseStatus(HttpStatus.BAD_REQUEST)
    public ApiResponse<?> handleTypeMismatch(MethodArgumentTypeMismatchException ex) {
        String message = "Invalid value for parameter '" + ex.getName() + "': " + ex.getValue();
        return ApiResponse.error("BAD_REQUEST", message);
    }

    @ExceptionHandler(MissingServletRequestParameterException.class)
    @ResponseStatus(HttpStatus.BAD_REQUEST)
    public ApiResponse<?> handleMissingParam(MissingServletRequestParameterException ex) {
        return ApiResponse.error("BAD_REQUEST", ex.getMessage());
    }

    @ExceptionHandler(NoResourceFoundException.class)
    @ResponseStatus(HttpStatus.NOT_FOUND)
    public ApiResponse<?> handleNoResource(NoResourceFoundException ex) {
        return ApiResponse.error("NOT_FOUND", ex.getMessage());
    }

    @ExceptionHandler(Exception.class)
    @ResponseStatus(HttpStatus.INTERNAL_SERVER_ERROR)
    public ApiResponse<?> handleGeneric(Exception ex) {
        log.error("Unhandled exception", ex);
        return ApiResponse.error("INTERNAL_ERROR", "An unexpected error occurred");
    }
}

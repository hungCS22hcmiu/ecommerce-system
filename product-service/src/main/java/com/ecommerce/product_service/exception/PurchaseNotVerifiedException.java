package com.ecommerce.product_service.exception;

public class PurchaseNotVerifiedException extends RuntimeException {
    public PurchaseNotVerifiedException() {
        super("Purchase could not be verified. You must have a delivered order for this product.");
    }
}

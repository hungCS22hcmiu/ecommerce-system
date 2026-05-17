package com.ecommerce.product_service.exception;

public class AlreadyReviewedException extends RuntimeException {
    public AlreadyReviewedException() {
        super("You have already submitted a review for this purchase.");
    }
}

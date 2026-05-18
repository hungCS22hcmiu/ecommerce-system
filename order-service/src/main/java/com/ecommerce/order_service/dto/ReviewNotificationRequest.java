package com.ecommerce.order_service.dto;

import java.util.UUID;

public record ReviewNotificationRequest(UUID sellerId, Long productId, String title, String body) {}

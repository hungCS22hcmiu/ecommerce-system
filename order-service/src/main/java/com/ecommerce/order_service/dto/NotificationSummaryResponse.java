package com.ecommerce.order_service.dto;

import java.util.List;

public record NotificationSummaryResponse(long unreadCount, List<NotificationResponse> items) {}

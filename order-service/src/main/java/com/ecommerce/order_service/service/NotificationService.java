package com.ecommerce.order_service.service;

import com.ecommerce.order_service.dto.NotificationResponse;
import com.ecommerce.order_service.dto.NotificationSummaryResponse;
import com.ecommerce.order_service.model.Notification;
import com.ecommerce.order_service.model.NotificationType;
import com.ecommerce.order_service.repository.NotificationRepository;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.OffsetDateTime;
import java.util.List;
import java.util.UUID;

@Slf4j
@Service
@RequiredArgsConstructor
public class NotificationService {

    private final NotificationRepository repo;

    @Transactional
    public void notifySeller(UUID sellerId, UUID orderId, String title, String body) {
        persist(sellerId, orderId, null, title, body);
    }

    @Transactional
    public void notifyBuyer(UUID buyerId, UUID orderId, String title, String body) {
        persist(buyerId, orderId, null, title, body);
    }

    @Transactional
    public void notifySellerReview(UUID sellerId, Long productId, String title, String body) {
        persist(sellerId, null, productId, title, body);
    }

    private void persist(UUID userId, UUID orderId, Long productId, String title, String body) {
        repo.save(Notification.builder()
                .userId(userId)
                .orderId(orderId)
                .productId(productId)
                .type(NotificationType.IN_APP)
                .title(title)
                .body(body)
                .isRead(false)
                .status("SENT")
                .createdAt(OffsetDateTime.now())
                .build());
    }

    @Transactional(readOnly = true)
    public NotificationSummaryResponse getSummary(UUID userId) {
        long unreadCount = repo.countByUserIdAndIsReadFalse(userId);
        List<NotificationResponse> items = repo.findTop20ByUserIdOrderByCreatedAtDesc(userId)
                .stream()
                .map(NotificationResponse::from)
                .toList();
        return new NotificationSummaryResponse(unreadCount, items);
    }

    @Transactional
    public void markAllRead(UUID userId) {
        repo.markAllReadForUser(userId);
    }
}

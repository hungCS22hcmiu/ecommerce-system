package com.ecommerce.order_service.model;

import jakarta.persistence.*;
import lombok.*;

import java.time.OffsetDateTime;
import java.util.UUID;

@Entity
@Table(name = "notifications")
@Getter
@Setter
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class Notification {

    @Id
    @GeneratedValue(strategy = GenerationType.UUID)
    @Column(columnDefinition = "uuid")
    private UUID id;

    @Column(name = "order_id", columnDefinition = "uuid")
    private UUID orderId;

    @Column(name = "product_id")
    private Long productId;

    @Column(name = "user_id", nullable = false, columnDefinition = "uuid")
    private UUID userId;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false)
    private NotificationType type;

    @Column
    private String title;

    @Column
    private String body;

    @Column(name = "is_read", nullable = false)
    private boolean isRead;

    @Column(name = "created_at", columnDefinition = "timestamptz")
    private OffsetDateTime createdAt;

    // Legacy columns for EMAIL/SMS — nullable and unused for IN_APP
    @Column
    private String channel;

    @Column
    private String subject;

    @Column
    private String status;
}

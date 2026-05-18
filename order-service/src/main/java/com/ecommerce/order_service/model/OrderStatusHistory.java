package com.ecommerce.order_service.model;

import jakarta.persistence.*;
import lombok.*;
import java.time.OffsetDateTime;
import java.util.UUID;

@Entity
@Table(name = "order_status_history")
@Getter
@Setter
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class OrderStatusHistory {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(name = "order_id", nullable = false, columnDefinition = "uuid")
    private UUID orderId;

    @Enumerated(EnumType.STRING)
    @Column(name = "old_status")
    private OrderStatus oldStatus;

    @Enumerated(EnumType.STRING)
    @Column(name = "new_status", nullable = false)
    private OrderStatus newStatus;

    @Column(name = "reason")
    private String reason;

    @Column(name = "changed_by", length = 100)
    private String changedBy;

    @Column(name = "changed_at", nullable = false, columnDefinition = "timestamptz")
    @Builder.Default
    private OffsetDateTime changedAt = OffsetDateTime.now();
}

# Async Payment Saga with Kafka

## Overview

The payment flow is fully asynchronous. `POST /orders` returns HTTP 201 the moment the order is written to the database — the client does not wait for a payment gateway response. Charging happens in a separate process, coordinated by Kafka. Three design properties make this safe:

1. **Instant order response** — the HTTP handler returns as soon as the order row and its outbox event are committed atomically.
2. **3-tier resilience** — every failure mode is classified and handled differently: poison-pill messages go to DLQ immediately, transient errors retry with exponential backoff before DLQ, permanent declines publish a failure event directly with no retry and no DLQ.
3. **Zero data loss** — the Kafka offset is never committed until the outcome event has been published. A process kill at any point causes re-delivery, not message loss.

---

## Saga Flow

```
Client
  └─► POST /orders
        │  (HTTP 201 — instant response)
        │
        ▼
  order-service
  ├── writes order row (status=PENDING)
  └── writes outbox row  ─ atomic, same DB transaction
            │
            ▼
  OutboxPublisher (polls every 100ms)
  └── publishes orders.created → Kafka
                │
                ▼
  payment-service (Kafka consumer, 5 workers)
  ├── creates PENDING payment row
  ├── calls mock gateway (50–200ms latency)
  └── publishes outcome
        ├── payments.completed ──► order-service: PENDING → CONFIRMED
        │                          seller notification sent
        └── payments.failed   ──► order-service: PENDING → CANCELLED
                                   stock released async
```

The client observes the saga completing by polling `GET /payments/order/:orderId` until the status reaches `COMPLETED` or `FAILED`. The k6 test `script/k6/saga_happy.js` measures this time-to-consistency (TTC) from order creation to terminal payment status.

---

## 1. Instant Order Response

Order creation is synchronous only for the database write. The Kafka event is published by a background polling loop, not by the HTTP handler:

`order-service/src/main/java/com/ecommerce/order_service/service/impl/OrderServiceImpl.java`
```java
@Transactional
public OrderResponse createOrder(UUID userId, CreateOrderRequest request) {
    // ... stock reservation, order build ...
    Order savedOrder = orderRepository.save(order);

    // Outbox row written atomically with the order — same transaction.
    OutboxEvent outbox = OutboxEvent.forOrderCreated(savedOrder, correlationId);
    outboxEventRepository.save(outbox);

    return OrderMapper.toResponse(savedOrder);
    // HTTP 201 returned here — no Kafka call on the hot path.
}
```

`order-service/src/main/java/com/ecommerce/order_service/kafka/OutboxPublisher.java`
```java
@Scheduled(fixedDelay = 100)
@Transactional
public void publishPending() {
    List<OutboxEvent> events = outboxEventRepository
        .findUnpublishedWithLock();   // SELECT ... FOR UPDATE SKIP LOCKED
    for (OutboxEvent e : events) {
        kafkaTemplate.send(toProducerRecord(e));
        e.markPublished();
        outboxEventRepository.save(e);
    }
}
```

The outbox guarantees that once the order transaction commits, the event will be published — there is no dual-write window where an order exists without a corresponding Kafka event.

---

## 2. Consumer Architecture

Payment-service subscribes to `orders.created` using a manually committed reader feeding a bounded worker pool:

`payment-service/internal/kafka/consumer.go`
```go
type Consumer struct {
    reader   *kafka.Reader
    producer *Producer
    svc      service.PaymentService
    cfg      *config.Config
    jobs     chan kafka.Message   // buffered cap 100 — absorbs bursts
    wg       sync.WaitGroup
}
```

`payment-service/config/config.go`
```go
KafkaWorkerCount: 5,   // 5 concurrent workers in the pool
```

`payment-service/internal/kafka/consumer.go` — `NewConsumer()`
```go
reader := kafka.NewReader(kafka.ReaderConfig{
    Brokers:        []string{cfg.KafkaBrokers},
    Topic:          "orders.created",
    GroupID:        cfg.KafkaConsumerGroup,
    CommitInterval: 0,               // manual commit — never auto-ack
    StartOffset:    kafka.FirstOffset, // process unacked messages after restart
})
```

`CommitInterval: 0` is the foundation of the zero-data-loss guarantee. The consumer only commits an offset after the outcome event has been successfully published to Kafka. Any process kill before that point causes re-delivery from the last committed offset.

---

## 3. Three-Tier Resilience

Every message entering the consumer is classified immediately after deserialization. The classification determines which of three handling paths applies.

```
orders.created message received
         │
         ▼
   deserialize JSON ──── fail ──► DLQ (poison pill, 1 attempt)
         │
         ▼ success
   ProcessPayment (attempt 1)
         │
    ┌────┴────────────────────────────┐
    │ error?                          │
    ▼ transient (DB, timeout)         ▼ permanent (declined)
  retry with backoff              publish payments.failed
  100ms → 200ms → 400ms           (no retry, no DLQ)
    │
    ▼ still failing after 4 attempts
  DLQ (process stage)
    │
    ▼ success
  publish payments.completed / payments.failed
    │
    ▼
  CommitMessages (offset committed)
```

### Tier 1 — Poison Pill → Immediate DLQ

A message that cannot be deserialized (malformed JSON, wrong schema) will never succeed regardless of retries. It is routed to `payments.dlq` immediately, bypassing `ProcessPayment` entirely:

`payment-service/internal/kafka/consumer.go` — `processMessage()`
```go
var evt kafkaevent.OrderCreatedEvent
if err := json.Unmarshal(msg.Value, &evt); err != nil {
    if dlqErr := c.sendToDLQ(msgCtx, msg, err.Error(), "deserialize", 1, correlationID, logBase); dlqErr != nil {
        return // DLQ publish failed — don't commit, redeliver on restart
    }
    _ = c.reader.CommitMessages(msgCtx, msg)
    return
}
```

The DLQ envelope preserves the full original message for offline inspection:

`payment-service/internal/kafka/consumer.go` — `DLQMessage`
```go
type DLQMessage struct {
    OriginalTopic     string `json:"originalTopic"`
    OriginalPartition int    `json:"originalPartition"`
    OriginalOffset    int64  `json:"originalOffset"`
    OriginalKey       string `json:"originalKey"`
    OriginalValue     string `json:"originalValue"` // base64-encoded raw bytes
    ErrorReason       string `json:"errorReason"`
    ErrorStage        string `json:"errorStage"`    // "deserialize" | "process"
    Attempts          int    `json:"attempts"`
    FailedAt          string `json:"failedAt"`      // RFC3339 UTC
    CorrelationID     string `json:"correlationId"`
}
```

### Tier 2 — Transient Error → Exponential Backoff → DLQ

Database blips, connection timeouts, and context deadlines are transient — they may succeed on the next attempt. The consumer retries up to 3 times with exponential backoff, then routes to DLQ:

`payment-service/internal/kafka/consumer.go`
```go
var backoffs = []time.Duration{
    100 * time.Millisecond,
    200 * time.Millisecond,
    400 * time.Millisecond,
}
```

`payment-service/internal/kafka/consumer.go` — `processMessage()`, retry loop
```go
attempts := 0
for ; attempts <= len(backoffs); attempts++ {
    payment, lastErr = c.svc.ProcessPayment(msgCtx, input)
    if lastErr == nil || classifyError(lastErr) == errKindPermanent {
        break
    }
    slog.Warn("kafka.worker: ProcessPayment transient error, retrying",
        append(logBase, "attempt", attempts+1, "error", lastErr)...)
    if attempts < len(backoffs) {
        select {
        case <-msgCtx.Done():
            return // shutdown mid-retry — don't commit, force redelivery
        case <-time.After(backoffs[attempts]):
        }
    }
}

if lastErr != nil {
    // All 4 attempts failed — route to DLQ with errorStage="process".
    if dlqErr := c.sendToDLQ(msgCtx, msg, lastErr.Error(), "process", attempts+1, ...); dlqErr != nil {
        return // DLQ publish failed — don't commit, redeliver
    }
    _ = c.reader.CommitMessages(msgCtx, msg)
    return
}
```

The `select` in the backoff sleep has two arms: `<-time.After(backoff)` waits for the delay, and `<-msgCtx.Done()` exits immediately if shutdown is signalled mid-retry. In that case, the offset is not committed and the message is re-delivered on restart.

Total backoff time before DLQ: 100 + 200 + 400 = **700ms** across 4 attempts.

### Tier 3 — Permanent Decline → payments.failed (No DLQ)

A gateway decline is a business outcome, not a processing failure. The gateway returns `ErrGatewayDeclined`, which `classifyError` identifies as permanent:

`payment-service/internal/kafka/consumer.go` — `classifyError()`
```go
func classifyError(err error) errKind {
    if errors.Is(err, gateway.ErrGatewayDeclined) {
        return errKindPermanent
    }
    return errKindTransient
}
```

`payment-service/internal/service/payment_service.go` — `ProcessPayment()`, decline path
```go
txnID, err := s.gw.Charge(gwCtx, p.Amount, p.Currency, p.ID.String())
if err != nil {
    if errors.Is(err, gateway.ErrGatewayDeclined) {
        if updateErr := s.repo.UpdateStatus(ctx, p.ID, model.PaymentStatusFailed, "", "gateway declined"); updateErr != nil {
            return nil, updateErr
        }
    } else {
        return nil, err   // transient — bubble up for retry
    }
}
```

When `ProcessPayment` returns a FAILED payment with `nil` error, the retry loop exits on the first attempt. `publishOutcome` sends `payments.failed` to Kafka, and the offset is committed. No DLQ entry is created.

`payment-service/internal/kafka/consumer.go` — `publishOutcome()`
```go
func (c *Consumer) publishOutcome(ctx context.Context, payment *model.Payment, correlationID string) error {
    switch payment.Status {
    case model.PaymentStatusCompleted:
        return c.producer.PublishCompleted(ctx, kafkaevent.PaymentCompletedEvent{
            OrderID:   payment.OrderID,
            PaymentID: payment.ID,
            Amount:    payment.Amount,
        }, correlationID)
    case model.PaymentStatusFailed:
        return c.producer.PublishFailed(ctx, kafkaevent.PaymentFailedEvent{
            OrderID: payment.OrderID,
            Reason:  fmt.Sprintf("gateway declined (paymentId=%s)", payment.ID),
        }, correlationID)
    default:
        // PENDING: skip commit, force redelivery
        return fmt.Errorf("payment %s still PENDING after ProcessPayment", payment.ID)
    }
}
```

---

## 4. Zero Data Loss: Offset Commit Discipline

The offset is committed only after the outcome event is published. Three cases deliberately skip the commit:

| Situation | Why no commit | What happens next |
|---|---|---|
| DLQ publish fails | Message is not acknowledged; re-delivered on restart, another DLQ attempt | Redeliver until DLQ succeeds |
| Payment still PENDING after `publishOutcome` | Service was killed after DB write but before gateway returned | Re-delivered; PENDING-resume path retries gateway |
| `ctx.Done()` fires mid-retry | Shutdown in progress; leaving offset uncommitted ensures redelivery | Re-delivered from last committed offset on restart |

`payment-service/internal/kafka/consumer.go` — `processMessage()`, commit section
```go
// Only reached if ProcessPayment succeeded and outcome published.
if commitErr := c.reader.CommitMessages(msgCtx, msg); commitErr != nil {
    slog.Error("kafka.worker: commit failed", append(logBase, "error", commitErr)...)
    return
}
```

### PENDING-Resume Path

If the service is killed after writing the PENDING row but before the gateway call completes, the next restart re-delivers the same Kafka message. `ProcessPayment` detects the existing row via the UNIQUE idempotency key constraint and resumes using the same payment ID as the gateway reference:

`payment-service/internal/service/payment_service.go` — `ProcessPayment()`
```go
if err := s.repo.Create(ctx, p, h); err != nil {
    if !errors.Is(err, repository.ErrDuplicateIdempotencyKey) {
        return nil, err
    }
    existing, findErr := s.repo.FindByIdempotencyKey(ctx, in.IdempotencyKey)
    if findErr != nil {
        return nil, findErr
    }
    if existing.Status != model.PaymentStatusPending {
        // Already terminal (COMPLETED or FAILED) — idempotent return.
        return existing, nil
    }
    // PENDING: killed mid-gateway-call. Resume with the existing payment ID
    // so the gateway call is retried with the same reference (idempotent charge).
    p = existing
}
// gateway Charge call continues with p (original or resumed)...
```

`payment-service/internal/repository/payment_repository.go` — `isDuplicateKey()`
```go
func isDuplicateKey(err error) bool {
    var pgErr *pgconn.PgError
    return errors.As(err, &pgErr) && pgErr.Code == "23505"   // SQLSTATE 23505: unique_violation
}
```

The schema enforces this at the database level:

`payment-service/migrations/000001_baseline_schema.up.sql`
```sql
CREATE UNIQUE INDEX IF NOT EXISTS idx_payments_idempotency ON payments(idempotency_key);
CONSTRAINT uq_payments_order_id UNIQUE (order_id)
```

---

## 5. Order-Service Closes the Loop

When payment-service publishes a result event, order-service transitions the order to its terminal state:

`order-service/src/main/java/com/ecommerce/order_service/kafka/PaymentEventConsumer.java`
```java
@KafkaListener(topics = "payments.completed", groupId = "order-service")
public void onPaymentCompleted(ConsumerRecord<String, PaymentCompletedEvent> record) {
    PaymentCompletedEvent event = record.value();
    orderService.updateOrderStatus(
        event.getOrderId(),
        OrderStatus.CONFIRMED,
        "Payment completed (paymentId=" + event.getPaymentId() + ")",
        "payment-service"
    );
    // Seller notification sent async
}

@KafkaListener(topics = "payments.failed", groupId = "order-service")
public void onPaymentFailed(ConsumerRecord<String, PaymentFailedEvent> record) {
    PaymentFailedEvent event = record.value();
    orderService.updateOrderStatus(
        event.getOrderId(), OrderStatus.CANCELLED, "Payment failed: " + event.getReason(), "payment-service"
    );
    orderService.releaseStockForOrder(event.getOrderId());   // async, @Retryable 3×
}
```

The correlation ID is propagated through every Kafka message header, linking the order HTTP request to all downstream Kafka events in the logs of all five services.

---

## 6. Producer Configuration

All three outbound writers use synchronous writes and `RequireAll` acknowledgements:

`payment-service/internal/kafka/producer.go` — `newWriter()`
```go
func newWriter(brokers []string, topic string) *kafka.Writer {
    return &kafka.Writer{
        Addr:         kafka.TCP(brokers...),
        Topic:        topic,
        RequiredAcks: kafka.RequireAll,   // wait for all in-sync replicas
        BatchTimeout: 10 * time.Millisecond,
        Async:        false,              // publish errors surface to the caller immediately
    }
}
```

`Async: false` means `WriteMessages` blocks until the broker confirms. If the publish fails, the caller knows immediately and can choose not to commit the offset.

---

## 7. Test Evidence

### Integration Tests (Testcontainers)

All three resilience paths are covered by integration tests in `payment-service/internal/integration/payment_kafka_test.go`, run against real Kafka and Postgres containers:

**`TestDLQOnPoisonPill`** — malformed JSON routes straight to `payments.dlq`; `ProcessPayment` is never called; `payments.completed` stays empty.

```go
badPayload := []byte("NOT_JSON{{{")
publishMsg(t, brokerAddr, "orders.created", badPayload)

raw := consumeOne(t, brokerAddr, "payments.dlq", 5*time.Second)
require.NotNil(t, raw, "payments.dlq must receive the poison-pill message")

var dlq struct { ErrorStage string `json:"errorStage"` }
require.NoError(t, json.Unmarshal(raw, &dlq))
assert.Equal(t, "deserialize", dlq.ErrorStage)
assert.Nil(t, consumeOne(t, brokerAddr, "payments.completed", 500*time.Millisecond))
assert.False(t, called, "ProcessPayment should not have been invoked")
```

**`TestDLQAfterRetryExhaustion`** — a service that always returns `"db: connection lost"` exhausts all 4 attempts within ~700ms and routes to DLQ with `errorStage="process"`.

```go
svc := &stubSvc{fn: func(ctx context.Context, in service.ProcessPaymentInput) (*model.Payment, error) {
    return nil, errors.New("db: connection lost")
}}
// ...
raw := consumeOne(t, brokerAddr, "payments.dlq", 6*time.Second)
var dlq struct {
    ErrorStage string `json:"errorStage"`
    Attempts   int    `json:"attempts"`
}
assert.Equal(t, "process", dlq.ErrorStage)
assert.GreaterOrEqual(t, dlq.Attempts, 4, "expected at least 4 attempts before DLQ")
assert.Nil(t, consumeOne(t, brokerAddr, "payments.completed", 500*time.Millisecond))
```

**`TestPermanentDeclineNoDLQ`** — a service returning a FAILED payment publishes to `payments.failed` only; `payments.dlq` stays empty.

```go
// Verify payments.failed received the decline event.
raw := consumeOne(t, brokerAddr, "payments.failed", 5*time.Second)
require.NotNil(t, raw, "payments.failed must receive the decline event")
// DLQ must remain empty.
assert.Nil(t, consumeOne(t, brokerAddr, "payments.dlq", 500*time.Millisecond),
    "permanent decline must NOT produce a DLQ entry")
```

**`TestDuplicateDeliveryIdempotency`** — the same `orders.created` event is published 3 times (simulating at-least-once Kafka redelivery). Exactly one payment row is created and reaches a terminal state; `payments.dlq` stays empty.

```go
for i := 0; i < 3; i++ {
    require.NoError(t, w.WriteMessages(ctx, segkafka.Message{
        Key: []byte(orderID.String()), Value: payload,
    }))
}
time.Sleep(8 * time.Second)

var paymentCount int64
db.Model(&model.Payment{}).Where("order_id = ?", orderID).Count(&paymentCount)
assert.Equal(t, int64(1), paymentCount, "idempotency: exactly one payment row for duplicate events")

var terminalCount int64
db.Model(&model.Payment{}).
    Where("order_id = ? AND status IN ('COMPLETED','FAILED')", orderID).
    Count(&terminalCount)
assert.Equal(t, int64(1), terminalCount, "payment must reach a terminal state")
assert.Nil(t, consumeOne(t, brokerAddr, "payments.dlq", 500*time.Millisecond))
```

### Chaos Test: Payment-Service Kill Mid-Saga

`script/test/chaos_saga_kill.sh` kills payment-service at t+10s while 10 orders/s are in-flight, waits 5s, then restarts it. After a 30s settle window:

| Assertion | Expected | Observed |
|---|---|---|
| PENDING orders | 0 | **0** |
| DLQ delta (new messages) | 0 | **0** |
| Duplicate payment rows | 0 | **0** |
| Payments processed in window | — | **900** |

The script:
```bash
# Snapshot DLQ baseline offset before chaos
DLQ_BASELINE=$(docker exec ecommerce-kafka kafka-run-class kafka.tools.GetOffsetShell \
  --broker-list kafka:29092 --topic payments.dlq | awk -F: '{sum+=$3} END {print sum+0}')

# Kill at t+10s
sleep "$KILL_AT"
docker compose kill payment-service

# Restart after 5s
sleep "$RESTART_AFTER"
docker compose start payment-service

# Assert: 0 PENDING orders, 0 DLQ delta, 0 duplicates
PENDING=$(psql -c "SELECT count(*) FROM orders WHERE status='PENDING' AND created_at >= '${START_TS}'")
DLQ_DELTA=$((DLQ_FINAL - DLQ_BASELINE))
DUP_COUNT=$(psql -c "SELECT count(*) FROM (SELECT order_id FROM payments GROUP BY order_id HAVING count(*) > 1) t")
```

### Saga Latency (k6 Load Test)

`script/k6/saga_happy.js` and `script/k6/saga_fail.js` each run 10 VUs × 200 iterations, measuring TTC from `POST /orders` to terminal payment status:

| Scenario | Target | Observed |
|---|---|---|
| Happy path P95 TTC | < 2,000ms | **822ms** |
| Happy path avg TTC | — | **411ms** |
| Failure path P95 TTC | < 1,500ms | **442ms** |
| Compensation TTC P95 | < 2,000ms | **741ms** |

The async design contributes directly to these numbers: because `POST /orders` returns before any Kafka or gateway work begins, the HTTP response latency is decoupled from payment processing time. The saga completes in under 1 second at median for both paths.

---

## Summary

| Property | Mechanism | Evidence |
|---|---|---|
| Instant order response | Transactional outbox; Kafka publish off the HTTP hot path | OutboxPublisher polls every 100ms; `POST /orders` returns 201 immediately |
| Poison pill → DLQ | Deserialization failure routes before `ProcessPayment` | `TestDLQOnPoisonPill`: DLQ received, `ProcessPayment` never called |
| Transient retry | 4 attempts with 100/200/400ms backoff; DLQ after exhaustion | `TestDLQAfterRetryExhaustion`: `attempts ≥ 4`, `errorStage="process"` |
| Permanent decline → no DLQ | `classifyError` returns `errKindPermanent`; exits retry loop on attempt 1 | `TestPermanentDeclineNoDLQ`: `payments.failed` received, DLQ empty |
| Zero data loss | `CommitInterval: 0`; offset committed only after outcome published | Chaos test: 0 PENDING, 0 DLQ after kill+restart at 10 orders/s |
| Idempotent redelivery | UNIQUE constraint on `idempotency_key`; PENDING-resume path | `TestDuplicateDeliveryIdempotency`: 3× redelivery → 1 payment row |

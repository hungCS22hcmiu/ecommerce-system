# Payment Service — How It Works

---

## 1. What Is It?

The payment service is an autonomous Go microservice that processes payments **asynchronously** via a Kafka-choreographed saga and **synchronously** via direct HTTP.

**Analogy:** Think of it like a bank teller who doesn't just handle cash — they also listen to a ticket queue (Kafka), make sure they don't charge you twice for the same transaction (idempotency key), and leave a paper trail for every status change (audit history). If the ticket is unreadable or the charge fails too many times, it goes into a "problem tray" (DLQ) for human review.

---

## 2. Why It Matters

**In this project:**
- Decouples the order lifecycle from payment processing — order-service publishes `orders.created` and moves on; payment-service processes in the background and publishes the outcome.
- Prevents double-charges under concurrent retries via a DB-level `UNIQUE` constraint on `idempotency_key`.
- Preserves a full audit trail (`payment_history`) for every status transition.

**In real-world systems:**
- Payment is the most failure-sensitive path. A missed charge means lost revenue; a double charge means a chargeback and legal exposure.
- Async processing absorbs traffic spikes — a flash sale creates a burst of orders; workers drain the queue without blocking the checkout flow.
- Idempotency keys are the standard pattern used by Stripe, Braintree, and Adyen for exactly-once charging semantics over unreliable networks.

---

## 3. How It Works

### Happy Path (Kafka Saga)

```
order-service                  Kafka                    payment-service               order-service
     │                           │                            │                            │
     │── publish orders.created ─►│                            │                            │
     │                           │── fetch (worker pool) ────►│                            │
     │                           │                            │── Create PENDING payment   │
     │                           │                            │   (DB UNIQUE on idem. key) │
     │                           │                            │── gateway.Charge() ────────│
     │                           │                            │◄─ txnID returned ──────────│
     │                           │                            │── UpdateStatus → COMPLETED │
     │                           │◄── publish payments.completed ──────────────────────────│
     │                           │                            │── commit offset            │
     │◄── consume payments.completed ──────────────────────────────────────────────────────│
     │── transition order → CONFIRMED                                                       │
```

### Step-by-Step

1. **Consume** — `Consumer.Run` fetches from `orders.created` using `StartOffset=earliest` (at-least-once delivery). Messages are pushed onto a buffered channel (`cap=100`).

2. **Dispatch** — 5 goroutines (`KafkaWorkerCount`) pull from the channel. Each goroutine extracts or generates an `X-Correlation-ID` for end-to-end tracing.

3. **Deserialize** — If `json.Unmarshal` fails the message is a poison pill; it goes straight to `payments.dlq` with `errorStage: "deserialize"`, then the offset is committed (no redelivery of a permanently broken message).

4. **ProcessPayment** (idempotency dedup):
   - Build a `Payment` row with `Status=PENDING` and `IdempotencyKey=OrderID`.
   - `repo.Create` wraps insert + initial `payment_history` row in one transaction.
   - Postgres `UNIQUE` on `idempotency_key` fires → `ErrDuplicateIdempotencyKey` → return the existing row. The saga delivers the same outcome without a second charge.

5. **Charge** — `gateway.Charge` is called with a 5-second timeout. The mock gateway simulates 90% success with random latency.
   - `ErrGatewayDeclined` → `UpdateStatus → FAILED` → no retry (permanent decline).
   - Other error (context deadline, DB blip) → transient; worker retries up to 3× with exponential backoff: 100ms → 200ms → 400ms.

6. **Retry exhaustion** → DLQ with `errorStage: "process"` and attempt count. Offset is then committed.

7. **Publish outcome** — Worker calls `producer.PublishCompleted` or `PublishFailed`. Message is keyed by `orderId` (partition ordering) and carries `__TypeId__` header so Spring Kafka's `JsonDeserializer` resolves the target class without config changes on order-service.

8. **Commit offset** — Only after successful outcome publish. If publish fails, the payment row is persisted (idempotency handles a retry), so offset is still committed to avoid redelivery.

### Shutdown Sequence

```
SIGTERM
  │
  ├─ consumerCancel() → fetch loop exits → jobs channel closed → workers drain
  ├─ producer.Close() → flush buffered acks
  ├─ srv.Shutdown()  → drain in-flight HTTP requests
  └─ sqlDB.Close()
```
30-second deadline covers the entire drain. If exceeded, consumer is force-closed.

---

## 4. System Design

### Component Map

```
┌─────────────────────────────────────────────────────────────┐
│                     payment-service                         │
│                                                             │
│  HTTP (Gin :8003)          Kafka Consumer                   │
│  ┌──────────────────┐      ┌────────────────────────┐       │
│  │ PaymentHandler   │      │ Consumer                │       │
│  │  POST /payments  │      │  reader (orders.created)│       │
│  │  GET  /payments  │      │  jobs chan (cap 100)     │       │
│  │  GET  /:id       │      │  5 × runWorker          │       │
│  │  GET  /order/:id │      │  runLagLogger (30s)     │       │
│  └────────┬─────────┘      └────────────┬────────────┘       │
│           │                             │                    │
│           ▼                             ▼                    │
│  ┌──────────────────────────────────────────────────────┐   │
│  │             PaymentService                           │   │
│  │  ProcessPayment · GetByID · GetByOrderID · ListByUser│   │
│  └────────────┬───────────────────────┬─────────────────┘   │
│               │                       │                     │
│       ┌───────▼──────┐      ┌─────────▼──────┐             │
│       │  PaymentRepo │      │  MockGateway   │             │
│       │  (GORM/PG)   │      │  Charge()      │             │
│       └──────────────┘      └────────────────┘             │
│                                                             │
│  Kafka Producer                                             │
│  ┌──────────────────────────────────────────┐              │
│  │  payments.completed · payments.failed    │              │
│  │  payments.dlq                            │              │
│  └──────────────────────────────────────────┘              │
└─────────────────────────────────────────────────────────────┘
         │                          │
    Postgres                      Kafka
  ecommerce_payments            broker:29092
```

### Database Schema

```
payments
  id               UUID PK
  order_id         UUID UNIQUE       ← one payment per order
  user_id          UUID INDEX
  amount           NUMERIC(10,2)
  currency         CHAR(3)
  status           payment_status    ← PENDING | COMPLETED | FAILED | REFUNDED
  method           payment_method
  idempotency_key  VARCHAR(255) UNIQUE NOT NULL  ← dedup anchor
  gateway_reference VARCHAR(255)

payment_history
  id         BIGINT PK AUTOINCREMENT
  payment_id UUID NOT NULL INDEX
  old_status payment_status           ← nullable on first PENDING row
  new_status payment_status NOT NULL
  reason     TEXT
  created_at TIMESTAMP
```

### Kafka Topics

| Topic | Direction | Key | Purpose |
|---|---|---|---|
| `orders.created` | Consumed | orderId | Triggers payment saga |
| `payments.completed` | Published | orderId | Order-service → CONFIRMED |
| `payments.failed` | Published | orderId | Order-service → FAILED |
| `payments.dlq` | Published | — | Poison pills + exhausted retries |

---

## 5. Code Example

### Idempotency Dedup (Repository Layer)

```go
// repository/payment_repository.go
func (r *paymentRepository) Create(ctx context.Context, p *model.Payment, h *model.PaymentHistory) error {
    return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        if err := tx.Create(p).Error; err != nil {
            if isDuplicateKey(err) {  // Postgres SQLSTATE 23505
                return ErrDuplicateIdempotencyKey
            }
            return err
        }
        h.PaymentID = p.ID
        return tx.Create(h).Error
    })
}
```

**Why it works at scale:** The `UNIQUE` constraint is enforced at the database level — no application-layer locking or Redis coordination needed. 10 concurrent goroutines racing to insert the same `idempotency_key` will have exactly 1 winner; the other 9 get `ErrDuplicateIdempotencyKey` and return the winner's row.

### Retry + DLQ in the Worker

```go
// internal/kafka/consumer.go
var backoffs = []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond}

for ; attempts <= len(backoffs); attempts++ {
    payment, lastErr = c.svc.ProcessPayment(msgCtx, input)
    if lastErr == nil || classifyError(lastErr) == errKindPermanent {
        break
    }
    // transient: wait and retry
    select {
    case <-msgCtx.Done():
        return // don't commit on shutdown mid-retry
    case <-time.After(backoffs[attempts]):
    }
}

if lastErr != nil {
    c.sendToDLQ(msgCtx, msg, lastErr.Error(), "process", attempts+1, ...)
    c.reader.CommitMessages(msgCtx, msg)
    return
}
```

**Key design decision:** `classifyError` separates `ErrGatewayDeclined` (business-permanent, no retry) from everything else (transient, retry eligible). This avoids burning retries on a card that the gateway will always reject.

### `__TypeId__` Header for Spring Interop

```go
// internal/kafka/producer.go — PublishCompleted
msg := kafka.Message{
    Key:   []byte(evt.OrderID.String()),
    Value: body,
    Headers: []kafka.Header{
        {Key: "__TypeId__", Value: []byte(
            "com.ecommerce.order_service.kafka.event.PaymentCompletedEvent",
        )},
    },
}
```

Spring Kafka's `JsonDeserializer` uses `__TypeId__` to look up the target class. Without it, order-service would need `SPRING_KAFKA_CONSUMER_PROPERTIES_SPRING_JSON_USE_TYPE_HEADERS=false` and manual type mapping. Setting it from the Go producer side means zero config change on the consumer.

---

## 6. Trade-offs

### Choreography Saga vs. Orchestration

| | Choreography (this service) | Orchestration |
|---|---|---|
| **Coupling** | Services only know topics, not each other | Central orchestrator knows all steps |
| **Observability** | Harder — trace spans multiple services | Easier — one place to inspect saga state |
| **Failure isolation** | Each service owns its retry/DLQ logic | Orchestrator handles compensation centrally |
| **Scalability** | Each service scales independently | Orchestrator becomes a bottleneck |

Choreography is appropriate here because there are only 2 services in the saga. With 5+ services, an orchestrator (e.g., Temporal) is usually worth the complexity.

### Idempotency Key = OrderID

**Pro:** Zero coordination overhead — consumers don't need to generate or store a separate key.  
**Con:** If an order is legitimately retried with a different amount (e.g., partial fulfillment), the old payment row is returned unchanged. In production you'd use a client-generated key that encodes the intent version.

### At-Least-Once Delivery + DB Idempotency

**Pro:** Simple. The `UNIQUE` constraint is the only dedup mechanism — no Redis, no distributed lock.  
**Con:** The window between `Create` (Postgres write) and `CommitMessages` (Kafka offset advance) means on crash the message redelivers. The idempotency key handles the duplicate insert, but `gateway.Charge` will be called again. A real payment gateway is expected to be idempotent on its own reference ID — our mock is not.

### Worker Pool (Fixed 5 Goroutines)

**Pro:** Bounded concurrency — predictable DB connection usage.  
**Con:** `KafkaWorkerCount` is static. If one partition has a hotspot (a burst of large orders), workers on idle partitions can't help.

---

## 7. When to Use / Avoid

### Use This Pattern When

- **Money moves** — any financial transaction where double-processing has real cost.
- **Cross-service state machines** — the order lifecycle (PENDING → CONFIRMED/FAILED) is naturally a saga.
- **Burst workloads** — the Kafka buffer absorbs order spikes; workers drain at their own pace.
- **At-least-once is acceptable** — as long as your processing function is idempotent.

### Avoid / Reconsider When

- **You need exactly-once semantics end-to-end** — Kafka transactions + idempotent producers are required; the current setup is at-least-once.
- **The saga has more than 3–4 steps** — choreography becomes a spaghetti of events; switch to orchestration (Temporal, Conductor).
- **Sub-100ms latency SLA** — async Kafka processing adds latency proportional to consumer lag. Synchronous HTTP with a distributed lock is faster for low-volume, latency-sensitive flows.
- **Partial payment / split-order scenarios** — the 1:1 `order_id → payment` schema and `idempotency_key=orderId` assumption breaks down.

---

## 8. Interview Insights

### Q: How do you prevent double-charging if the Kafka consumer crashes after charging but before committing the offset?

**A:** The `idempotency_key` (set to `orderId`) has a `UNIQUE` constraint in Postgres. On redeliver, `repo.Create` returns `ErrDuplicateIdempotencyKey`, and `ProcessPayment` returns the existing payment row. The real-world risk is that `gateway.Charge` fires again before the DB constraint fires — which is why production payment gateways are themselves idempotent on the reference ID you pass. Our mock does not simulate this, which is a documented limitation.

### Q: Why use DB-level dedup instead of Redis?

**A:** Redis is an ephemeral cache; if it evicts the key or restarts, the dedup guarantee disappears. A Postgres `UNIQUE` constraint is durable by definition and participates in the same transaction as the payment insert. One fewer dependency, one fewer failure mode.

### Q: How does the DLQ differ from a retry topic?

**A:** A retry topic re-enqueues the message with a delay for re-processing (similar to SQS visibility timeout). A DLQ is a terminal destination — we've given up and need human intervention. This service routes to DLQ after 4 attempts (initial + 3 backoffs) for transient errors, and immediately for permanent errors (`ErrGatewayDeclined`) and poison pills (deserialize failure). A production system would add a separate retry topic between the two.

### Q: What happens if `payments.completed` publish fails after the payment is persisted?

**A:** The offset is still committed (line 214 in `consumer.go`). The rationale: the payment row is in Postgres with `COMPLETED` status. If we don't commit, Kafka redelivers, `idempotency_key` dedup returns the same `COMPLETED` row, and we retry the publish — which is fine. Committing even on publish failure avoids that retry at the cost of order-service never getting the `payments.completed` event. In a production system you'd want an outbox pattern or at least a reconciliation job to detect `COMPLETED` payments with no corresponding order transition.

### Q: Why is `StartOffset=earliest` set on the consumer?

**A:** It maps to Kafka's `auto.offset.reset=earliest`. When the consumer group has no committed offset (e.g., first deployment, or the group was deleted), it replays from the beginning of the partition rather than skipping to the latest message. This is the safe default for financial processing — you'd rather re-process an already-idempotent message than silently drop an order.

### Q: How does the `__TypeId__` header solve the Go ↔ Spring Kafka interop problem?

**A:** Spring Kafka's `JsonDeserializer` by default uses the `__TypeId__` header to resolve the target Java class. Without it, the consumer needs `spring.json.use.type.headers=false` and an explicit `value-default-type` mapping in `application.yml` — a config change on every new event type. By setting the header from the Go producer side, order-service can use the standard Spring auto-configuration with zero extra properties.

# ADR: Saga Resilience — Error Triage, Retry, and DLQ

## Context

The payment-service consumes `orders.created` via a Kafka choreography saga. Processing is
at-least-once delivery: if the consumer crashes mid-message, the uncommitted message is
redelivered on restart. The idempotency key (= `orderId.String()`) stored in a DB `UNIQUE`
column prevents double-charging on redelivery (see `locking-strategy.md §5`).

At the consumer boundary, three distinct failure modes require different handling:

| Failure mode | Example | Right response |
|---|---|---|
| Poison (deserialization failure) | `{"bad":"json"` — truncated | DLQ immediately, no retry |
| Transient | DB connection blip, context deadline exceeded | Retry with backoff, DLQ after exhaustion |
| Permanent business decline | Gateway declined the charge | Emit `payments.failed`, commit — saga healthy |

---

## Decision: Three-tier error triage

### Tier 1 — Poison pill → DLQ immediately

If `json.Unmarshal` fails, the message is permanently malformed. Retrying is futile and blocks
the partition. The consumer routes it to `payments.dlq` with `errorStage="deserialize"` and
the original bytes base64-encoded, then commits and moves on. The consumer stays alive.

### Tier 2 — Transient error → retry + DLQ on exhaustion

Transient failures (DB blips, network timeouts, `context.DeadlineExceeded`) are retried up to
3 times with exponential backoff: **100ms / 200ms / 400ms** (4 total attempts). The same
idempotency key is used on every attempt — if attempt 1 partially succeeded (gateway charged
but `UpdateStatus` failed), attempt 2 finds the existing row via `ErrDuplicateIdempotencyKey`
and returns it without re-charging. After exhaustion, the message goes to `payments.dlq` with
`errorStage="process"` and `attempts` set to the number of calls made.

If the DLQ publish itself fails, the message is NOT committed — it will be redelivered on
restart. This is intentional: silent loss is worse than redelivery.

### Tier 3 — Permanent business decline → payments.failed, no DLQ

`ErrGatewayDeclined` is a healthy saga outcome, not an error. The service returns a `FAILED`
payment with nil error. The consumer calls `publishOutcome(FAILED)`, which sends the event to
`payments.failed`. Order-service receives it and cancels the order. Nothing goes to DLQ.

**Why not retry permanent declines?** The gateway has already made a decision. A retry would
re-invoke `Charge` against a stateful gateway, potentially double-charging. The correct
compensation is order cancellation, not payment retry.

---

## Graceful Shutdown

On SIGTERM:
1. Cancel the consumer context → fetch loop exits.
2. `jobs` channel is closed → workers drain remaining in-flight messages.
3. A 30-second deadline is applied to the drain. If exceeded, the consumer is force-closed
   (with a `slog.Warn`). Unprocessed messages will be redelivered on restart.
4. Kafka writers are flushed and closed.
5. HTTP server accepts no new requests and drains in-flight handlers.
6. DB pool is closed.

The retry backoff `select` listens on `ctx.Done()`, so a shutdown during a sleep exits cleanly
without committing the in-progress message.

---

## At-least-once vs. exactly-once

Exactly-once Kafka semantics require transactional producers + consumers on both sides,
`RequiredAcks=all`, and careful offset management. The added complexity is not justified here:

- The idempotency key (`UNIQUE(idempotency_key)` on the `payments` table) provides the same
  correctness guarantee for the duplicate-message case.
- The concurrent insert test (`TestConcurrentIdempotency`) proves this at the DB layer.
- The Kafka integration test (`TestDuplicateDeliveryIdempotency`) proves it through the full
  consumer stack.

---

## Consumer Lag Alert Threshold

A goroutine logs `reader.Stats()` every 30 seconds. If `lag > 10,000` messages, `slog.Warn`
fires instead of `slog.Info`. This threshold was chosen as an upper bound for the backlog that
would take >30s to drain at a sustained rate of 333 msg/s. Adjust the threshold in the
`runLagLogger` constant if throughput requirements change.

---

## DLQ Inspection

```bash
# Read all DLQ messages (from the beginning)
docker exec ecommerce-kafka kafka-console-consumer \
  --bootstrap-server localhost:29092 --topic payments.dlq \
  --from-beginning --timeout-ms 5000

# Count DLQ depth
docker exec ecommerce-kafka kafka-consumer-groups \
  --bootstrap-server localhost:29092 --group payment-service --describe
```

Each DLQ message is a JSON envelope with the original bytes (base64), error stage, reason,
attempt count, timestamp, and correlation ID. This provides enough context to diagnose and
manually replay the original message after fixing the root cause.

---

## Consequences

- All transient errors are retried up to 3× before escalation — reduces false-positive DLQ
  entries from transient infrastructure blips.
- Poison pills never block the partition — they are forwarded to DLQ within one processing
  cycle.
- Permanent gateway declines are a normal saga outcome and do not generate alerts.
- The DLQ is append-only and inspectable at any time.
- Manual replay: publish the base64-decoded `originalValue` back to `orders.created`. The
  idempotency key prevents double-charging if the original payment row already exists.

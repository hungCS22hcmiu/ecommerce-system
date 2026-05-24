# Concurrent Workloads

## Overview

The three Go services — payment-service, cart-service, user-service — use goroutines and channels throughout, not just at the HTTP layer. This document explains every non-trivial concurrent workload: what goroutines are spawned, how they communicate, how they stop cleanly, and why each design choice was made.

The primary example is the **payment-service Kafka consumer**, which combines all three primitives in one coherent design: a goroutine pool fed by a buffered channel, a background monitor goroutine, and a context-based shutdown sequence bounded by a 30-second deadline.

---

## Goroutine Inventory

| Service | Goroutine | Trigger | Lifecycle |
|---|---|---|---|
| payment-service | HTTP server | startup | `srv.Shutdown(ctx)` — drains in-flight requests |
| payment-service | Kafka fetch loop | startup | `consumerCancel()` → `reader.FetchMessage` returns |
| payment-service | 5× Kafka workers | startup | `close(jobs)` channel → range loop exits |
| payment-service | Lag monitor | startup | `ctx.Done()` select arm exits |
| payment-service | pprof server | startup | process-lifetime (non-critical) |
| cart-service | HTTP server | startup | `srv.Shutdown(ctx)` |
| cart-service | Redis→Postgres sync | startup | `syncCancel()` → `ctx.Done()` select arm exits |
| user-service | HTTP server | startup | `srv.Shutdown(ctx)` |
| user-service | `runtime.NumCPU()` bcrypt workers | startup | `close(done)` → select arm exits |

---

## 1. Kafka Consumer Worker Pool (payment-service)

### Why a pool instead of one goroutine per message

`kafka-go` delivers messages one at a time from `FetchMessage`. Processing each message involves a DB write, a mock gateway HTTP call (50–200ms simulated latency), and a Kafka publish. A single goroutine would process roughly 5 messages per second — far below the needed throughput. Spawning one goroutine per message creates unbounded concurrency and can exhaust DB connections.

A **bounded pool of 5 workers** sharing a buffered channel gives concurrency control with a fixed DB connection budget: at most 5 concurrent `ProcessPayment` calls, each holding at most one Postgres connection from the `MaxOpenConns=25` pool.

### Structure

`payment-service/internal/kafka/consumer.go`
```go
type Consumer struct {
    reader   *kafka.Reader
    producer *Producer
    svc      service.PaymentService
    cfg      *config.Config
    jobs     chan kafka.Message   // buffered capacity 100 — absorbs bursts
    wg       sync.WaitGroup      // tracks live worker goroutines
}
```

The `jobs` channel buffers up to 100 messages. This lets the fetch loop run ahead of the workers during bursts without blocking, while back-pressure is applied naturally if all workers are busy and the buffer fills.

### Startup

`payment-service/internal/kafka/consumer.go` — `Run()`
```go
func (c *Consumer) Run(ctx context.Context) {
    for i := 0; i < c.cfg.KafkaWorkerCount; i++ {  // KafkaWorkerCount = 5
        c.wg.Add(1)
        go c.runWorker(ctx, i)
    }

    go c.runLagLogger(ctx)   // separate monitoring goroutine

    // Fetch loop — runs on the calling goroutine
    for {
        msg, err := c.reader.FetchMessage(ctx)
        if err != nil {
            break  // ctx cancelled → normal shutdown
        }
        c.jobs <- msg   // blocks if all 100 slots are full (back-pressure)
    }

    close(c.jobs)   // signal: no more messages coming
    c.wg.Wait()     // wait for every worker to finish its current message
    c.reader.Close()
}
```

```
main goroutine
│
├── go consumer.Run(consumerCtx)
│     ├── go runWorker(ctx, 0)  ─┐
│     ├── go runWorker(ctx, 1)   │  5 workers, each ranging over jobs channel
│     ├── go runWorker(ctx, 2)   │
│     ├── go runWorker(ctx, 3)   │
│     ├── go runWorker(ctx, 4)  ─┘
│     ├── go runLagLogger(ctx)
│     └── [fetch loop]  FetchMessage → jobs <- msg
│
└── [signal wait]
```

### Worker logic

`payment-service/internal/kafka/consumer.go` — `runWorker()`
```go
func (c *Consumer) runWorker(ctx context.Context, workerID int) {
    defer c.wg.Done()
    for msg := range c.jobs {      // exits when channel is closed and drained
        c.processMessage(ctx, msg, workerID)
    }
}
```

`range c.jobs` blocks when the channel is empty and returns when it is both closed and empty. This guarantees every message pushed before `close(c.jobs)` is processed — no messages are silently dropped.

### Per-message processing with retry

Each message goes through four stages. The retry loop uses exponential backoff only for transient errors:

`payment-service/internal/kafka/consumer.go` — `processMessage()`
```go
var backoffs = []time.Duration{100ms, 200ms, 400ms}  // 3 retries, initial + 3 = 4 attempts max

for ; attempts <= len(backoffs); attempts++ {
    payment, lastErr = c.svc.ProcessPayment(msgCtx, input)
    if lastErr == nil || classifyError(lastErr) == errKindPermanent {
        break   // success or permanent decline — stop immediately
    }
    // transient: wait backoff[attempts] or exit if ctx is cancelled
    select {
    case <-msgCtx.Done():
        return  // shutdown mid-retry — don't commit offset
    case <-time.After(backoffs[attempts]):
    }
}
```

Error classification:

| Error | Kind | Outcome |
|---|---|---|
| `gateway.ErrGatewayDeclined` | permanent | Publish `payments.failed`, no retry, no DLQ |
| DB error, context deadline | transient | Retry up to 3× with 100/200/400ms backoff |
| Retry exhausted | — | Route to `payments.dlq`, commit offset |
| Deserialization error | poison pill | Route to `payments.dlq` immediately |

### Offset commit discipline

`CommitInterval: 0` disables auto-commit. The offset is committed manually, **after** the outcome event has been published to Kafka:

`payment-service/internal/kafka/consumer.go` — `processMessage()`
```go
// Only reached if ProcessPayment succeeded and outcome published
if commitErr := c.reader.CommitMessages(msgCtx, msg); commitErr != nil { ... }
```

Three cases skip the commit intentionally:

1. **DLQ publish failed** — return without commit; the message will be re-delivered on restart
2. **Payment still PENDING after `publishOutcome`** — gateway call may not have completed; skip commit, force redelivery, resume path will retry the gateway
3. **ctx cancelled mid-retry** — return without commit; re-delivered from last committed offset after restart

`payment-service/internal/kafka/consumer.go` — `publishOutcome()`
```go
default:
    // PENDING: return error so caller skips CommitMessages
    return fmt.Errorf("payment %s still PENDING after ProcessPayment", payment.ID)
```

This is the PENDING-resume guarantee: even if the service is killed between the DB write and the Kafka publish, the same message is re-delivered, and `ProcessPayment` detects the existing PENDING row via `ErrDuplicateIdempotencyKey` and retries the gateway using the same payment ID.

---

## 2. Consumer Lag Monitor Goroutine

`runLagLogger` is started once per `Consumer.Run` call and runs for the entire consumer lifetime:

`payment-service/internal/kafka/consumer.go` — `runLagLogger()`
```go
func (c *Consumer) runLagLogger(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return   // consumer context cancelled — exit cleanly
        case <-ticker.C:
            stats := c.reader.Stats()
            highWatermark := stats.Offset + stats.Lag
            fields := []any{
                "topic",         stats.Topic,
                "partition",     stats.Partition,
                "lag",           stats.Lag,
                "offset",        stats.Offset,
                "highWatermark", highWatermark,
            }
            if stats.Lag > 10_000 {
                slog.Warn("kafka.lag: consumer lag above alert threshold", fields...)
            } else {
                slog.Info("kafka.lag", fields...)
            }
        }
    }
}
```

The two-arm `select` is the idiomatic Go pattern for a cancellable ticker loop:

- **`<-ticker.C`** fires every 30 seconds and logs current lag
- **`<-ctx.Done()`** fires when `consumerCancel()` is called during shutdown and exits the goroutine

`defer ticker.Stop()` ensures the ticker's internal goroutine is released and its channel is no longer sent to after the function returns.

The 10,000-message alert threshold matches the Phase 2 load test observation where peak consumer lag reached 7,318 messages at 10 orders/s sustained load — just below the alert level under normal conditions.

---

## 3. Graceful Shutdown Within 30-Second Deadline

All three Go services follow the same shutdown protocol. The payment-service is the most complex because it has a consumer pool to drain in addition to the HTTP server.

### Signal capture

`payment-service/cmd/server/main.go`
```go
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
<-quit   // block until docker stop / Ctrl+C
```

The buffered channel (size 1) prevents signal loss if the runtime delivers the signal before the receive.

### Ordered shutdown sequence

`payment-service/cmd/server/main.go`
```go
slog.Info("shutting down payment-service...")
shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

// Step 1 — Stop consumer: cancel fetch loop, wait for workers to drain
consumerCancel()
consumerDone := make(chan struct{})
go func() { consumer.Wait(); close(consumerDone) }()

select {
case <-consumerDone:
    slog.Info("consumer drained cleanly")
case <-shutdownCtx.Done():
    slog.Warn("shutdown deadline exceeded — forcing consumer close")
}

// Step 2 — Flush Kafka producer (buffered acks)
producer.Close()

// Step 3 — Drain in-flight HTTP requests
if err := srv.Shutdown(shutdownCtx); err != nil {
    slog.Error("server shutdown error", "error", err)
}

// Step 4 — Close DB connection pool
sqlDB.Close()
```

The ordering matters:

1. **Consumer first** — stops fetching new messages and drains the `jobs` channel, so no new `ProcessPayment` calls start after shutdown begins
2. **Producer second** — flushes any Kafka writes still buffered (outcome events for messages already processed by workers)
3. **HTTP server third** — stops accepting new requests and drains in-flight handlers that may be doing DB reads
4. **DB last** — nothing is using connections by this point

### Why the `consumerDone` goroutine pattern

`consumer.Wait()` blocks until all workers call `wg.Done()`. If workers are mid-gateway-call (50–200ms latency) when shutdown begins, they may take several hundred milliseconds to finish. Wrapping `Wait()` in a goroutine and racing it against `shutdownCtx.Done()` gives the deadline enforcement without blocking the main goroutine unconditionally:

```
consumerCancel() called
    │
    ▼
fetch loop: FetchMessage(ctx) returns error → break
    │
    ▼
close(c.jobs)
    │
    ├── worker 0: finishes current message → range exits → wg.Done()
    ├── worker 1: finishes current message → range exits → wg.Done()
    ├── worker 2: finishes current message → range exits → wg.Done()
    ├── worker 3: finishes current message → range exits → wg.Done()
    └── worker 4: finishes current message → range exits → wg.Done()
                                                           │
                                                           ▼
                                                    consumer.Wait() returns
                                                    close(consumerDone)
                                                    ← main select receives
```

If the deadline fires instead, no worker is forcibly killed — they continue running until `wg.Done()` is called, but the main goroutine proceeds with the producer and HTTP shutdown steps without waiting for them.

---

## 4. bcrypt Worker Pool (user-service)

bcrypt at cost factor 10 takes ~100ms of CPU per call. Under concurrent login load, spawning one goroutine per request would saturate all CPU cores with bcrypt work, starving the HTTP server goroutines and blocking all other operations.

A bounded pool of `runtime.NumCPU()` workers serializes bcrypt through a fixed number of goroutines, leaving remaining cores free for other work.

### Channel-per-request reply pattern

`user-service/pkg/password/pool.go`
```go
type verifyRequest struct {
    hash     string
    password string
    reply    chan error   // one channel per caller — carries the result back
}

type Pool struct {
    jobs chan verifyRequest   // buffered 256 — queue of pending verifications
    done chan struct{}        // closed on Stop() to exit all workers
}
```

`user-service/pkg/password/pool.go` — `Verify()`
```go
func (p *Pool) Verify(ctx context.Context, hash, password string) error {
    reply := make(chan error, 1)   // buffered 1 — worker never blocks on send

    // Non-blocking enqueue: if queue is full, shed the load immediately
    select {
    case p.jobs <- verifyRequest{hash, password, reply}:
    default:
        return ErrBcryptOverload   // HTTP handler returns 503 + Retry-After: 1
    }

    // Wait for result or context cancellation
    select {
    case err := <-reply:
        return err
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

`user-service/pkg/password/pool.go` — `worker()`
```go
func (p *Pool) worker() {
    for {
        select {
        case <-p.done:
            return
        case req := <-p.jobs:
            req.reply <- bcrypt.CompareHashAndPassword([]byte(req.hash), []byte(req.password))
        }
    }
}
```

The two-select structure in `Verify` achieves two independent goals:

- **Non-blocking send** (`default` arm): prevents request goroutines from queuing indefinitely behind a full bcrypt pool — they get a fast 503 instead of a 60s timeout
- **ctx-aware receive**: if the client disconnects while waiting, the request goroutine unblocks immediately rather than holding the reply channel open

Workers do not leak: `Stop()` closes `p.done`, and each worker's `select` will fire the `<-p.done` arm on its next iteration.

### Shutdown ordering (user-service)

`user-service/cmd/server/main.go`
```go
<-quit   // SIGINT / SIGTERM

shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
defer shutdownCancel()

srv.Shutdown(shutdownCtx)   // drain HTTP — no new Verify() calls after this
bcryptPool.Stop()           // workers exit cleanly, no in-flight calls remain
sqlDB.Close()
rdb.Close()
```

`bcryptPool.Stop()` is called **after** `srv.Shutdown` completes, guaranteeing that no handler goroutine is mid-`Verify` when the pool is torn down.

---

## 5. Cart Sync Background Goroutine (cart-service)

Cart writes go to Redis first (low latency) and are periodically persisted to Postgres as a durability backstop. A single background goroutine handles this without coupling the write path to Postgres latency.

`cart-service/internal/cache/sync.go` — `StartSyncWorker()`
```go
func StartSyncWorker(
    ctx context.Context,
    rdb *redis.Client,
    redisRepo repository.RedisCartRepository,
    cartRepo repository.CartRepository,
) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            syncAll(ctx, rdb, redisRepo, cartRepo)
        case <-ctx.Done():
            return
        }
    }
}
```

`syncAll` scans `cart:*` keys from Redis in pages of 100, calls `cartRepo.ReplaceItems` (full replace, not merge) for each, and logs aggregate synced/failed counts. Individual cart sync failures are logged and skipped — one user's cart failure never blocks the others.

`cart-service/cmd/server/main.go`
```go
syncCtx, syncCancel := context.WithCancel(context.Background())
go cache.StartSyncWorker(syncCtx, rdb, redisRepo, cartRepo)
defer syncCancel()   // called on SIGINT/SIGTERM before HTTP shutdown
```

---

## Design Patterns Summary

| Pattern | Where used | Problem solved |
|---|---|---|
| Buffered channel as work queue | Kafka consumer `jobs` (cap 100) | Decouples fetch rate from processing rate; absorbs bursts |
| Bounded goroutine pool + WaitGroup | Kafka workers (5), bcrypt workers (NumCPU) | Caps concurrency; enables clean drain on shutdown |
| Channel-per-request reply | bcrypt `Verify()` | Routes result back to caller without shared state |
| Non-blocking channel send | bcrypt `Verify()` enqueue | Fast load shedding — 503 instead of queue buildup |
| `select { case <-ctx.Done() ... }` | lag monitor, sync worker, bcrypt workers | Cooperative cancellation — no goroutine leaks |
| `close(channel)` to broadcast stop | `close(c.jobs)`, `close(p.done)` | Single-sender, N-receiver shutdown signal |
| Deadline-bounded `select` | consumer drain in shutdown | Prevents indefinite block; guarantees exit within 30s |
| `defer ticker.Stop()` | lag monitor, sync worker | Releases ticker goroutine; prevents channel send after return |

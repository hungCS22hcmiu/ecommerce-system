# Phase 4 — Testing Debt: Implementation Plan

> Companion to [`testing_plan.md`](./testing_plan.md) §Phase 4 (also `testing_target.md` §9). Closes unit/integration testing gaps in security utilities, repositories, and clients.

## Context

Phases 1–3 surfaced 14 improvement items but Phase 4 is independent: it's about test code coverage, not perf. An audit found:

| # | Debt area | Current coverage | Action |
|---|---|---|---|
| 1 | PaymentRepository idempotency (N concurrent same-key inserts) | `payment_idempotency_test.go:TestConcurrentIdempotency` at N=10 | **EXT** bump to N=20 |
| 2 | Kafka consumer DLQ branches (poison / transient / permanent decline) | Already 4 tests in `payment_kafka_test.go` | ✓ done in Phase 1 |
| 3 | Cart ProductClient CB state transitions CLOSED→OPEN→HALF_OPEN→CLOSED | CB-open covered, recovery not | **NEW** test added to `product_contract_test.go` |
| 4 | user-service/pkg/blacklist (revocation TTL, idempotent re-revoke, expiry) | No tests | **NEW** test file |
| 5 | user-service/pkg/verification (entropy, cooldown, attempt-counter expiry) | No tests | **NEW** test file |
| 6 | user-service/pkg/reset (token TTL, single-use, reuse rejected) | No tests | **NEW** test file |
| 7 | ProductRepository FTS + pgvector edge cases (empty results, dimension mismatch) | Ranking tests only | **EXT** new IT class |
| 8 | OrderRepository pessimistic lock blocking | `OrderConcurrencyTest` | ✓ done |
| 9 | Product EmbeddingClient unit test (timeout / 500 / 200) | Service-level fallback covered, client direct not | **NEW** unit test |

**Decisions confirmed:**
- Redis in pkg tests: **miniredis** in-process (`github.com/alicebob/miniredis/v2`). No Docker, ms per test.
- Coverage: **report current numbers**, don't enforce. Run `go test -coverprofile` + parse; record in `test_result.md`. No JaCoCo, no CI gate.
- Scope: **all 6 areas in one run**.

## Artifacts

| File | Action | Test target |
|---|---|---|
| `payment-service/internal/integration/payment_idempotency_test.go` | EXT | bump `concurrency := 10` → `20`; assert 1 success + 19 dup errors |
| `cart-service/internal/integration/product_contract_test.go` | EXT | new `TestProductContract_CircuitBreakerHalfOpenThenClose` — drive 5 failures (OPEN), wait timeout (HALF_OPEN), single probe 200 → CLOSED |
| `user-service/pkg/blacklist/blacklist_test.go` | NEW | TTL respected, re-revoke idempotent, expired key removed |
| `user-service/pkg/verification/store_test.go` | NEW | code generated within range, cooldown enforced, attempt-counter expires |
| `user-service/pkg/reset/store_test.go` | NEW | token TTL respected, single-use, reuse rejected |
| `product-service/src/test/java/.../integration/ProductRepositoryQueryIT.java` | NEW | FTS empty result + tsquery escaping + pgvector no-match + dimension mismatch |
| `product-service/src/test/java/.../client/EmbeddingClientTest.java` | NEW | WireMock 500 → `AIServiceException`; WireMock delay > timeout → `AIServiceException`; 200 → vector returned |
| `script/test/phase4_run.sh` + `script/test/aggregate_phase4.py` | NEW | run all test commands, parse `go test -json` + Maven Surefire XML, append Phase 4 section to `test_result.md` with PASS/FAIL + coverage % |

## Execution

```bash
bash script/test/phase4_run.sh           # ≈ 5–8 min full run
cat docs/testing/test_result.md          # Phase 4 section appended
```

Per-target independent runs documented in `phase4_run.sh` summary.

# Phase 5 — Frontend UX & Reliability: Implementation Plan

> Companion to [`testing_plan.md`](./testing_plan.md) §Phase 5 (testing_target.md §8). Adds the first test runner + suite to the React frontend.

## Context

Phase 1 audit confirmed: **zero test runners installed** in `frontend/`. No vitest, no playwright, no @testing-library, no `data-testid` attributes. Phase 5 closes this gap with a Vitest unit suite plus a minimal Playwright suite for the backend-agnostic UX targets.

**Decisions confirmed:**
- **Scope:** Vitest unit tests for the 3 §8 reliability paths (axios queue, optimistic cart, toast surfacing) + Playwright **responsive + a11y** specs (no backend dependency, won't flake on IMP-6 / IMP-8). The auth / cart / refresh E2E specs are deferred — they'd surface known Phase 1/2 findings instead of new ones.
- Test runner: **Vitest** (config block extends `vite.config.ts`, no separate `vitest.config.ts`).
- DOM env: **happy-dom** (smaller + faster than jsdom for these tests).
- E2E browser: Chromium only (the responsive + a11y specs don't need cross-browser).
- Coverage: **report numbers only**, no enforcement (mirrors Phase 4 convention).

---

## Artifacts

### Vitest unit suite (3 files)

| File | Verifies | Source code |
|---|---|---|
| `frontend/src/lib/__tests__/axios.test.ts` | 401 interceptor queues 3 concurrent failed requests, mocks single `/auth/refresh`, replays all 3 with new token, zero `/login` redirects. (§8.C) | `frontend/src/lib/axios.ts` (queue logic lines 14–57) |
| `frontend/src/features/cart/__tests__/useCart.test.ts` | TanStack optimistic mutation increments `useCartStore.itemCount` then rolls back on API error within 2s. (§8.B) | `frontend/src/features/cart/useCart.ts:addItem` (lines 23–36) |
| `frontend/src/lib/__tests__/toast.test.ts` | Backend `error.message` ("Password must be at least 8 characters") rendered verbatim — not a generic fallback. (§8.A) | `frontend/src/lib/toast.ts` (showToast/subscribeToast) |
| `frontend/src/__tests__/setup.ts` | Vitest setup — extends `expect` with jest-dom matchers, resets axios module state between tests | NEW |

### Playwright E2E (2 specs, no backend dependency)

| File | Verifies | Pages |
|---|---|---|
| `frontend/tests/e2e/responsive.spec.ts` | 320px viewport — no horizontal overflow on Home / Cart / Product list. (§8.D) | `/`, `/products`, `/cart` |
| `frontend/tests/e2e/a11y.spec.ts` | `@axe-core/playwright` scan — zero serious violations on Home / Products. (§8.D) | `/`, `/products` |
| `frontend/playwright.config.ts` | Chromium-only, baseURL `http://localhost:3001`, retries=1 | NEW |

### Wiring

| File | Action |
|---|---|
| `frontend/package.json` | Add devDeps + `test`/`test:e2e` scripts |
| `frontend/vite.config.ts` | Add `test:` config block (happy-dom env, globals, setupFiles) |

### Orchestrator + aggregator

| File | Role |
|---|---|
| `script/test/phase5_run.sh` | `npm install` (if needed) → `npm test` (Vitest) → `npx playwright install --with-deps chromium` (once) → `npm run test:e2e` → aggregate |
| `script/test/aggregate_phase5.py` | Parses Vitest JSON reporter output + Playwright JSON reporter, appends Phase 5 section to `test_result.md` |

---

## Execution

```bash
bash script/test/phase5_run.sh        # ≈ 4–6 min (1st run includes browser download)
cat docs/testing/test_result.md        # Phase 5 section appended
```

Per-target independent runs:

| Target | Command |
|---|---|
| §8.A toast | `cd frontend && npx vitest run src/lib/__tests__/toast.test.ts` |
| §8.B optimistic cart | `cd frontend && npx vitest run src/features/cart/__tests__/useCart.test.ts` |
| §8.C axios queue | `cd frontend && npx vitest run src/lib/__tests__/axios.test.ts` |
| §8.D responsive | `cd frontend && npx playwright test responsive.spec.ts` |
| §8.D a11y | `cd frontend && npx playwright test a11y.spec.ts` |

**Expected findings:**
- §8.A toast — PASS (the source code passes through `error.message`).
- §8.B optimistic cart — PASS (TanStack `onError` rollback is already wired).
- §8.C axios queue — PASS, but may surface subtle ordering bug if the failedQueue resolves in wrong order.
- §8.D a11y — likely **AT_RISK / FAIL**: components use Tailwind without explicit `aria-label`s; axe is strict.
- §8.D responsive — likely **PASS** on Home/Products (Tailwind is mobile-first), **FAIL on Cart** if the drawer overflows on 320px.

---

## Out of Scope

- Backend-coupled Playwright specs (auth, cart, refresh) — deferred; they'd surface IMP-6 / IMP-8.
- Coverage enforcement / thresholds — report only.
- CI integration.
- Visual regression testing.

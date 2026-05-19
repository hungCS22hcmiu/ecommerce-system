// Vitest setup: extend expect with @testing-library/jest-dom matchers + reset
// module-level state between tests (axios interceptor isRefreshing/queue, toast
// listeners, Zustand stores).
import '@testing-library/jest-dom/vitest'
import { afterEach, vi } from 'vitest'

// crypto.randomUUID is required by toast.ts but happy-dom 20 exposes it; fall back
// to a stable polyfill if missing so test runs aren't environment-dependent.
if (typeof crypto !== 'undefined' && typeof crypto.randomUUID !== 'function') {
  // @ts-expect-error — minimal polyfill
  crypto.randomUUID = () => 'test-' + Math.random().toString(36).slice(2)
}

afterEach(() => {
  vi.clearAllMocks()
})

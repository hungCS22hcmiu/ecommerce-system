// Phase 5 §8.C — Axios 401 interceptor queue.
//
// Test contract: when three independent in-flight requests all receive 401
// concurrently, only ONE refresh call goes out, and all three originals are
// replayed with the new token. window.location.href must NOT be reassigned.
//
// We don't hit a real network — instead we install a custom axios adapter on
// the singleton `api` instance that returns canned responses based on URL.

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import type { AxiosAdapter, AxiosRequestConfig, AxiosResponse } from 'axios'

import { api } from '@/lib/axios'
import { useAuthStore } from '@/store/authStore'

// ── helpers ──────────────────────────────────────────────────────────────────

interface MockBehavior {
  refreshCalls: number
  originalCallsByUrl: Map<string, number>
  // Which token each call observed in the Authorization header
  tokensSeenByUrl: Map<string, string[]>
}

function makeAdapter(b: MockBehavior): AxiosAdapter {
  return async (cfg: AxiosRequestConfig): Promise<AxiosResponse> => {
    const url = cfg.url ?? ''
    const auth = (cfg.headers as Record<string, string> | undefined)?.['Authorization'] ?? ''

    if (url.includes('/auth/refresh')) {
      b.refreshCalls++
      return {
        data: { success: true, data: { access_token: 'refreshed-token-xyz' } },
        status: 200,
        statusText: 'OK',
        headers: {},
        config: cfg as never,
      }
    }

    // Track every other call.
    b.originalCallsByUrl.set(url, (b.originalCallsByUrl.get(url) ?? 0) + 1)
    const arr = b.tokensSeenByUrl.get(url) ?? []
    arr.push(auth)
    b.tokensSeenByUrl.set(url, arr)

    // First attempt: 401. Replay (i.e. second attempt) with the refreshed token: 200.
    const isReplay = auth === 'Bearer refreshed-token-xyz'
    if (!isReplay) {
      // Reject with axios-shaped error so the response interceptor sees error.response.status === 401.
      const err: Error & { response?: unknown; config?: unknown } = new Error('401 Unauthorized')
      err.response = {
        data: { success: false, error: { code: 'UNAUTHORIZED' } },
        status: 401,
        statusText: 'Unauthorized',
        headers: {},
        config: cfg,
      }
      err.config = cfg
      throw err
    }
    return {
      data: { ok: true, url },
      status: 200,
      statusText: 'OK',
      headers: {},
      config: cfg as never,
    }
  }
}

describe('axios — 401 interceptor queue (§8.C)', () => {
  let originalAdapter: AxiosAdapter | undefined
  let originalHref: PropertyDescriptor | undefined
  let hrefSet: ReturnType<typeof vi.fn>

  beforeEach(() => {
    // Seed initial auth state.
    useAuthStore.setState({
      accessToken: 'initial-stale-token',
      refreshToken: 'rt-1',
      userId: 'u-1',
      email: 'u@x',
      role: 'customer',
      _isInitialized: true,
    } as never)

    // Spy on window.location.href assignment.
    hrefSet = vi.fn()
    originalHref = Object.getOwnPropertyDescriptor(window.location, 'href')
    Object.defineProperty(window.location, 'href', {
      configurable: true,
      get: () => 'http://localhost/',
      set: (v: string) => hrefSet(v),
    })

    originalAdapter = api.defaults.adapter as AxiosAdapter | undefined
  })

  afterEach(() => {
    api.defaults.adapter = originalAdapter
    if (originalHref) {
      Object.defineProperty(window.location, 'href', originalHref)
    }
  })

  it('queues 3 concurrent 401 requests, refreshes once, replays all three with new token', async () => {
    const b: MockBehavior = {
      refreshCalls: 0,
      originalCallsByUrl: new Map(),
      tokensSeenByUrl: new Map(),
    }
    api.defaults.adapter = makeAdapter(b)

    const [r1, r2, r3] = await Promise.all([
      api.get('/products/1'),
      api.get('/products/2'),
      api.get('/products/3'),
    ])

    expect(r1.status).toBe(200)
    expect(r2.status).toBe(200)
    expect(r3.status).toBe(200)

    // Exactly ONE refresh call across the three concurrent 401s.
    expect(b.refreshCalls).toBe(1)

    // Each original URL was hit twice: 1st time 401, 2nd time after replay.
    for (const url of ['/products/1', '/products/2', '/products/3']) {
      expect(b.originalCallsByUrl.get(url)).toBe(2)
      const tokens = b.tokensSeenByUrl.get(url) ?? []
      expect(tokens[0]).toBe('Bearer initial-stale-token')
      expect(tokens[1]).toBe('Bearer refreshed-token-xyz')
    }

    // No /login redirect on success path.
    expect(hrefSet).not.toHaveBeenCalled()

    // Auth store was updated with the new access token.
    expect(useAuthStore.getState().accessToken).toBe('refreshed-token-xyz')
  })

  it('on refresh failure, clears auth and redirects to /login', async () => {
    // Refresh fails with 500 (not 401) so the interceptor's catch path fires
    // cleanly without re-entering the refresh logic. NOTE — IMP candidate:
    // /auth/refresh is NOT in the interceptor's bypass list (only /auth/login
    // and /auth/register), so a 401 on refresh would re-enter the interceptor
    // and deadlock; this 500 path side-steps that.
    api.defaults.adapter = async (cfg: AxiosRequestConfig) => {
      const url = cfg.url ?? ''
      const status = url.includes('/auth/refresh') ? 500 : 401
      const err: Error & { response?: unknown; config?: unknown } = new Error(`status ${status}`)
      err.response = { data: {}, status, statusText: 'Err', headers: {}, config: cfg }
      err.config = cfg
      throw err
    }

    await expect(api.get('/products/1')).rejects.toBeDefined()

    expect(useAuthStore.getState().accessToken).toBe(null)
    expect(hrefSet).toHaveBeenCalledWith('/login')
  })
})

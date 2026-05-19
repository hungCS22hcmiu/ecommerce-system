// Phase 5 §8.A — Backend error message must be surfaced verbatim through the
// toast pub/sub, not replaced with a generic "An error occurred" fallback.

import { describe, it, expect, beforeEach, vi } from 'vitest'
import { showToast, subscribeToast, type Toast } from '@/lib/toast'

describe('toast — error surfacing (§8.A)', () => {
  let received: Toast[]
  let unsubscribe: () => void

  beforeEach(() => {
    received = []
    unsubscribe = subscribeToast((t) => received.push(t))
  })

  it('passes through the exact backend error.message verbatim', () => {
    const backendMessage = 'Password must be at least 8 characters'

    showToast(backendMessage, 'error')

    expect(received).toHaveLength(1)
    expect(received[0].message).toBe(backendMessage)
    expect(received[0].level).toBe('error')
    expect(received[0].id).toMatch(/.+/)

    unsubscribe()
  })

  it('does not transform messages with special characters', () => {
    const messages = [
      'Cart item "Widget Pro" is out of stock.',
      "Order #ABCD-1234 — payment <failed>",
      'rate limit: 10 req/s exceeded',
    ]
    for (const msg of messages) showToast(msg, 'error')

    expect(received.map((t) => t.message)).toEqual(messages)
    unsubscribe()
  })

  it('multiple subscribers each receive the same toast', () => {
    const second: Toast[] = []
    const unsub2 = subscribeToast((t) => second.push(t))

    showToast('hello', 'info')

    expect(received).toHaveLength(1)
    expect(second).toHaveLength(1)
    expect(received[0].id).toBe(second[0].id)

    unsubscribe()
    unsub2()
  })

  it('after unsubscribe, listener stops receiving', () => {
    unsubscribe()
    showToast('gone', 'error')
    expect(received).toHaveLength(0)
  })

  it('default level is "info" when not specified', () => {
    showToast('no level here')
    expect(received[0].level).toBe('info')
    unsubscribe()
  })

  it('toast subscriber receives within microtask (synchronous publish)', () => {
    // Verify that showToast does NOT defer; the listener fires before the
    // next tick. This matters because the toast UI assumes immediate delivery.
    const spy = vi.fn()
    const unsub = subscribeToast(spy)
    showToast('sync', 'error')
    expect(spy).toHaveBeenCalledTimes(1)
    unsub()
    unsubscribe()
  })
})

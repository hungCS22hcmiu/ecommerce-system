// Phase 5 §8.B — TanStack optimistic cart mutation.
// Asserts addItem increments useCartStore.itemCount immediately, then rolls back
// the increment within 2s when the underlying API call rejects.

import { describe, it, expect, beforeEach, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement, type PropsWithChildren } from 'react'

import { useCartStore } from '@/store/cartStore'
import { useCartMutations } from '@/features/cart/useCart'

// Mock the cart API so we control resolve/reject timing.
vi.mock('@/features/cart/cartApi', () => ({
  cartApi: {
    addItem: vi.fn(),
    updateItem: vi.fn(),
    removeItem: vi.fn(),
    clearCart: vi.fn(),
    getCart: vi.fn(),
  },
}))

import { cartApi } from '@/features/cart/cartApi'

function wrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return ({ children }: PropsWithChildren) =>
    createElement(QueryClientProvider, { client }, children)
}

describe('useCart — optimistic addItem (§8.B)', () => {
  beforeEach(() => {
    useCartStore.getState().setItemCount(0)
    vi.clearAllMocks()
  })

  it('increments itemCount immediately on mutate', async () => {
    ;(cartApi.addItem as ReturnType<typeof vi.fn>).mockResolvedValue({
      success: true,
      data: { items: [], total: 0 },
    })

    const { result } = renderHook(() => useCartMutations(), { wrapper: wrapper() })
    expect(useCartStore.getState().itemCount).toBe(0)

    result.current.addItem.mutate({ product_id: 1, quantity: 1 })

    // Increment must happen synchronously in onMutate, before the API resolves.
    await waitFor(() => expect(useCartStore.getState().itemCount).toBe(1))
  })

  it('rolls back the increment within 2s when the API rejects', async () => {
    // Delay the rejection so we can observe the optimistic increment before
    // the rollback fires.
    ;(cartApi.addItem as ReturnType<typeof vi.fn>).mockImplementation(
      () =>
        new Promise((_resolve, reject) =>
          setTimeout(() => reject(new Error('boom: backend down')), 100),
        ),
    )

    const { result } = renderHook(() => useCartMutations(), { wrapper: wrapper() })
    expect(useCartStore.getState().itemCount).toBe(0)

    const t0 = Date.now()
    result.current.addItem.mutate({ product_id: 1, quantity: 1 })

    // Optimistic increment fires immediately in onMutate.
    await waitFor(() => expect(useCartStore.getState().itemCount).toBe(1))

    // Rollback must complete inside 2s of mutation start.
    await waitFor(
      () => expect(useCartStore.getState().itemCount).toBe(0),
      { timeout: 2000 },
    )

    expect(Date.now() - t0).toBeLessThan(2000)
  })

  it('on success, optimistic increment stays applied', async () => {
    ;(cartApi.addItem as ReturnType<typeof vi.fn>).mockResolvedValue({
      success: true,
      data: { items: [{ product_id: 1, quantity: 1, subtotal: 9.99, unit_price: 9.99 }], total: 9.99 },
    })

    const { result } = renderHook(() => useCartMutations(), { wrapper: wrapper() })
    await new Promise<void>((resolve, reject) => {
      result.current.addItem.mutate({ product_id: 1, quantity: 1 }, {
        onSuccess: () => resolve(),
        onError: (e) => reject(e),
      })
    })

    // onSuccess implies no rollback — itemCount still 1 (until invalidate refetches).
    expect(useCartStore.getState().itemCount).toBeGreaterThanOrEqual(1)
  })
})

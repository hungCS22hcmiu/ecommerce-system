import { Link } from 'react-router-dom'
import { useQueries } from '@tanstack/react-query'
import { useCart, useCartMutations } from '@/features/cart/useCart'
import { CartItem } from '@/features/cart/CartItem'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/shared/EmptyState'
import { formatCurrency } from '@/lib/utils'
import { productApi } from '@/features/products/productApi'

export function CartPage() {
  const { data, isLoading, isError } = useCart()
  const { clearCart } = useCartMutations()
  const cart = data?.data
  const items = [...(cart?.items ?? [])].sort((a, b) => a.product_id - b.product_id)

  const productQueries = useQueries({
    queries: items.map((item) => ({
      queryKey: ['product', item.product_id],
      queryFn: () => productApi.getById(item.product_id),
      staleTime: 60_000,
    })),
  })

  const stockMap: Record<number, number> = {}
  productQueries.forEach((q, i) => {
    if (q.data?.data) stockMap[items[i].product_id] = q.data.data.stockAvailable
  })

  const hasStockWarning = items.some(
    (item) => stockMap[item.product_id] !== undefined && item.quantity > stockMap[item.product_id],
  )

  if (isLoading) {
    return (
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        <p className="text-fg-subtle text-sm">Loading cart…</p>
      </div>
    )
  }

  if (isError) {
    return (
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        <p className="text-status-failed text-sm">Failed to load cart. Please try again.</p>
      </div>
    )
  }

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      <h1 className="font-display text-3xl text-fg-base mb-8">Your Cart</h1>

      {items.length === 0 ? (
        <EmptyState
          title="Your cart is empty"
          description="Add some products to get started."
          action={
            <Link to="/products">
              <Button>Browse Products</Button>
            </Link>
          }
        />
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
          {/* Items column */}
          <div className="lg:col-span-2">
            {hasStockWarning && (
              <div className="mb-4 px-4 py-3 bg-amber-500/10 border border-amber-500/30 rounded-lg text-xs text-amber-500">
                Some items in your cart exceed current available stock. Update quantities before checking out.
              </div>
            )}
            <div className="bg-surface-raised border border-surface-border rounded-lg divide-y divide-surface-border px-5">
              {items.map((item) => (
                <CartItem key={item.product_id} item={item} stockAvailable={stockMap[item.product_id]} />
              ))}
            </div>

            <button
              type="button"
              onClick={() => clearCart.mutate()}
              disabled={clearCart.isPending}
              className="mt-4 text-xs text-fg-subtle hover:text-status-failed transition-colors disabled:opacity-40"
            >
              Clear cart
            </button>
          </div>

          {/* Summary column */}
          <div className="lg:col-span-1">
            <div className="bg-surface-raised border border-surface-border rounded-lg p-5 space-y-4">
              <h2 className="font-semibold text-fg-base text-sm">Order Summary</h2>

              <div className="space-y-2 text-sm">
                <div className="flex justify-between">
                  <span className="text-fg-muted">
                    Subtotal ({items.reduce((n, i) => n + i.quantity, 0)} items)
                  </span>
                  <span className="font-mono text-fg-base">{formatCurrency(cart?.total ?? 0)}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-fg-muted">Shipping</span>
                  <span className="font-mono text-status-delivered">Free</span>
                </div>
              </div>

              <div className="border-t border-surface-border pt-3 flex justify-between">
                <span className="text-fg-base font-semibold">Total</span>
                <span className="font-mono text-accent font-semibold text-lg">
                  {formatCurrency(cart?.total ?? 0)}
                </span>
              </div>

              <Link to="/checkout">
                <Button className="w-full mt-2">Proceed to Checkout →</Button>
              </Link>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

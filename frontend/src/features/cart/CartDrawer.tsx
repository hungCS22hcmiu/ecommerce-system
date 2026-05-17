import { Link } from 'react-router-dom'
import { useQueries } from '@tanstack/react-query'
import { useCart, useCartMutations } from './useCart'
import { CartItem } from './CartItem'
import { Button } from '@/components/ui/button'
import { formatCurrency } from '@/lib/utils'
import { productApi } from '@/features/products/productApi'

interface CartDrawerProps {
  open: boolean
  onClose: () => void
}

export function CartDrawer({ open, onClose }: CartDrawerProps) {
  const { data, isLoading } = useCart()
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

  return (
    <>
      {/* Backdrop */}
      {open && (
        <div
          className="fixed inset-0 bg-black/60 z-40"
          onClick={onClose}
        />
      )}

      {/* Drawer */}
      <div
        className={`fixed top-0 right-0 h-full w-80 sm:w-96 bg-surface-overlay border-l border-surface-border z-50 flex flex-col transition-transform duration-300 ${
          open ? 'translate-x-0' : 'translate-x-full'
        }`}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-surface-border">
          <h2 className="font-display text-lg text-fg-base">Your Cart</h2>
          <button
            type="button"
            onClick={onClose}
            className="text-fg-subtle hover:text-fg-base transition-colors text-xl leading-none"
          >
            ✕
          </button>
        </div>

        {/* Items */}
        <div className="flex-1 overflow-y-auto px-5">
          {isLoading ? (
            <p className="text-sm text-fg-subtle py-8 text-center">Loading cart…</p>
          ) : items.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-16 gap-3">
              <p className="text-fg-subtle text-sm">Your cart is empty</p>
              <Link to="/products" onClick={onClose}>
                <Button variant="outline" size="sm">Browse Products</Button>
              </Link>
            </div>
          ) : (
            <div className="divide-y divide-surface-border">
              {items.map((item) => (
                <CartItem key={item.product_id} item={item} stockAvailable={stockMap[item.product_id]} />
              ))}
            </div>
          )}
        </div>

        {/* Footer */}
        {items.length > 0 && (
          <div className="px-5 py-4 border-t border-surface-border space-y-3">
            <div className="flex justify-between text-sm">
              <span className="text-fg-muted">Total</span>
              <span className="font-mono text-fg-base font-medium">
                {formatCurrency(cart?.total ?? 0)}
              </span>
            </div>

            <Link to="/checkout" onClick={onClose} className="block">
              <Button className="w-full">Proceed to Checkout →</Button>
            </Link>

            <Link to="/cart" onClick={onClose} className="block">
              <Button variant="outline" className="w-full">View Cart</Button>
            </Link>

            <button
              type="button"
              onClick={() => clearCart.mutate()}
              disabled={clearCart.isPending}
              className="w-full text-xs text-fg-subtle hover:text-status-failed transition-colors disabled:opacity-40"
            >
              Clear cart
            </button>
          </div>
        )}
      </div>
    </>
  )
}

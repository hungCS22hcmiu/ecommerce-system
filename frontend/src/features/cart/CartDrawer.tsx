import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
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
  const navigate = useNavigate()
  const { data, isLoading } = useCart()
  const { clearCart } = useCartMutations()
  const cart = data?.data
  const items = [...(cart?.items ?? [])].sort((a, b) => a.product_id - b.product_id)

  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set())

  const productQueries = useQueries({
    queries: items.map((item) => ({
      queryKey: ['product', item.product_id],
      queryFn: () => productApi.getById(item.product_id),
      staleTime: 60_000,
    })),
  })

  const stockMap: Record<number, number> = {}
  const sellerByProductId: Record<number, string> = {}
  productQueries.forEach((q, i) => {
    if (q.data?.data) {
      stockMap[items[i].product_id] = q.data.data.stockAvailable
      sellerByProductId[items[i].product_id] = q.data.data.sellerId
    }
  })

  function toggleItem(productId: number) {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      next.has(productId) ? next.delete(productId) : next.add(productId)
      return next
    })
  }

  const selectedItems = items.filter((i) => selectedIds.has(i.product_id))
  const uniqueSellerIds = [
    ...new Set(selectedItems.map((i) => sellerByProductId[i.product_id]).filter(Boolean)),
  ]
  const isMultiSeller = uniqueSellerIds.length > 1
  const canCheckout = selectedIds.size > 0 && !isMultiSeller

  function handleCheckout() {
    if (!canCheckout) return
    onClose()
    navigate('/checkout', { state: { items: selectedItems } })
  }

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
            aria-label="Close cart"
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
                <div key={item.product_id} className="flex items-start gap-2">
                  <input
                    type="checkbox"
                    className="mt-4 flex-shrink-0 accent-amber-500 cursor-pointer"
                    checked={selectedIds.has(item.product_id)}
                    onChange={() => toggleItem(item.product_id)}
                    aria-label={`Select ${item.product_name}`}
                  />
                  <div className="flex-1">
                    <CartItem item={item} stockAvailable={stockMap[item.product_id]} />
                  </div>
                </div>
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

            {isMultiSeller && (
              <p className="text-xs text-status-failed text-center">
                Select items from one seller only to checkout together.
              </p>
            )}

            <Button
              className="w-full"
              disabled={!canCheckout}
              onClick={handleCheckout}
            >
              {selectedIds.size === 0
                ? 'Select items to checkout'
                : `Checkout ${selectedIds.size} item${selectedIds.size !== 1 ? 's' : ''} →`}
            </Button>

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

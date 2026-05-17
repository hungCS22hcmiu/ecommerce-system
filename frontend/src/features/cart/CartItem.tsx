import { Link } from 'react-router-dom'
import { useCartMutations } from './useCart'
import { formatCurrency } from '@/lib/utils'
import type { CartItem as CartItemType } from '@/types/cart'

interface CartItemProps {
  item: CartItemType
  stockAvailable?: number
  thumbnailUrl?: string
}

export function CartItem({ item, stockAvailable, thumbnailUrl }: CartItemProps) {
  const { updateItem, removeItem } = useCartMutations()

  const isOutOfStock = stockAvailable !== undefined && stockAvailable === 0
  const isAtMax = stockAvailable !== undefined && item.quantity >= stockAvailable

  return (
    <div className="flex items-start gap-3 py-3">
      <Link to={`/products/${item.product_id}`} className="flex-shrink-0">
        <div className="w-14 h-14 bg-surface-overlay border border-surface-border rounded overflow-hidden">
          {thumbnailUrl ? (
            <img src={thumbnailUrl} alt={item.product_name} className="w-full h-full object-cover" />
          ) : (
            <div className="w-full h-full flex items-center justify-center">
              <span className="text-fg-subtle text-xs">IMG</span>
            </div>
          )}
        </div>
      </Link>

      <div className="flex-1 min-w-0">
        <Link to={`/products/${item.product_id}`} className="hover:text-accent transition-colors">
          <p className="text-sm text-fg-base truncate font-medium">{item.product_name}</p>
        </Link>
        <p className="text-xs font-mono text-accent mt-0.5">{formatCurrency(item.unit_price)}</p>

        {isOutOfStock && (
          <p className="text-xs text-status-failed mt-1">Out of stock</p>
        )}

        <div className="flex items-center gap-2 mt-2">
          <div className="flex items-center border border-surface-border rounded overflow-hidden">
            <button
              type="button"
              onClick={() => {
                if (item.quantity <= 1) {
                  removeItem.mutate(item.product_id)
                } else {
                  updateItem.mutate({ productId: item.product_id, quantity: item.quantity - 1 })
                }
              }}
              disabled={updateItem.isPending || removeItem.isPending || isOutOfStock}
              className="w-7 h-7 flex items-center justify-center text-fg-muted hover:text-fg-base hover:bg-surface-raised disabled:opacity-40 transition-colors text-sm"
            >
              −
            </button>
            <span className="w-8 text-center text-xs font-mono text-fg-base">{item.quantity}</span>
            <button
              type="button"
              onClick={() =>
                updateItem.mutate({ productId: item.product_id, quantity: item.quantity + 1 })
              }
              disabled={updateItem.isPending || isAtMax || isOutOfStock}
              className="w-7 h-7 flex items-center justify-center text-fg-muted hover:text-fg-base hover:bg-surface-raised disabled:opacity-40 transition-colors text-sm"
            >
              +
            </button>
          </div>

          {isAtMax && !isOutOfStock && (
            <span className="text-xs text-amber-500 font-medium">Max</span>
          )}

          <span className="text-xs font-mono text-fg-muted ml-auto">
            {formatCurrency(item.subtotal)}
          </span>

          <button
            type="button"
            onClick={() => removeItem.mutate(item.product_id)}
            disabled={removeItem.isPending}
            className="text-fg-subtle hover:text-status-failed transition-colors text-xs disabled:opacity-40"
          >
            ✕
          </button>
        </div>
      </div>
    </div>
  )
}

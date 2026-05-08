import { useCartMutations } from './useCart'
import { formatCurrency } from '@/lib/utils'
import type { CartItem as CartItemType } from '@/types/cart'

interface CartItemProps {
  item: CartItemType
}

export function CartItem({ item }: CartItemProps) {
  const { updateItem, removeItem } = useCartMutations()

  return (
    <div className="flex items-start gap-3 py-3">
      <div className="w-14 h-14 bg-surface-overlay border border-surface-border rounded flex-shrink-0 flex items-center justify-center">
        <span className="text-zinc-700 text-xs">IMG</span>
      </div>

      <div className="flex-1 min-w-0">
        <p className="text-sm text-white truncate font-medium">{item.product_name}</p>
        <p className="text-xs font-mono text-accent mt-0.5">{formatCurrency(item.unit_price)}</p>

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
              disabled={updateItem.isPending || removeItem.isPending}
              className="w-7 h-7 flex items-center justify-center text-zinc-400 hover:text-white hover:bg-surface-raised disabled:opacity-40 transition-colors text-sm"
            >
              −
            </button>
            <span className="w-8 text-center text-xs font-mono text-white">{item.quantity}</span>
            <button
              type="button"
              onClick={() =>
                updateItem.mutate({ productId: item.product_id, quantity: item.quantity + 1 })
              }
              disabled={updateItem.isPending}
              className="w-7 h-7 flex items-center justify-center text-zinc-400 hover:text-white hover:bg-surface-raised disabled:opacity-40 transition-colors text-sm"
            >
              +
            </button>
          </div>

          <span className="text-xs font-mono text-zinc-400 ml-auto">
            {formatCurrency(item.subtotal)}
          </span>

          <button
            type="button"
            onClick={() => removeItem.mutate(item.product_id)}
            disabled={removeItem.isPending}
            className="text-zinc-600 hover:text-status-failed transition-colors text-xs disabled:opacity-40"
          >
            ✕
          </button>
        </div>
      </div>
    </div>
  )
}

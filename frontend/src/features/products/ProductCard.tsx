import { Link, useNavigate } from 'react-router-dom'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { StarRating } from '@/components/ui/StarRating'
import { formatCurrency } from '@/lib/utils'
import { showToast } from '@/lib/toast'
import { useCartMutations } from '@/features/cart/useCart'
import { useAuthStore } from '@/store/authStore'
import type { Product } from '@/types/product'

function stockBadge(stock: number) {
  if (stock === 0) return <Badge variant="red">Out of Stock</Badge>
  if (stock <= 5) return <Badge variant="amber">Low Stock ({stock} left)</Badge>
  return <Badge variant="emerald">In Stock</Badge>
}

interface ProductCardProps {
  product: Product
}

export function ProductCard({ product }: ProductCardProps) {
  const outOfStock = product.stockAvailable === 0
  const { addItem } = useCartMutations()
  const accessToken = useAuthStore((s) => s.accessToken)
  const navigate = useNavigate()

  function handleAddToCart(e: React.MouseEvent) {
    e.preventDefault()
    if (!accessToken) {
      navigate('/login?from=/products')
      return
    }
    addItem.mutate(
      { product_id: product.id, quantity: 1 },
      {
        onSuccess: () => showToast('Added to cart', 'success'),
        onError: (err: unknown) => {
          const code = (err as { response?: { data?: { code?: string } } })?.response?.data?.code
          if (code === 'SELLER_CANNOT_BUY_OWN_PRODUCT') {
            showToast('You cannot purchase your own products', 'error')
          } else if (code === 'INSUFFICIENT_STOCK') {
            showToast('Not enough stock available', 'error')
          } else {
            showToast('Failed to add to cart', 'error')
          }
        },
      },
    )
  }

  return (
    <div className="bg-surface-raised border border-surface-border rounded-lg overflow-hidden flex flex-col group">
      <Link to={`/products/${product.id}`} className="block">
        {product.thumbnailUrl ? (
          <img
            src={product.thumbnailUrl}
            alt={product.name}
            className="w-full aspect-square object-cover group-hover:opacity-90 transition-opacity"
          />
        ) : (
          <div className="w-full aspect-square bg-surface-overlay flex items-center justify-center">
            <span className="text-fg-muted text-sm">No image</span>
          </div>
        )}
      </Link>

      <div className="p-4 flex flex-col gap-2 flex-1">
        <Link to={`/products/${product.id}`}>
          <h3 className="font-display text-fg-base truncate hover:text-accent transition-colors leading-tight">
            {product.name}
          </h3>
        </Link>

        <p className="font-mono text-accent text-lg font-medium">
          {formatCurrency(product.price)}
        </p>

        {product.avgRating != null && product.ratingCount > 0 && (
          <div className="flex items-center gap-1 mt-1">
            <StarRating value={product.avgRating} size="sm" />
            <span className="text-xs text-fg-subtle">({product.ratingCount})</span>
          </div>
        )}

        <div className="flex items-center justify-between mt-auto pt-2">
          {stockBadge(product.stockAvailable)}
        </div>

        <Button
          className="w-full mt-1"
          disabled={outOfStock || addItem.isPending}
          onClick={handleAddToCart}
        >
          {addItem.isPending ? 'Adding…' : 'Add to Cart'}
        </Button>
      </div>
    </div>
  )
}

export function ProductCardSkeleton() {
  return (
    <div className="bg-surface-raised border border-surface-border rounded-lg overflow-hidden flex flex-col">
      <Skeleton className="w-full aspect-square rounded-none" />
      <div className="p-4 flex flex-col gap-3">
        <Skeleton className="h-5 w-3/4" />
        <Skeleton className="h-5 w-1/3" />
        <Skeleton className="h-5 w-1/2" />
        <Skeleton className="h-9 w-full mt-1" />
      </div>
    </div>
  )
}

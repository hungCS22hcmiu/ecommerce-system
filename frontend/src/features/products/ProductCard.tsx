import { Link } from 'react-router-dom'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { formatCurrency } from '@/lib/utils'
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
            <span className="text-zinc-700 text-sm">No image</span>
          </div>
        )}
      </Link>

      <div className="p-4 flex flex-col gap-2 flex-1">
        <Link to={`/products/${product.id}`}>
          <h3 className="font-display text-white truncate hover:text-accent transition-colors leading-tight">
            {product.name}
          </h3>
        </Link>

        <p className="font-mono text-accent text-lg font-medium">
          {formatCurrency(product.price)}
        </p>

        <div className="flex items-center justify-between mt-auto pt-2">
          {stockBadge(product.stockAvailable)}
        </div>

        <Button className="w-full mt-1" disabled={outOfStock}>
          Add to Cart
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

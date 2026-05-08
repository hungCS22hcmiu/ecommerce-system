import { ProductCard, ProductCardSkeleton } from './ProductCard'
import type { Product } from '@/types/product'

interface ProductGridProps {
  products?: Product[]
  isLoading: boolean
  count?: number
}

export function ProductGrid({ products, isLoading, count = 8 }: ProductGridProps) {
  return (
    <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
      {isLoading
        ? Array.from({ length: count }).map((_, i) => <ProductCardSkeleton key={i} />)
        : products?.map((p) => <ProductCard key={p.id} product={p} />)}
    </div>
  )
}

import { useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useProduct } from '@/features/products/useProducts'
import { useCartMutations } from '@/features/cart/useCart'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { formatCurrency } from '@/lib/utils'

function StockBadge({ stock }: { stock: number }) {
  if (stock === 0) return <Badge variant="red">Out of Stock</Badge>
  if (stock <= 5) return <Badge variant="amber">Low Stock ({stock} left)</Badge>
  return <Badge variant="emerald">In Stock</Badge>
}

export function ProductDetailPage() {
  const { id } = useParams<{ id: string }>()
  const { data, isLoading, isError } = useProduct(Number(id))
  const { addItem } = useCartMutations()
  const [qty, setQty] = useState(1)

  const product = data?.data

  if (isError) {
    return (
      <div className="max-w-7xl mx-auto px-4 py-12 text-center">
        <p className="text-status-failed mb-4">Product not found or failed to load.</p>
        <Link to="/products" className="text-accent hover:text-accent-dim text-sm">
          ← Back to products
        </Link>
      </div>
    )
  }

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      <Link
        to="/products"
        className="text-sm text-zinc-500 hover:text-white mb-6 inline-block transition-colors"
      >
        ← Back to products
      </Link>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-10 mt-2">
        {/* Image */}
        <div className="bg-surface-raised border border-surface-border rounded-lg overflow-hidden aspect-square flex items-center justify-center">
          {isLoading ? (
            <Skeleton className="w-full h-full rounded-none" />
          ) : product?.images?.[0]?.url ? (
            <img
              src={product.images[0].url}
              alt={product.images[0].altText ?? product.name}
              className="w-full h-full object-cover"
            />
          ) : (
            <span className="text-zinc-700 text-sm">No image</span>
          )}
        </div>

        {/* Info */}
        <div className="flex flex-col gap-4">
          {isLoading ? (
            <>
              <Skeleton className="h-10 w-3/4" />
              <Skeleton className="h-8 w-1/3" />
              <Skeleton className="h-4 w-full" />
              <Skeleton className="h-4 w-5/6" />
            </>
          ) : product ? (
            <>
              <h1 className="font-display text-4xl text-white leading-tight">{product.name}</h1>

              <p className="font-mono text-accent text-3xl font-medium">
                {formatCurrency(product.price)}
              </p>

              <div className="border-t border-surface-border pt-4">
                {product.description && (
                  <p className="text-sm text-zinc-400 leading-relaxed mb-4">
                    {product.description}
                  </p>
                )}

                <div className="flex items-center gap-3 mb-5">
                  <StockBadge stock={product.stockAvailable} />
                  <span className="text-xs text-zinc-600 font-mono">
                    {product.categoryName}
                  </span>
                </div>

                {/* Qty selector */}
                <div className="flex items-center gap-3 mb-5">
                  <span className="text-sm text-zinc-400">Quantity</span>
                  <div className="flex items-center border border-surface-border rounded-md overflow-hidden">
                    <button
                      type="button"
                      onClick={() => setQty((q) => Math.max(1, q - 1))}
                      disabled={qty <= 1 || product.stockAvailable === 0}
                      className="w-9 h-9 flex items-center justify-center text-zinc-400 hover:text-white hover:bg-surface-raised disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                    >
                      −
                    </button>
                    <span className="w-10 text-center text-sm font-mono text-white">{qty}</span>
                    <button
                      type="button"
                      onClick={() => setQty((q) => Math.min(product.stockAvailable, q + 1))}
                      disabled={qty >= product.stockAvailable || product.stockAvailable === 0}
                      className="w-9 h-9 flex items-center justify-center text-zinc-400 hover:text-white hover:bg-surface-raised disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                    >
                      +
                    </button>
                  </div>
                </div>

                <Button
                  className="w-full sm:w-auto px-8"
                  disabled={product.stockAvailable === 0 || addItem.isPending}
                  onClick={() =>
                    addItem.mutate({ product_id: product.id, quantity: qty })
                  }
                >
                  {addItem.isPending ? 'Adding…' : 'Add to Cart'}
                </Button>
              </div>
            </>
          ) : null}
        </div>
      </div>
    </div>
  )
}

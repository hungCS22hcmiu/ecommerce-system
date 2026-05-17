import { useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useProduct } from '@/features/products/useProducts'
import { useCartMutations } from '@/features/cart/useCart'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { formatCurrency } from '@/lib/utils'
import { showToast } from '@/lib/toast'

function StockBadge({ stock }: { stock: number }) {
  const safe = Math.max(0, stock)
  if (safe === 0) return <Badge variant="red">Out of Stock</Badge>
  if (safe <= 5) return <Badge variant="amber">Only {safe} left</Badge>
  return <Badge variant="emerald">{safe} available</Badge>
}

export function ProductDetailPage() {
  const { id } = useParams<{ id: string }>()
  const { data, isLoading, isError } = useProduct(Number(id))
  const { addItem } = useCartMutations()
  const [qty, setQty] = useState(1)
  const [inputVal, setInputVal] = useState('1')
  const [selectedImg, setSelectedImg] = useState(0)

  function applyQty(raw: string, max: number) {
    const parsed = parseInt(raw, 10)
    const clamped = Number.isNaN(parsed) ? 1 : Math.min(Math.max(1, parsed), max)
    setQty(clamped)
    setInputVal(String(clamped))
  }

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
        className="text-sm text-fg-subtle hover:text-fg-base mb-6 inline-block transition-colors"
      >
        ← Back to products
      </Link>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-10 mt-2">
        {/* Image gallery */}
        <div className="flex flex-col gap-3">
          <div className="bg-surface-raised border border-surface-border rounded-lg overflow-hidden aspect-square flex items-center justify-center">
            {isLoading ? (
              <Skeleton className="w-full h-full rounded-none" />
            ) : product?.images?.[selectedImg]?.url ? (
              <img
                src={product.images[selectedImg].url}
                alt={product.images[selectedImg].altText ?? product.name}
                className="w-full h-full object-cover"
              />
            ) : (
              <span className="text-fg-subtle text-sm">No image</span>
            )}
          </div>
          {product && product.images.length > 1 && (
            <div className="flex gap-2">
              {product.images.map((img, i) => (
                <button
                  key={i}
                  type="button"
                  onClick={() => setSelectedImg(i)}
                  className={`w-16 h-16 rounded-md overflow-hidden border-2 transition-colors flex-shrink-0 ${
                    i === selectedImg
                      ? 'border-accent'
                      : 'border-surface-border hover:border-fg-subtle'
                  }`}
                >
                  <img
                    src={img.url}
                    alt={img.altText ?? `Image ${i + 1}`}
                    className="w-full h-full object-cover"
                  />
                </button>
              ))}
            </div>
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
              <h1 className="font-display text-4xl text-fg-base leading-tight">{product.name}</h1>

              <p className="font-mono text-accent text-3xl font-medium">
                {formatCurrency(product.price)}
              </p>

              <div className="border-t border-surface-border pt-4">
                {product.description && (
                  <p className="text-sm text-fg-muted leading-relaxed mb-4">
                    {product.description}
                  </p>
                )}

                <div className="flex items-center gap-3 mb-5">
                  <StockBadge stock={product.stockAvailable} />
                  {product.categoryName && (
                    <Link to={`/categories/${product.categoryId}`}>
                      <Badge variant="blue">{product.categoryName}</Badge>
                    </Link>
                  )}
                  {product.stockReserved > 0 && (
                    <span className="text-xs text-fg-subtle font-mono">
                      {product.stockReserved} sold
                    </span>
                  )}
                </div>

                {/* Qty selector */}
                <div className="flex items-center gap-3 mb-5">
                  <span className="text-sm text-fg-muted">Quantity</span>
                  <div className="flex items-center border border-surface-border rounded-md overflow-hidden">
                    <button
                      type="button"
                      onClick={() => {
                        const next = Math.max(1, qty - 1)
                        setQty(next)
                        setInputVal(String(next))
                      }}
                      disabled={qty <= 1 || product.stockAvailable === 0}
                      className="w-9 h-9 flex items-center justify-center text-fg-muted hover:text-fg-base hover:bg-surface-raised disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                    >
                      −
                    </button>
                    <input
                      type="text"
                      inputMode="numeric"
                      value={inputVal}
                      onChange={(e) => setInputVal(e.target.value.replace(/\D/g, ''))}
                      onBlur={() => applyQty(inputVal, product.stockAvailable)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') applyQty(inputVal, product.stockAvailable)
                      }}
                      disabled={product.stockAvailable === 0}
                      className="w-14 text-center text-sm font-mono text-fg-base bg-transparent focus:outline-none disabled:opacity-40"
                    />
                    <button
                      type="button"
                      onClick={() => {
                        const next = Math.min(product.stockAvailable, qty + 1)
                        setQty(next)
                        setInputVal(String(next))
                      }}
                      disabled={qty >= product.stockAvailable || product.stockAvailable === 0}
                      className="w-9 h-9 flex items-center justify-center text-fg-muted hover:text-fg-base hover:bg-surface-raised disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                    >
                      +
                    </button>
                  </div>
                </div>

                <Button
                  className="w-full sm:w-auto px-8"
                  disabled={product.stockAvailable === 0 || addItem.isPending}
                  onClick={() =>
                    addItem.mutate(
                      { product_id: product.id, quantity: qty },
                      {
                        onSuccess: () => showToast('Added to cart', 'success'),
                        onError: (err: unknown) => {
                          const code = (err as { response?: { data?: { code?: string } } })
                            ?.response?.data?.code
                          if (code === 'INSUFFICIENT_STOCK') {
                            showToast(
                              `Only ${product.stockAvailable} unit(s) available`,
                              'error',
                            )
                          } else {
                            showToast('Failed to add to cart', 'error')
                          }
                        },
                      },
                    )
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

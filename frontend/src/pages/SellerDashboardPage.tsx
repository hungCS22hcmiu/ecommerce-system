import { useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useMyProducts, useDeleteProduct } from '@/features/seller/useSellerProducts'
import { ProductStatusBadge } from '@/features/seller/ProductStatusBadge'
import { DeleteConfirmDialog } from '@/features/seller/DeleteConfirmDialog'
import { Pagination } from '@/components/shared/Pagination'
import { EmptyState } from '@/components/shared/EmptyState'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { formatCurrency, formatDate } from '@/lib/utils'

const STATUS_OPTIONS = ['', 'ACTIVE', 'INACTIVE', 'DELETED'] as const

export function SellerDashboardPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const [deletingId, setDeletingId] = useState<number | null>(null)

  const urlPage = Number(searchParams.get('page') ?? '1')
  const status = searchParams.get('status') ?? ''
  const apiPage = urlPage - 1

  const { data, isLoading, isError } = useMyProducts({
    status: status || undefined,
    page: apiPage,
    size: 20,
  })

  const deleteProduct = useDeleteProduct()

  const products = data?.data ?? []
  const meta = data?.meta

  function handleStatusChange(value: string) {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      if (value) {
        next.set('status', value)
      } else {
        next.delete('status')
      }
      next.delete('page')
      return next
    })
  }

  return (
    <div className="max-w-6xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      <div className="flex items-center justify-between mb-6">
        <h1 className="font-display text-3xl text-fg-base">My Products</h1>
        <Link to="/seller/products/new">
          <Button>+ New Product</Button>
        </Link>
      </div>

      <div className="flex items-center gap-3 mb-6">
        <label className="text-sm text-fg-muted">Filter by status:</label>
        <select
          value={status}
          onChange={(e) => handleStatusChange(e.target.value)}
          className="h-9 rounded-md border border-surface-border bg-surface-overlay px-3 text-sm text-fg-base focus:outline-none focus:ring-1 focus:ring-accent"
        >
          {STATUS_OPTIONS.map((s) => (
            <option key={s} value={s}>
              {s || 'All statuses'}
            </option>
          ))}
        </select>
      </div>

      {isError && (
        <p className="text-sm text-status-failed mb-4">Failed to load products. Please try again.</p>
      )}

      {!isLoading && !isError && products.length === 0 && (
        <EmptyState
          title="No products found"
          description={status ? `No ${status.toLowerCase()} products.` : 'Create your first product to start selling.'}
          action={
            !status ? (
              <Link to="/seller/products/new">
                <Button>Create Product</Button>
              </Link>
            ) : undefined
          }
        />
      )}

      {(isLoading || products.length > 0) && (
        <div className="bg-surface-raised border border-surface-border rounded-lg overflow-hidden">
          <div className="grid grid-cols-[1fr_auto_auto_auto_auto] gap-4 px-5 py-3 border-b border-surface-border text-xs text-fg-subtle uppercase tracking-wider font-semibold">
            <span>Product</span>
            <span className="text-right">Price</span>
            <span className="text-right">Stock</span>
            <span className="text-right">Status</span>
            <span className="text-right">Actions</span>
          </div>

          {isLoading
            ? Array.from({ length: 5 }).map((_, i) => (
                <div key={i} className="grid grid-cols-[1fr_auto_auto_auto_auto] gap-4 px-5 py-4 border-b border-surface-border last:border-0">
                  <div className="space-y-1.5">
                    <Skeleton className="h-4 w-40" />
                    <Skeleton className="h-3 w-24" />
                  </div>
                  <Skeleton className="h-4 w-16 self-center" />
                  <Skeleton className="h-4 w-10 self-center" />
                  <Skeleton className="h-6 w-20 self-center rounded-full" />
                  <Skeleton className="h-8 w-24 self-center" />
                </div>
              ))
            : products.map((product) => (
                <div key={product.id}>
                  <div className="grid grid-cols-[1fr_auto_auto_auto_auto] gap-4 px-5 py-4 border-b border-surface-border last:border-0 items-center">
                    <div>
                      <p className="text-sm font-medium text-fg-base">{product.name}</p>
                      <p className="text-xs text-fg-subtle mt-0.5">{formatDate(product.createdAt)}</p>
                    </div>
                    <span className="text-sm font-mono text-fg-base text-right">
                      {formatCurrency(product.price)}
                    </span>
                    <span className="text-sm text-fg-muted text-right">
                      {product.stockAvailable}
                    </span>
                    <div className="flex justify-end">
                      <ProductStatusBadge status={product.status} />
                    </div>
                    <div className="flex items-center gap-2 justify-end">
                      <Link to={`/seller/products/${product.id}/edit`}>
                        <Button variant="outline" size="sm">Edit</Button>
                      </Link>
                      <Button
                        variant="destructive"
                        size="sm"
                        onClick={() => setDeletingId(product.id)}
                        disabled={deleteProduct.isPending && deleteProduct.variables === product.id}
                      >
                        Delete
                      </Button>
                    </div>
                  </div>
                  {deletingId === product.id && (
                    <div className="px-5 pb-4">
                      <DeleteConfirmDialog
                        productName={product.name}
                        isPending={deleteProduct.isPending}
                        onConfirm={() => {
                          deleteProduct.mutate(product.id, {
                            onSettled: () => setDeletingId(null),
                          })
                        }}
                        onCancel={() => setDeletingId(null)}
                      />
                    </div>
                  )}
                </div>
              ))}
        </div>
      )}

      {meta && <Pagination meta={meta} />}

      {meta && (
        <p className="text-center text-xs text-fg-subtle mt-3">
          {meta.totalElements} product{meta.totalElements !== 1 ? 's' : ''} total
        </p>
      )}
    </div>
  )
}

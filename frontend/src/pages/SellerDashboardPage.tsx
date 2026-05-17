import { useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useMyProducts, useDeleteProduct, useSellerProduct } from '@/features/seller/useSellerProducts'
import { useAuthStore } from '@/store/authStore'
import { ProductStatusBadge } from '@/features/seller/ProductStatusBadge'
import { DeleteConfirmDialog } from '@/features/seller/DeleteConfirmDialog'
import { Pagination } from '@/components/shared/Pagination'
import { EmptyState } from '@/components/shared/EmptyState'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { formatCurrency, formatDate } from '@/lib/utils'
import type { Product } from '@/types/product'

const STATUS_OPTIONS = ['', 'ACTIVE', 'INACTIVE', 'DELETED'] as const

function ProductRow({
  product,
  deletingId,
  setDeletingId,
  deleteProduct,
}: {
  product: Product
  deletingId: number | null
  setDeletingId: (id: number | null) => void
  deleteProduct: ReturnType<typeof useDeleteProduct>
}) {
  return (
    <div>
      <div className="grid grid-cols-[1fr_7rem_7rem_6rem_7rem_10rem] gap-4 px-5 py-4 border-b border-surface-border last:border-0 items-center">
        <div>
          <p className="text-sm font-medium text-fg-base">{product.name}</p>
          <p className="text-xs text-fg-subtle mt-0.5">ID: {product.id} · {formatDate(product.createdAt)}</p>
        </div>
        <span className="text-sm font-mono text-fg-base text-right">
          {formatCurrency(product.price)}
        </span>
        <span className="text-sm font-mono text-fg-base text-right">
          {product.stockAvailable} avail
        </span>
        <span className="text-sm font-mono text-fg-base text-right">
          {product.stockReserved}
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
  )
}

export function SellerDashboardPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const [deletingId, setDeletingId] = useState<number | null>(null)
  const [idSearch, setIdSearch] = useState('')
  const [sort, setSort] = useState<'default' | 'mostSold'>('default')

  const userId = useAuthStore((s) => s.userId)
  const urlPage = Number(searchParams.get('page') ?? '1')
  const status = searchParams.get('status') ?? ''
  const apiPage = urlPage - 1

  const isIdSearch = /^\d+$/.test(idSearch.trim()) && idSearch.trim() !== ''

  const { data, isLoading, isError } = useMyProducts(
    {
      status: status || undefined,
      page: apiPage,
      size: 20,
      sort: sort === 'mostSold' ? 'stockReserved,DESC' : undefined,
    },
    { enabled: !isIdSearch }
  )

  const { data: singleData, isLoading: singleLoading } = useSellerProduct(
    isIdSearch ? Number(idSearch) : undefined
  )

  const deleteProduct = useDeleteProduct()

  const products = data?.data ?? []
  const meta = data?.meta

  const singleProduct =
    isIdSearch && singleData?.data?.sellerId === userId ? singleData.data : null
  const singleNotFound = isIdSearch && !singleLoading && singleProduct === null

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

  const tableHeader = (
    <div className="grid grid-cols-[1fr_7rem_7rem_6rem_7rem_10rem] gap-4 px-5 py-3 border-b border-surface-border text-xs text-fg-subtle uppercase tracking-wider font-semibold">
      <span>Product</span>
      <span className="text-right">Price</span>
      <span className="text-right">Stock</span>
      <span className="text-right">Sold</span>
      <span className="text-right">Status</span>
      <span className="text-right">Actions</span>
    </div>
  )

  return (
    <div className="max-w-6xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      <div className="flex items-center justify-between mb-6">
        <h1 className="font-display text-3xl text-fg-base">My Products</h1>
        <Link to="/seller/products/new">
          <Button>+ New Product</Button>
        </Link>
      </div>

      {/* Controls row */}
      <div className="flex flex-wrap items-center gap-3 mb-6">
        <input
          type="text"
          value={idSearch}
          onChange={(e) => setIdSearch(e.target.value.replace(/\D/g, ''))}
          placeholder="Search by product ID…"
          className="h-9 w-48 rounded-md border border-surface-border bg-surface-overlay px-3 text-sm text-fg-base placeholder:text-fg-subtle focus:outline-none focus:ring-1 focus:ring-accent font-mono"
        />

        {!isIdSearch && (
          <>
            <div className="flex items-center rounded-md border border-surface-border overflow-hidden">
              <button
                type="button"
                onClick={() => setSort('default')}
                className={`px-3 h-9 text-sm transition-colors ${sort === 'default' ? 'bg-accent text-white' : 'bg-surface-overlay text-fg-muted hover:text-fg-base'}`}
              >
                Newest
              </button>
              <button
                type="button"
                onClick={() => setSort('mostSold')}
                className={`px-3 h-9 text-sm transition-colors ${sort === 'mostSold' ? 'bg-accent text-white' : 'bg-surface-overlay text-fg-muted hover:text-fg-base'}`}
              >
                Most Sold
              </button>
            </div>

            <div className="flex items-center gap-2 ml-auto">
              <label className="text-sm text-fg-muted">Status:</label>
              <select
                value={status}
                onChange={(e) => handleStatusChange(e.target.value)}
                className="h-9 rounded-md border border-surface-border bg-surface-overlay px-3 text-sm text-fg-base focus:outline-none focus:ring-1 focus:ring-accent"
              >
                {STATUS_OPTIONS.map((s) => (
                  <option key={s} value={s}>
                    {s || 'All'}
                  </option>
                ))}
              </select>
            </div>
          </>
        )}
      </div>

      {/* ID search result */}
      {isIdSearch && (
        <div className="bg-surface-raised border border-surface-border rounded-lg overflow-hidden">
          {tableHeader}
          {singleLoading && (
            <div className="grid grid-cols-[1fr_7rem_7rem_6rem_7rem_10rem] gap-4 px-5 py-4">
              <div className="space-y-1.5">
                <Skeleton className="h-4 w-40" />
                <Skeleton className="h-3 w-24" />
              </div>
              <Skeleton className="h-4 w-16 self-center" />
              <Skeleton className="h-4 w-10 self-center" />
              <Skeleton className="h-4 w-8 self-center" />
              <Skeleton className="h-6 w-20 self-center rounded-full" />
              <Skeleton className="h-8 w-24 self-center" />
            </div>
          )}
          {singleNotFound && (
            <p className="px-5 py-6 text-sm text-fg-subtle">
              No product found with ID <span className="font-mono">{idSearch}</span> in your store.
            </p>
          )}
          {singleProduct && (
            <ProductRow
              product={singleProduct}
              deletingId={deletingId}
              setDeletingId={setDeletingId}
              deleteProduct={deleteProduct}
            />
          )}
        </div>
      )}

      {/* Normal paginated list */}
      {!isIdSearch && (
        <>
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
              {tableHeader}
              {isLoading
                ? Array.from({ length: 5 }).map((_, i) => (
                    <div key={i} className="grid grid-cols-[1fr_7rem_7rem_6rem_7rem_10rem] gap-4 px-5 py-4 border-b border-surface-border last:border-0">
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
                    <ProductRow
                      key={product.id}
                      product={product}
                      deletingId={deletingId}
                      setDeletingId={setDeletingId}
                      deleteProduct={deleteProduct}
                    />
                  ))}
            </div>
          )}

          {meta && <Pagination meta={meta} />}

          {meta && (
            <p className="text-center text-xs text-fg-subtle mt-3">
              {meta.totalElements} product{meta.totalElements !== 1 ? 's' : ''} total
            </p>
          )}
        </>
      )}
    </div>
  )
}

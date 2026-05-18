import { useEffect } from 'react'
import { useParams, useSearchParams, Link } from 'react-router-dom'
import { useSellerProfile } from '@/features/sellers/useSellerProfile'
import { useProductList, useProductSearch } from '@/features/products/useProducts'
import { useProductAISearch } from '@/features/products/useProductAISearch'
import { ProductGrid } from '@/features/products/ProductGrid'
import { ProductCard } from '@/features/products/ProductCard'
import { SearchBar } from '@/features/products/SearchBar'
import { Pagination } from '@/components/shared/Pagination'
import { EmptyState } from '@/components/shared/EmptyState'
import { AISearchBadge } from '@/components/shared/AISearchBadge'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { showToast } from '@/lib/toast'

const SORT_OPTIONS = [
  { label: 'Most Sold', value: 'stockReserved,DESC' },
  { label: 'Highest Rated', value: 'avgRating,DESC' },
  { label: 'Newest', value: 'createdAt,DESC' },
  { label: 'Price: Low', value: 'price,ASC' },
  { label: 'Price: High', value: 'price,DESC' },
] as const

export function SellerShopPage() {
  const { id } = useParams<{ id: string }>()
  const [searchParams, setSearchParams] = useSearchParams()

  const q = searchParams.get('q') ?? ''
  const mode = (searchParams.get('mode') ?? 'keyword') as 'keyword' | 'ai'
  const sort = searchParams.get('sort') ?? 'createdAt,DESC'
  const urlPage = Number(searchParams.get('page') ?? '1')
  const apiPage = Math.max(0, urlPage - 1)

  const isSearching = q.trim().length >= 2
  const isAIMode = isSearching && mode === 'ai'

  const { data: sellerData, isLoading: sellerLoading, isError: sellerError } = useSellerProfile(id)
  const seller = sellerData?.data

  const listQuery = useProductList({ sellerId: id, status: 'ACTIVE', sort, page: apiPage, size: 20 })
  const searchQuery = useProductSearch(q, apiPage, undefined, id)
  const aiQuery = useProductAISearch(q, 20, undefined, id)

  const keywordQuery = isSearching ? searchQuery : listQuery
  const { isLoading: productsLoading, isError: productsError } = isAIMode ? aiQuery : keywordQuery
  const aiResults = aiQuery.data?.data.results ?? []
  const products = isAIMode ? aiResults : (keywordQuery.data?.data ?? [])
  const meta = isAIMode ? undefined : keywordQuery.data?.meta

  useEffect(() => {
    if (aiQuery.data?.data.mode === 'fallback-keyword') {
      showToast('Smart search unavailable — showing keyword results', 'info')
    }
  }, [aiQuery.data])

  const memberSince = seller?.createdAt ? new Date(seller.createdAt).getFullYear() : null

  function handleSearch(value: string) {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      if (value) next.set('q', value)
      else next.delete('q')
      next.delete('mode')
      next.set('page', '1')
      return next
    })
  }

  function handleModeChange(m: 'keyword' | 'ai') {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      if (m === 'ai') next.set('mode', 'ai')
      else next.delete('mode')
      next.set('page', '1')
      return next
    })
  }

  function handleSort(value: string) {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      next.set('sort', value)
      next.set('page', '1')
      return next
    })
  }

  if (sellerError) {
    return (
      <div className="max-w-7xl mx-auto px-4 py-16 text-center">
        <p className="text-status-failed mb-4">Seller not found.</p>
        <Link to="/products" className="text-accent hover:text-accent-dim text-sm">
          ← Browse products
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
        ← Browse products
      </Link>

      {/* Seller header */}
      <div className="bg-surface-raised border border-surface-border rounded-xl p-6 mb-8 flex items-center gap-5">
        {sellerLoading ? (
          <>
            <Skeleton className="w-16 h-16 rounded-full flex-shrink-0" />
            <div className="flex flex-col gap-2">
              <Skeleton className="h-7 w-48" />
              <Skeleton className="h-4 w-32" />
            </div>
          </>
        ) : seller ? (
          <>
            {seller.avatarUrl ? (
              <img
                src={seller.avatarUrl}
                alt={`${seller.firstName} ${seller.lastName}`}
                className="w-16 h-16 rounded-full object-cover flex-shrink-0 border border-surface-border"
              />
            ) : (
              <div className="w-16 h-16 rounded-full flex-shrink-0 bg-accent/10 flex items-center justify-center border border-surface-border">
                <span className="text-accent font-display text-xl font-semibold">
                  {seller.firstName[0]}{seller.lastName[0]}
                </span>
              </div>
            )}
            <div>
              <h1 className="font-display text-2xl text-fg-base">
                {seller.firstName} {seller.lastName}
              </h1>
              {memberSince && (
                <p className="text-sm text-fg-subtle mt-1">Member since {memberSince}</p>
              )}
            </div>
          </>
        ) : null}
      </div>

      {/* Search bar */}
      <div className="mb-6">
        <SearchBar
          onSearch={handleSearch}
          defaultValue={q}
          mode={mode}
          onModeChange={handleModeChange}
        />
      </div>

      {/* Sort controls — only shown when not searching */}
      {!isSearching && (
        <div className="flex items-center gap-2 flex-wrap mb-6">
          <span className="text-sm text-fg-muted mr-1">Sort by:</span>
          {SORT_OPTIONS.map((opt) => (
            <button
              key={opt.value}
              type="button"
              onClick={() => handleSort(opt.value)}
              className={`px-3 py-1.5 text-xs rounded-md border transition-colors ${
                sort === opt.value
                  ? 'bg-accent text-white border-accent'
                  : 'border-surface-border text-fg-muted hover:text-fg-base hover:border-fg-subtle'
              }`}
            >
              {opt.label}
            </button>
          ))}
        </div>
      )}

      {productsError && (
        <p className="text-sm text-status-failed mb-4">Failed to load products. Please try again.</p>
      )}

      {!productsLoading && !productsError && products.length === 0 && (
        <EmptyState
          title={isSearching ? `No results for "${q}" in this shop` : 'No products yet'}
          description={isSearching ? 'Try a different search term.' : 'This seller has no active products.'}
          action={
            isSearching ? (
              <Button variant="outline" onClick={() => handleSearch('')}>
                Clear search
              </Button>
            ) : undefined
          }
        />
      )}

      {isAIMode ? (
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          {productsLoading
            ? Array.from({ length: 8 }).map((_, i) => (
                <div key={i} className="flex flex-col gap-1">
                  <div className="bg-surface-raised border border-surface-border rounded-lg overflow-hidden animate-pulse">
                    <div className="w-full aspect-square bg-surface-overlay" />
                    <div className="p-4 flex flex-col gap-3">
                      <div className="h-5 w-3/4 bg-surface-overlay rounded" />
                      <div className="h-5 w-1/3 bg-surface-overlay rounded" />
                      <div className="h-9 w-full bg-surface-overlay rounded mt-1" />
                    </div>
                  </div>
                </div>
              ))
            : aiResults.map((p) => (
                <div key={p.id} className="flex flex-col gap-1">
                  <ProductCard product={p} />
                  <AISearchBadge />
                </div>
              ))}
        </div>
      ) : (
        <ProductGrid products={products} isLoading={productsLoading} />
      )}

      {!isAIMode && meta && meta.totalPages > 1 && <Pagination meta={meta} />}

      {!isAIMode && meta && (
        <p className="text-center text-xs text-fg-subtle mt-3">
          {meta.totalElements} product{meta.totalElements !== 1 ? 's' : ''}
        </p>
      )}
    </div>
  )
}

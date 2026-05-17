import { useEffect } from 'react'
import { useParams, useSearchParams, Link } from 'react-router-dom'
import { useProductList, useProductSearch } from '@/features/products/useProducts'
import { useProductAISearch } from '@/features/products/useProductAISearch'
import { useCategories } from '@/features/seller/useCategories'
import { ProductGrid } from '@/features/products/ProductGrid'
import { ProductCard } from '@/features/products/ProductCard'
import { SearchBar } from '@/features/products/SearchBar'
import { Pagination } from '@/components/shared/Pagination'
import { EmptyState } from '@/components/shared/EmptyState'
import { AISearchBadge } from '@/components/shared/AISearchBadge'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { showToast } from '@/lib/toast'

export function CategoryProductsPage() {
  const { id } = useParams<{ id: string }>()
  const categoryId = Number(id)
  const [searchParams, setSearchParams] = useSearchParams()

  const q = searchParams.get('q') ?? ''
  const urlPage = Number(searchParams.get('page') ?? '1')
  const apiPage = urlPage - 1
  const mode = (searchParams.get('mode') ?? 'keyword') as 'keyword' | 'ai'

  const isSearching = q.trim().length >= 2
  const isAIMode = isSearching && mode === 'ai'

  const categoriesQuery = useCategories()
  const category = categoriesQuery.data?.data.find((c) => c.id === categoryId)
  const categoryName = category?.name ?? `Category #${categoryId}`

  const listQuery = useProductList({ page: apiPage, limit: 20, categoryId })
  const searchQuery = useProductSearch(q, apiPage, categoryId)
  const aiQuery = useProductAISearch(q, 20, categoryId)

  const keywordQuery = isSearching ? searchQuery : listQuery
  const { isLoading, isError } = isAIMode ? aiQuery : keywordQuery
  const aiResults = aiQuery.data?.data.results ?? []
  const products = isAIMode ? aiResults : (keywordQuery.data?.data ?? [])
  const meta = isAIMode ? undefined : keywordQuery.data?.meta

  useEffect(() => {
    if (aiQuery.data?.data.mode === 'fallback-keyword') {
      showToast('Smart search unavailable — showing keyword results', 'info')
    }
  }, [aiQuery.data])

  const handleSearch = (value: string) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      if (value) next.set('q', value)
      else next.delete('q')
      next.set('page', '1')
      return next
    })
  }

  const handleModeChange = (m: 'keyword' | 'ai') => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      if (m === 'ai') next.set('mode', 'ai')
      else next.delete('mode')
      next.set('page', '1')
      return next
    })
  }

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      <Link
        to="/categories"
        className="text-sm text-fg-muted hover:text-fg-base transition-colors"
      >
        ← All Categories
      </Link>

      <div className="mt-6 mb-6">
        {categoriesQuery.isLoading ? (
          <Skeleton className="h-9 w-48" />
        ) : (
          <h1 className="font-display text-3xl text-fg-base">{categoryName}</h1>
        )}
      </div>

      <div className="mb-6">
        <SearchBar
          onSearch={handleSearch}
          defaultValue={q}
          mode={mode}
          onModeChange={handleModeChange}
        />
      </div>

      {isError && (
        <p className="text-sm text-status-failed mb-4">
          Failed to load products. Please try again.
        </p>
      )}

      {!isLoading && !isError && products.length === 0 && (
        <EmptyState
          title={isSearching ? `No results for "${q}" in this category` : 'No products in this category'}
          description={isSearching ? 'Try a different search term.' : 'This category has no products yet.'}
          action={
            isSearching ? (
              <Button variant="outline" onClick={() => handleSearch('')}>
                Clear search
              </Button>
            ) : (
              <Link to="/categories" className="text-sm text-accent hover:underline">
                Browse other categories
              </Link>
            )
          }
        />
      )}

      {isAIMode ? (
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          {isLoading
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
        <ProductGrid products={products} isLoading={isLoading} />
      )}

      {!isAIMode && meta && <Pagination meta={meta} />}

      {!isAIMode && meta && (
        <p className="text-center text-xs text-fg-subtle mt-3">
          {meta.totalElements} product{meta.totalElements !== 1 ? 's' : ''}
        </p>
      )}
    </div>
  )
}

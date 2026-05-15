import { useEffect } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useProductList, useProductSearch } from '@/features/products/useProducts'
import { useProductAISearch } from '@/features/products/useProductAISearch'
import { ProductGrid } from '@/features/products/ProductGrid'
import { ProductCard } from '@/features/products/ProductCard'
import { SearchBar } from '@/features/products/SearchBar'
import { Pagination } from '@/components/shared/Pagination'
import { EmptyState } from '@/components/shared/EmptyState'
import { AISearchBadge } from '@/components/shared/AISearchBadge'
import { Button } from '@/components/ui/button'
import { showToast } from '@/lib/toast'

export function ProductListPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const q = searchParams.get('q') ?? ''
  const urlPage = Number(searchParams.get('page') ?? '1')
  const apiPage = urlPage - 1 // convert 1-indexed URL → 0-indexed API
  const mode = (searchParams.get('mode') ?? 'keyword') as 'keyword' | 'ai'

  const listQuery = useProductList({ page: apiPage, limit: 20 })
  const searchQuery = useProductSearch(q, apiPage)
  const aiQuery = useProductAISearch(q, 20)

  const isSearching = q.trim().length >= 2
  const isAIMode = isSearching && mode === 'ai'

  const keywordQuery = isSearching ? searchQuery : listQuery
  const { isLoading, isError } = isAIMode ? aiQuery : keywordQuery

  const aiResults = aiQuery.data?.data.results ?? []
  const products = isAIMode ? aiResults : (keywordQuery.data?.data ?? [])
  const meta = isAIMode ? undefined : keywordQuery.data?.meta

  // Toast when AI search falls back to keyword results
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
          title={isSearching ? `No results for "${q}"` : 'No products found'}
          description={isSearching ? 'Try a different search term.' : undefined}
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

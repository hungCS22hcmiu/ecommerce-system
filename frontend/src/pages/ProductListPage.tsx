import { useSearchParams } from 'react-router-dom'
import { useProductList, useProductSearch } from '@/features/products/useProducts'
import { ProductGrid } from '@/features/products/ProductGrid'
import { SearchBar } from '@/features/products/SearchBar'
import { Pagination } from '@/components/shared/Pagination'
import { EmptyState } from '@/components/shared/EmptyState'
import { Button } from '@/components/ui/button'

export function ProductListPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const q = searchParams.get('q') ?? ''
  const urlPage = Number(searchParams.get('page') ?? '1')
  const apiPage = urlPage - 1 // convert 1-indexed URL → 0-indexed API

  const listQuery = useProductList({ page: apiPage, limit: 20 })
  const searchQuery = useProductSearch(q, apiPage)

  const isSearching = q.trim().length >= 2
  const { data, isLoading, isError } = isSearching ? searchQuery : listQuery

  const products = data?.data ?? []
  const meta = data?.meta

  const handleSearch = (value: string) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      if (value) next.set('q', value)
      else next.delete('q')
      next.set('page', '1')
      return next
    })
  }

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      <div className="mb-6">
        <SearchBar onSearch={handleSearch} defaultValue={q} />
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

      <ProductGrid products={products} isLoading={isLoading} />

      {meta && <Pagination meta={meta} />}

      {meta && (
        <p className="text-center text-xs text-fg-subtle mt-3">
          {meta.totalElements} product{meta.totalElements !== 1 ? 's' : ''}
        </p>
      )}
    </div>
  )
}

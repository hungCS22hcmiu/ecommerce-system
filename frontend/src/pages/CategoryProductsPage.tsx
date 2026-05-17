import { useParams, useSearchParams, Link } from 'react-router-dom'
import { useProductList } from '@/features/products/useProducts'
import { useCategories } from '@/features/seller/useCategories'
import { ProductGrid } from '@/features/products/ProductGrid'
import { Pagination } from '@/components/shared/Pagination'
import { EmptyState } from '@/components/shared/EmptyState'
import { Skeleton } from '@/components/ui/skeleton'

export function CategoryProductsPage() {
  const { id } = useParams<{ id: string }>()
  const categoryId = Number(id)
  const [searchParams] = useSearchParams()
  const urlPage = Number(searchParams.get('page') ?? '1')
  const apiPage = urlPage - 1

  const categoriesQuery = useCategories()
  const category = categoriesQuery.data?.data.find((c) => c.id === categoryId)
  const categoryName = category?.name ?? `Category #${categoryId}`

  const productsQuery = useProductList({ page: apiPage, limit: 20, categoryId })
  const products = productsQuery.data?.data ?? []
  const meta = productsQuery.data?.meta

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

      {productsQuery.isError && (
        <p className="text-sm text-status-failed mb-4">Failed to load products. Please try again.</p>
      )}

      {!productsQuery.isLoading && !productsQuery.isError && products.length === 0 && (
        <EmptyState
          title="No products in this category"
          description="This category has no products yet."
          action={
            <Link to="/categories" className="text-sm text-accent hover:underline">
              Browse other categories
            </Link>
          }
        />
      )}

      <ProductGrid products={products} isLoading={productsQuery.isLoading} />

      {meta && <Pagination meta={meta} />}

      {meta && (
        <p className="text-center text-xs text-fg-subtle mt-3">
          {meta.totalElements} product{meta.totalElements !== 1 ? 's' : ''}
        </p>
      )}
    </div>
  )
}

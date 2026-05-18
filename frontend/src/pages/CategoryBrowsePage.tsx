import { Link } from 'react-router-dom'
import { useCategories } from '@/features/seller/useCategories'
import { EmptyState } from '@/components/shared/EmptyState'
import { Skeleton } from '@/components/ui/skeleton'
import type { Category } from '@/types/category'

function CategoryCard({ category }: { category: Category }) {
  return (
    <Link
      to={`/categories/${category.id}`}
      className="group bg-surface-raised border border-surface-border rounded-lg p-6 flex flex-col gap-3 hover:border-accent transition-colors cursor-pointer"
    >
      <div className="w-10 h-10 rounded-md bg-accent/10 text-accent flex items-center justify-center font-display text-xl">
        {category.name[0].toUpperCase()}
      </div>
      <span className="font-display text-fg-base text-base group-hover:text-accent transition-colors">
        {category.name}
      </span>
    </Link>
  )
}

function CategoryCardSkeleton() {
  return (
    <div className="bg-surface-raised border border-surface-border rounded-lg p-6 flex flex-col gap-3">
      <Skeleton className="w-10 h-10 rounded-md" />
      <Skeleton className="h-5 w-3/4" />
    </div>
  )
}

export function CategoryBrowsePage() {
  const { data, isLoading, isError } = useCategories()
  const categories = data?.data ?? []

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      <div className="mb-6">
        <h1 className="font-display text-3xl text-fg-base">Browse Categories</h1>
        <p className="text-sm text-fg-muted mt-1">Explore products by category</p>
      </div>

      {isError && (
        <p className="text-sm text-status-failed mb-4">Failed to load categories. Please try again.</p>
      )}

      {isLoading ? (
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
          {Array.from({ length: 8 }).map((_, i) => (
            <CategoryCardSkeleton key={i} />
          ))}
        </div>
      ) : !isError && categories.length === 0 ? (
        <EmptyState title="No categories found" description="Check back later." />
      ) : (
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
          {categories.map((c) => (
            <CategoryCard key={c.id} category={c} />
          ))}
        </div>
      )}
    </div>
  )
}

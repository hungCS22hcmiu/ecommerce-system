import { Link } from 'react-router-dom'
import { useProductList } from '@/features/products/useProducts'
import { ProductGrid } from '@/features/products/ProductGrid'
import { Button } from '@/components/ui/button'

export function HomePage() {
  const { data, isLoading } = useProductList({ page: 0, limit: 8 })

  return (
    <div>
      {/* Hero */}
      <div className="relative bg-surface-raised border-b border-surface-border overflow-hidden">
        <div
          className="absolute inset-0 opacity-20 pointer-events-none"
          style={{
            background:
              'radial-gradient(ellipse at 90% 110%, #f59e0b 0%, transparent 60%)',
          }}
        />
        <div className="relative max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-24">
          <h1
            className="font-display text-6xl sm:text-7xl text-white leading-tight mb-6"
            style={{ animation: 'fadeUp 0.6s ease both' }}
          >
            Everything you need,
            <br />
            shipped to your door.
          </h1>
          <p
            className="text-zinc-400 text-lg mb-8 max-w-lg"
            style={{ animation: 'fadeUp 0.6s ease 0.15s both' }}
          >
            Browse our full catalog of products, from electronics to essentials.
          </p>
          <div style={{ animation: 'fadeUp 0.6s ease 0.3s both' }}>
            <Link to="/products">
              <Button size="lg">Browse Products →</Button>
            </Link>
          </div>
        </div>
      </div>

      {/* Featured products */}
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
        <div className="flex items-center justify-between mb-6">
          <h2 className="text-xl font-semibold text-white">Featured Products</h2>
          <Link to="/products" className="text-sm text-zinc-500 hover:text-accent transition-colors">
            View all →
          </Link>
        </div>
        <ProductGrid products={data?.data} isLoading={isLoading} count={8} />
      </div>
    </div>
  )
}

import { useSearchParams, useNavigate } from 'react-router-dom'
import { useOrders } from '@/features/orders/useOrders'
import { StatusBadge } from '@/features/orders/StatusBadge'
import { Pagination } from '@/components/shared/Pagination'
import { EmptyState } from '@/components/shared/EmptyState'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { formatCurrency, formatDate, truncateId } from '@/lib/utils'
import { Link } from 'react-router-dom'

export function OrderHistoryPage() {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const urlPage = Number(searchParams.get('page') ?? '1')
  const apiPage = urlPage - 1

  const { data, isLoading, isError } = useOrders(apiPage)
  const orders = data?.data ?? []
  const meta = data?.meta

  return (
    <div className="max-w-5xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      <h1 className="font-display text-3xl text-white mb-8">Order History</h1>

      {isError && (
        <p className="text-sm text-status-failed mb-4">Failed to load orders. Please try again.</p>
      )}

      {!isLoading && !isError && orders.length === 0 && (
        <EmptyState
          title="No orders yet"
          description="Place your first order to see it here."
          action={
            <Link to="/products">
              <Button>Browse Products</Button>
            </Link>
          }
        />
      )}

      {(isLoading || orders.length > 0) && (
        <div className="bg-surface-raised border border-surface-border rounded-lg overflow-hidden">
          {/* Table header */}
          <div className="grid grid-cols-[1fr_auto_auto_auto] gap-4 px-5 py-3 border-b border-surface-border text-xs text-zinc-500 uppercase tracking-wider font-semibold">
            <span>Order</span>
            <span className="text-right">Items</span>
            <span className="text-right">Total</span>
            <span className="text-right">Status</span>
          </div>

          {/* Rows */}
          {isLoading
            ? Array.from({ length: 5 }).map((_, i) => (
                <div key={i} className="grid grid-cols-[1fr_auto_auto_auto] gap-4 px-5 py-4 border-b border-surface-border last:border-0">
                  <div className="space-y-1.5">
                    <Skeleton className="h-4 w-24" />
                    <Skeleton className="h-3 w-32" />
                  </div>
                  <Skeleton className="h-4 w-8 self-center" />
                  <Skeleton className="h-4 w-16 self-center" />
                  <Skeleton className="h-6 w-20 self-center rounded-full" />
                </div>
              ))
            : orders.map((order) => (
                <button
                  key={order.id}
                  type="button"
                  onClick={() => navigate(`/orders/${order.id}`)}
                  className="w-full grid grid-cols-[1fr_auto_auto_auto] gap-4 px-5 py-4 border-b border-surface-border last:border-0 hover:bg-surface-overlay transition-colors text-left cursor-pointer"
                >
                  <div>
                    <p className="text-sm font-mono text-white">
                      #{truncateId(order.id).toUpperCase()}
                    </p>
                    <p className="text-xs text-zinc-500 mt-0.5">{formatDate(order.createdAt)}</p>
                  </div>
                  <span className="text-sm text-zinc-400 self-center text-right">
                    {order.itemCount} item{order.itemCount !== 1 ? 's' : ''}
                  </span>
                  <span className="text-sm font-mono text-white self-center text-right">
                    {formatCurrency(order.totalAmount)}
                  </span>
                  <div className="self-center flex justify-end">
                    <StatusBadge status={order.status} />
                  </div>
                </button>
              ))}
        </div>
      )}

      {meta && <Pagination meta={meta} />}

      {meta && (
        <p className="text-center text-xs text-zinc-600 mt-3">
          {meta.totalElements} order{meta.totalElements !== 1 ? 's' : ''} total
        </p>
      )}
    </div>
  )
}

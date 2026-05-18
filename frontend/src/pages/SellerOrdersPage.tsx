import { useSearchParams, useNavigate } from 'react-router-dom'
import { useSellerOrders, useShipOrder } from '@/features/orders/useSellerOrders'
import { StatusBadge } from '@/features/orders/StatusBadge'
import { Pagination } from '@/components/shared/Pagination'
import { EmptyState } from '@/components/shared/EmptyState'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { formatCurrency, formatDate, truncateId } from '@/lib/utils'
import { showToast } from '@/lib/toast'
import type { OrderStatus } from '@/types/order'

const STATUS_TABS: { label: string; value: OrderStatus | '' }[] = [
  { label: 'All', value: '' },
  { label: 'Pending', value: 'PENDING' },
  { label: 'Confirmed', value: 'CONFIRMED' },
  { label: 'Shipped', value: 'SHIPPED' },
  { label: 'Delivered', value: 'DELIVERED' },
  { label: 'Cancelled', value: 'CANCELLED' },
]

export function SellerOrdersPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const navigate = useNavigate()

  const urlPage = Number(searchParams.get('page') ?? '1')
  const statusParam = (searchParams.get('status') ?? '') as OrderStatus | ''
  const apiPage = Math.max(0, urlPage - 1)

  const { data, isLoading, isError } = useSellerOrders(
    statusParam || undefined,
    apiPage,
  )
  const shipOrder = useShipOrder()

  const orders = data?.data ?? []
  const meta = data?.meta

  function handleTabChange(value: OrderStatus | '') {
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

  function handleShip(e: React.MouseEvent, orderId: string) {
    e.stopPropagation()
    shipOrder.mutate(orderId, {
      onSuccess: () => showToast('Order marked as shipped', 'success'),
      onError: () => showToast('Failed to ship order', 'error'),
    })
  }

  const tableHeader = (
    <div className="grid grid-cols-[1fr_9rem_5rem_7rem_7rem_7rem_9rem] gap-4 px-5 py-3 border-b border-surface-border text-xs text-fg-subtle uppercase tracking-wider font-semibold">
      <span>Order</span>
      <span>Buyer</span>
      <span className="text-right">Items</span>
      <span className="text-right">Total</span>
      <span>Date</span>
      <span>Status</span>
      <span className="text-right">Action</span>
    </div>
  )

  return (
    <div className="max-w-6xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      <h1 className="font-display text-3xl text-fg-base mb-6">Seller Orders</h1>

      {/* Status tabs */}
      <div className="flex items-center rounded-md border border-surface-border overflow-hidden w-fit mb-6">
        {STATUS_TABS.map((tab) => (
          <button
            key={tab.value}
            type="button"
            onClick={() => handleTabChange(tab.value)}
            className={`px-4 h-9 text-sm transition-colors border-r border-surface-border last:border-r-0 ${
              statusParam === tab.value
                ? 'bg-accent text-white'
                : 'bg-surface-overlay text-fg-muted hover:text-fg-base'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {isError && (
        <p className="text-sm text-status-failed mb-4">Failed to load orders. Please try again.</p>
      )}

      {!isLoading && !isError && orders.length === 0 && (
        <EmptyState
          title="No orders found"
          description={statusParam ? `No ${statusParam.toLowerCase()} orders.` : 'Orders from customers will appear here.'}
        />
      )}

      {(isLoading || orders.length > 0) && (
        <div className="bg-surface-raised border border-surface-border rounded-lg overflow-hidden">
          {tableHeader}

          {isLoading
            ? Array.from({ length: 5 }).map((_, i) => (
                <div
                  key={i}
                  className="grid grid-cols-[1fr_9rem_5rem_7rem_7rem_7rem_9rem] gap-4 px-5 py-4 border-b border-surface-border last:border-0 items-center"
                >
                  <div className="space-y-1.5">
                    <Skeleton className="h-4 w-24" />
                    <Skeleton className="h-3 w-32" />
                  </div>
                  <Skeleton className="h-4 w-20" />
                  <Skeleton className="h-4 w-8" />
                  <Skeleton className="h-4 w-16" />
                  <Skeleton className="h-4 w-20" />
                  <Skeleton className="h-6 w-20 rounded-full" />
                  <Skeleton className="h-8 w-24 ml-auto" />
                </div>
              ))
            : orders.map((order) => (
                <button
                  key={order.id}
                  type="button"
                  onClick={() => navigate(`/seller/orders/${order.id}`)}
                  className="w-full grid grid-cols-[1fr_9rem_5rem_7rem_7rem_7rem_9rem] gap-4 px-5 py-4 border-b border-surface-border last:border-0 hover:bg-surface-overlay transition-colors text-left cursor-pointer items-center"
                >
                  <div>
                    <p className="text-sm font-mono text-fg-base">
                      #{truncateId(order.id).toUpperCase()}
                    </p>
                    <p className="text-xs text-fg-subtle mt-0.5">{formatDate(order.createdAt)}</p>
                  </div>
                  <span className="text-xs font-mono text-fg-subtle truncate">
                    {order.userId ? `${order.userId.slice(0, 8)}…` : '—'}
                  </span>
                  <span className="text-sm text-fg-muted text-right">
                    {order.itemCount}
                  </span>
                  <span className="text-sm font-mono text-fg-base text-right">
                    {formatCurrency(order.totalAmount)}
                  </span>
                  <span className="text-xs text-fg-subtle">{formatDate(order.createdAt)}</span>
                  <div>
                    <StatusBadge status={order.status} />
                  </div>
                  <div className="flex justify-end">
                    {order.status === 'CONFIRMED' && (
                      <Button
                        size="sm"
                        disabled={shipOrder.isPending && shipOrder.variables === order.id}
                        onClick={(e) => handleShip(e, order.id)}
                      >
                        {shipOrder.isPending && shipOrder.variables === order.id
                          ? 'Shipping…'
                          : 'Ship Order'}
                      </Button>
                    )}
                  </div>
                </button>
              ))}
        </div>
      )}

      {meta && <Pagination meta={meta} />}

      {meta && (
        <p className="text-center text-xs text-fg-subtle mt-3">
          {meta.totalElements} order{meta.totalElements !== 1 ? 's' : ''} total
        </p>
      )}
    </div>
  )
}

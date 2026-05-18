import { useParams, Link, useNavigate } from 'react-router-dom'
import { useSellerOrder, useShipOrder } from '@/features/orders/useSellerOrders'
import { StatusBadge } from '@/features/orders/StatusBadge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { formatCurrency, formatDate, truncateId } from '@/lib/utils'
import { showToast } from '@/lib/toast'

export function SellerOrderDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { data: orderData, isLoading, isError } = useSellerOrder(id ?? '')
  const shipOrder = useShipOrder()

  const order = orderData?.data
  const canShip = order?.status === 'CONFIRMED'

  if (isError) {
    return (
      <div className="max-w-5xl mx-auto px-4 py-12 text-center">
        <p className="text-status-failed mb-4">Order not found or access denied.</p>
        <Link to="/seller/orders" className="text-accent hover:text-accent-dim text-sm">
          ← Back to orders
        </Link>
      </div>
    )
  }

  return (
    <div className="max-w-5xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      <Link
        to="/seller/orders"
        className="text-sm text-fg-subtle hover:text-fg-base mb-6 inline-block transition-colors"
      >
        ← Back to orders
      </Link>

      {isLoading ? (
        <div className="space-y-4">
          <Skeleton className="h-8 w-48" />
          <Skeleton className="h-4 w-32" />
          <div className="grid grid-cols-1 lg:grid-cols-5 gap-8 mt-6">
            <Skeleton className="lg:col-span-3 h-64 rounded-lg" />
            <Skeleton className="lg:col-span-2 h-48 rounded-lg" />
          </div>
        </div>
      ) : order ? (
        <>
          <div className="flex items-start justify-between mb-6 flex-wrap gap-3">
            <div>
              <h1 className="font-display text-2xl text-fg-base">
                Order #{truncateId(order.id).toUpperCase()}
              </h1>
              <p className="text-sm text-fg-subtle mt-1">{formatDate(order.createdAt)}</p>
            </div>
            <StatusBadge status={order.status} />
          </div>

          <div className="grid grid-cols-1 lg:grid-cols-5 gap-8">
            {/* Left — items + address */}
            <div className="lg:col-span-3 space-y-5">
              <div className="bg-surface-raised border border-surface-border rounded-lg p-5">
                <h2 className="text-xs font-semibold text-fg-muted uppercase tracking-wider mb-4">
                  Items
                </h2>
                <div className="space-y-3">
                  {order.items.map((item) => (
                    <div key={item.id} className="flex justify-between text-sm">
                      <span className="text-fg-muted">
                        {item.productName} × {item.quantity}
                      </span>
                      <span className="font-mono text-fg-base">{formatCurrency(item.subtotal)}</span>
                    </div>
                  ))}
                </div>
                <div className="border-t border-surface-border pt-3 mt-3 flex justify-between text-sm">
                  <span className="text-fg-base font-semibold">Total</span>
                  <span className="font-mono text-accent font-semibold">
                    {formatCurrency(order.totalAmount)}
                  </span>
                </div>
              </div>

              <div className="bg-surface-raised border border-surface-border rounded-lg p-5">
                <h2 className="text-xs font-semibold text-fg-muted uppercase tracking-wider mb-3">
                  Shipping Address
                </h2>
                <address className="text-sm text-fg-muted not-italic leading-relaxed">
                  {order.shippingAddress.street}
                  <br />
                  {order.shippingAddress.city}
                  {order.shippingAddress.state && `, ${order.shippingAddress.state}`}
                  <br />
                  {order.shippingAddress.country} {order.shippingAddress.zipCode}
                </address>
              </div>
            </div>

            {/* Right — status + action + buyer info */}
            <div className="lg:col-span-2">
              <div className="bg-surface-raised border border-surface-border rounded-lg p-5 space-y-4">
                <h2 className="text-xs font-semibold text-fg-muted uppercase tracking-wider">
                  Order Info
                </h2>

                <div className="space-y-2 text-sm">
                  <div className="flex justify-between">
                    <span className="text-fg-muted">Status</span>
                    <StatusBadge status={order.status} />
                  </div>
                  <div className="flex justify-between">
                    <span className="text-fg-muted">Buyer</span>
                    <span className="font-mono text-xs text-fg-subtle">
                      {order.userId ? `${order.userId.slice(0, 8)}…` : '—'}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-fg-muted">Placed</span>
                    <span className="text-xs text-fg-subtle">{formatDate(order.createdAt)}</span>
                  </div>
                </div>

                {canShip && (
                  <div className="pt-2 border-t border-surface-border">
                    <Button
                      className="w-full"
                      disabled={shipOrder.isPending}
                      onClick={() =>
                        shipOrder.mutate(order.id, {
                          onSuccess: () => {
                            showToast('Order marked as shipped', 'success')
                            navigate('/seller/orders')
                          },
                          onError: () => showToast('Failed to ship order', 'error'),
                        })
                      }
                    >
                      {shipOrder.isPending ? 'Shipping…' : 'Mark as Shipped'}
                    </Button>
                  </div>
                )}
              </div>
            </div>
          </div>
        </>
      ) : null}
    </div>
  )
}

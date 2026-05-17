import { useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useOrder, useOrderHistory, useCancelOrder, useDeliverOrder } from '@/features/orders/useOrders'
import { StatusBadge } from '@/features/orders/StatusBadge'
import { OrderTimeline } from '@/features/orders/OrderTimeline'
import { ReviewDialog } from '@/components/shared/ReviewDialog'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { formatCurrency, formatDate, truncateId } from '@/lib/utils'
import { showToast } from '@/lib/toast'

export function OrderDetailPage() {
  const { id } = useParams<{ id: string }>()
  const { data: orderData, isLoading, isError } = useOrder(id ?? '')
  const { data: historyData } = useOrderHistory(id ?? '')
  const cancelOrder = useCancelOrder()
  const deliverOrder = useDeliverOrder()
  const [showReview, setShowReview] = useState(false)

  const order = orderData?.data
  const history = historyData?.data ?? []

  const canCancel = order?.status === 'PENDING' || order?.status === 'CONFIRMED'
  const canDeliver = order?.status === 'SHIPPED'
  const isDelivered = order?.status === 'DELIVERED'

  if (isError) {
    return (
      <div className="max-w-5xl mx-auto px-4 py-12 text-center">
        <p className="text-status-failed mb-4">Order not found or failed to load.</p>
        <Link to="/orders" className="text-accent hover:text-accent-dim text-sm">
          ← Back to orders
        </Link>
      </div>
    )
  }

  return (
    <div className="max-w-5xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      <Link to="/orders" className="text-sm text-fg-subtle hover:text-fg-base mb-6 inline-block transition-colors">
        ← Back to orders
      </Link>

      {isLoading ? (
        <div className="space-y-4">
          <Skeleton className="h-8 w-48" />
          <Skeleton className="h-4 w-32" />
          <div className="grid grid-cols-1 lg:grid-cols-5 gap-8 mt-6">
            <Skeleton className="lg:col-span-3 h-64 rounded-lg" />
            <Skeleton className="lg:col-span-2 h-64 rounded-lg" />
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
                  {order.shippingAddress.street}<br />
                  {order.shippingAddress.city}
                  {order.shippingAddress.state && `, ${order.shippingAddress.state}`}<br />
                  {order.shippingAddress.country} {order.shippingAddress.zipCode}
                </address>
              </div>
            </div>

            {/* Right — timeline + cancel */}
            <div className="lg:col-span-2">
              <div className="bg-surface-raised border border-surface-border rounded-lg p-5">
                <h2 className="text-xs font-semibold text-fg-muted uppercase tracking-wider mb-4">
                  Status Timeline
                </h2>
                <OrderTimeline history={history} createdAt={order.createdAt} />

                {canCancel && (
                  <div className="mt-5 pt-4 border-t border-surface-border">
                    <Button
                      variant="destructive"
                      className="w-full"
                      disabled={cancelOrder.isPending}
                      onClick={() => cancelOrder.mutate(order.id)}
                    >
                      {cancelOrder.isPending ? 'Cancelling…' : 'Cancel Order'}
                    </Button>
                  </div>
                )}

                {canDeliver && (
                  <div className="mt-5 pt-4 border-t border-surface-border">
                    <Button
                      className="w-full"
                      disabled={deliverOrder.isPending}
                      onClick={() =>
                        deliverOrder.mutate(order.id, {
                          onSuccess: () => {
                            showToast('Order marked as delivered', 'success')
                            setShowReview(true)
                          },
                          onError: () => showToast('Failed to update order', 'error'),
                        })
                      }
                    >
                      {deliverOrder.isPending ? 'Confirming…' : 'Mark as Delivered'}
                    </Button>
                  </div>
                )}

                {isDelivered && (
                  <div className="mt-3">
                    <Button
                      variant="outline"
                      className="w-full text-sm"
                      onClick={() => setShowReview(true)}
                    >
                      Write / Edit Reviews
                    </Button>
                  </div>
                )}
              </div>
            </div>
          </div>

          {showReview && order && (
            <ReviewDialog items={order.items} onClose={() => setShowReview(false)} />
          )}
        </>
      ) : null}
    </div>
  )
}

import { useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useOrder } from '@/features/orders/useOrders'
import { usePaymentStatus } from '@/features/payment/usePaymentStatus'
import { formatCurrency } from '@/lib/utils'

export function OrderConfirmationPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { data: orderData } = useOrder(id ?? '')
  const { data: paymentData } = usePaymentStatus(id ?? '')

  const order = orderData?.data
  const payment = paymentData?.data
  const status = payment?.status

  const isTerminal = status === 'COMPLETED' || status === 'FAILED'

  useEffect(() => {
    if (!isTerminal) return
    const t = setTimeout(() => navigate('/orders'), 3000)
    return () => clearTimeout(t)
  }, [isTerminal, navigate])

  return (
    <div className="max-w-xl mx-auto px-4 py-16 text-center">
      <p className="text-xs font-mono text-zinc-500 mb-2">Order Confirmation</p>
      {order && (
        <p className="text-sm font-mono text-zinc-400 mb-8 truncate">
          #{order.id.slice(0, 8).toUpperCase()}
        </p>
      )}

      {/* Payment status indicator */}
      <div className="relative flex items-center justify-center mb-8">
        {!isTerminal && (
          <>
            {/* Amber ping ring */}
            <span className="absolute inline-flex h-16 w-16 rounded-full bg-accent opacity-20 animate-ping" />
            <span className="relative inline-flex rounded-full h-12 w-12 bg-accent/20 border border-accent items-center justify-center">
              <svg
                className="animate-spin h-5 w-5 text-accent"
                xmlns="http://www.w3.org/2000/svg"
                fill="none"
                viewBox="0 0 24 24"
              >
                <circle
                  className="opacity-25"
                  cx="12"
                  cy="12"
                  r="10"
                  stroke="currentColor"
                  strokeWidth="4"
                />
                <path
                  className="opacity-75"
                  fill="currentColor"
                  d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
                />
              </svg>
            </span>
          </>
        )}

        {status === 'COMPLETED' && (
          <span className="inline-flex h-12 w-12 rounded-full bg-status-delivered/20 border border-status-delivered items-center justify-center">
            <svg className="h-6 w-6 text-status-delivered" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
            </svg>
          </span>
        )}

        {status === 'FAILED' && (
          <span className="inline-flex h-12 w-12 rounded-full bg-status-failed/20 border border-status-failed items-center justify-center">
            <svg className="h-6 w-6 text-status-failed" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </span>
        )}
      </div>

      <h1 className="font-display text-2xl text-white mb-2">
        {!isTerminal && 'Processing payment…'}
        {status === 'COMPLETED' && 'Payment confirmed!'}
        {status === 'FAILED' && 'Payment failed'}
      </h1>

      <p className="text-sm text-zinc-500 mb-8">
        {!isTerminal && 'Your order is being processed. This usually takes a few seconds.'}
        {status === 'COMPLETED' && 'Your order has been confirmed. Redirecting to orders…'}
        {status === 'FAILED' && 'Something went wrong with your payment. Redirecting…'}
      </p>

      {/* Order summary */}
      {order && (
        <div className="bg-surface-raised border border-surface-border rounded-lg p-5 text-left space-y-3">
          <h2 className="text-xs font-semibold text-zinc-400 uppercase tracking-wider">
            Order Summary
          </h2>
          <div className="space-y-2">
            {order.items.map((item) => (
              <div key={item.id} className="flex justify-between text-sm">
                <span className="text-zinc-300">
                  {item.productName} × {item.quantity}
                </span>
                <span className="font-mono text-white">{formatCurrency(item.subtotal)}</span>
              </div>
            ))}
          </div>
          <div className="border-t border-surface-border pt-3 flex justify-between text-sm">
            <span className="text-white font-semibold">Total</span>
            <span className="font-mono text-accent font-semibold">
              {formatCurrency(order.totalAmount)}
            </span>
          </div>
        </div>
      )}

      {isTerminal && (
        <p className="text-xs text-zinc-600 mt-6">Redirecting in 3 seconds…</p>
      )}
    </div>
  )
}

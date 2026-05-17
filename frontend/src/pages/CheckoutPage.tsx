import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useCart } from '@/features/cart/useCart'
import { useCreateOrder } from '@/features/orders/useOrders'
import { authApi } from '@/features/auth/authApi'
import { Button } from '@/components/ui/button'
import { formatCurrency } from '@/lib/utils'
import type { ShippingAddress } from '@/types/order'
import type { Address } from '@/types/auth'

const EMPTY_ADDR: ShippingAddress = {
  street: '',
  city: '',
  state: '',
  country: '',
  zipCode: '',
}

function addressToShipping(a: Address): ShippingAddress {
  return {
    street: a.address_line1,
    city: a.city,
    state: a.state ?? '',
    country: a.country,
    zipCode: a.postal_code ?? '',
  }
}

export function CheckoutPage() {
  const navigate = useNavigate()
  const { data: cartData } = useCart()
  const createOrder = useCreateOrder()
  const [address, setAddress] = useState<ShippingAddress>(EMPTY_ADDR)
  const [saved, setSaved] = useState<Address[]>([])
  const [selectedSaved, setSelectedSaved] = useState<string | null>(null)

  const cart = cartData?.data
  const items = cart?.items ?? []

  useEffect(() => {
    authApi.getProfile().then((res) => {
      const addrs = res.data?.addresses ?? []
      setSaved(addrs)
      const def = addrs.find((a) => a.is_default) ?? addrs[0]
      if (def) {
        setSelectedSaved(def.id)
        setAddress(addressToShipping(def))
      }
    }).catch(() => {})
  }, [])

  function handleSavedSelect(a: Address) {
    setSelectedSaved(a.id)
    setAddress(addressToShipping(a))
  }

  function handleField(field: keyof ShippingAddress, value: string) {
    setSelectedSaved(null)
    setAddress((prev) => ({ ...prev, [field]: value }))
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!items.length) return

    const result = await createOrder.mutateAsync({
      items: items.map((i) => ({ productId: i.product_id, quantity: i.quantity })),
      shippingAddress: address,
    })

    const orderId = result.data?.id
    if (orderId) navigate(`/orders/${orderId}/confirmation`)
  }

  const isValid =
    address.street && address.city && address.state && address.country && address.zipCode

  return (
    <div className="max-w-5xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      <h1 className="font-display text-3xl text-fg-base mb-8">Checkout</h1>

      <form onSubmit={handleSubmit}>
        <div className="grid grid-cols-1 lg:grid-cols-5 gap-8">
          {/* Address form */}
          <div className="lg:col-span-3 space-y-5">
            <div className="bg-surface-raised border border-surface-border rounded-lg p-5">
              <h2 className="font-semibold text-fg-base text-sm mb-4">Shipping Address</h2>

              {/* Saved addresses */}
              {saved.length > 0 && (
                <div className="space-y-2 mb-5">
                  {saved.map((a) => (
                    <label
                      key={a.id}
                      className={`flex items-start gap-3 p-3 rounded-md border cursor-pointer transition-colors ${
                        selectedSaved === a.id
                          ? 'border-accent bg-accent/5'
                          : 'border-surface-border hover:border-zinc-500'
                      }`}
                    >
                      <input
                        type="radio"
                        name="saved-address"
                        checked={selectedSaved === a.id}
                        onChange={() => handleSavedSelect(a)}
                        className="mt-0.5 accent-amber-500"
                      />
                      <div className="text-xs text-fg-muted leading-relaxed">
                        <span className="text-fg-base font-medium">
                          {a.label ?? 'Address'}
                          {a.is_default && (
                            <span className="ml-2 text-accent text-xs">(Default)</span>
                          )}
                        </span>
                        <br />
                        {a.address_line1}
                        {a.address_line2 && `, ${a.address_line2}`}, {a.city}
                        {a.state && `, ${a.state}`}, {a.country}
                      </div>
                    </label>
                  ))}
                </div>
              )}

              {/* Manual form */}
              <div className="space-y-3">
                {(
                  [
                    ['street', 'Street Address'],
                    ['city', 'City'],
                    ['state', 'State / Province'],
                    ['country', 'Country'],
                    ['zipCode', 'ZIP / Postal Code'],
                  ] as [keyof ShippingAddress, string][]
                ).map(([field, label]) => (
                  <div key={field}>
                    <label className="block text-xs text-fg-muted mb-1">{label}</label>
                    <input
                      type="text"
                      value={address[field]}
                      onChange={(e) => handleField(field, e.target.value)}
                      required
                      className="w-full bg-surface-base border border-surface-border rounded-md px-3 py-2 text-sm text-fg-base placeholder:text-fg-subtle focus:outline-none focus:border-accent transition-colors"
                    />
                  </div>
                ))}
              </div>
            </div>
          </div>

          {/* Order summary */}
          <div className="lg:col-span-2">
            <div className="bg-surface-raised border border-surface-border rounded-lg p-5 space-y-4 sticky top-6">
              <h2 className="font-semibold text-fg-base text-sm">Order Summary</h2>

              <div className="space-y-2 max-h-48 overflow-y-auto">
                {items.map((item) => (
                  <div key={item.product_id} className="flex justify-between text-xs">
                    <span className="text-fg-muted truncate pr-2">
                      {item.product_name} × {item.quantity}
                    </span>
                    <span className="font-mono text-fg-base flex-shrink-0">
                      {formatCurrency(item.subtotal)}
                    </span>
                  </div>
                ))}
              </div>

              <div className="border-t border-surface-border pt-3">
                <div className="flex justify-between text-sm mb-1">
                  <span className="text-fg-muted">Shipping</span>
                  <span className="font-mono text-status-delivered text-xs">Free</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-fg-base font-semibold text-sm">Total</span>
                  <span className="font-mono text-accent font-semibold">
                    {formatCurrency(cart?.total ?? 0)}
                  </span>
                </div>
              </div>

              {createOrder.isError && (
                <p className="text-xs text-status-failed">
                  {(() => {
                    const code = (createOrder.error as { response?: { data?: { error?: { code?: string; message?: string } } } })
                      ?.response?.data?.error
                    if (code?.code === 'INSUFFICIENT_STOCK') {
                      return `Out of stock: ${code.message ?? 'one or more items exceed available stock. Please update your cart.'}`
                    }
                    return 'Failed to place order. Please try again.'
                  })()}
                </p>
              )}

              <Button
                type="submit"
                className="w-full"
                disabled={!isValid || createOrder.isPending || items.length === 0}
              >
                {createOrder.isPending ? 'Placing Order…' : 'Place Order →'}
              </Button>
            </div>
          </div>
        </div>
      </form>
    </div>
  )
}

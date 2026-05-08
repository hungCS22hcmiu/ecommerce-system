export type OrderStatus = 'PENDING' | 'CONFIRMED' | 'SHIPPED' | 'DELIVERED' | 'CANCELLED'

export interface ShippingAddress {
  street: string
  city: string
  state: string
  country: string
  zipCode: string
}

export interface OrderItem {
  id: string
  productId: number
  productName: string
  quantity: number
  unitPrice: number
  subtotal: number
}

export interface Order {
  id: string
  userId: string
  cartId: string
  totalAmount: number
  status: OrderStatus
  shippingAddress: ShippingAddress
  items: OrderItem[]
  createdAt: string
  updatedAt: string
}

export interface OrderSummary {
  id: string
  totalAmount: number
  status: OrderStatus
  itemCount: number
  createdAt: string
}

export interface CreateOrderRequest {
  cartId?: string
  items: { productId: number; quantity: number }[]
  shippingAddress: ShippingAddress
}

export interface OrderStatusHistory {
  id: number
  oldStatus: string
  newStatus: string
  reason?: string
  changedBy: string
  changedAt: string
}

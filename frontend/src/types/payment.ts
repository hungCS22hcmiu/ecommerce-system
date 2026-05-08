export type PaymentStatus = 'PENDING' | 'COMPLETED' | 'FAILED' | 'REFUNDED'

export interface Payment {
  id: string
  orderId: string
  userId: string
  amount: number
  currency: string
  status: PaymentStatus
  method: string
  gatewayReference?: string
  createdAt: string
}

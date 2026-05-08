import { api } from '@/lib/axios'
import type { ApiResponse } from '@/types/api'
import type { Payment } from '@/types/payment'

export const paymentApi = {
  getByOrderId: (orderId: string) =>
    api.get<ApiResponse<Payment>>(`/payments/order/${orderId}`).then((r) => r.data),
}

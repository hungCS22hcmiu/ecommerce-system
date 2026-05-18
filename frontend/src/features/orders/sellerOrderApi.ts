import { api } from '@/lib/axios'
import type { ApiResponse } from '@/types/api'
import type { Order, OrderSummary, OrderStatus } from '@/types/order'

export const sellerOrderApi = {
  list: (params: { status?: OrderStatus; page?: number; size?: number }) =>
    api
      .get<ApiResponse<OrderSummary[]>>('/orders/seller', { params: { size: 20, ...params } })
      .then((r) => r.data),

  getById: (id: string) =>
    api.get<ApiResponse<Order>>(`/orders/seller/${id}`).then((r) => r.data),

  ship: (id: string) =>
    api.put<ApiResponse<Order>>(`/orders/${id}/ship`).then((r) => r.data),
}

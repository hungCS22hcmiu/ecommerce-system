import { api } from '@/lib/axios'
import type { ApiResponse } from '@/types/api'
import type { Order, OrderSummary, OrderStatusHistory, CreateOrderRequest } from '@/types/order'

export const orderApi = {
  create: (req: CreateOrderRequest) =>
    api.post<ApiResponse<Order>>('/orders', req).then((r) => r.data),

  getById: (id: string) =>
    api.get<ApiResponse<Order>>(`/orders/${id}`).then((r) => r.data),

  list: (page = 0, size = 20) =>
    api.get<ApiResponse<OrderSummary[]>>('/orders', { params: { page, size } }).then((r) => r.data),

  getHistory: (id: string) =>
    api.get<ApiResponse<OrderStatusHistory[]>>(`/orders/${id}/history`).then((r) => r.data),

  cancel: (id: string) =>
    api.put<ApiResponse<Order>>(`/orders/${id}/cancel`).then((r) => r.data),
}

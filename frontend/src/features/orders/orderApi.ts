import { api } from '@/lib/axios'
import type { ApiResponse } from '@/types/api'
import type { Order, CreateOrderRequest } from '@/types/order'

export const orderApi = {
  create: (req: CreateOrderRequest) =>
    api.post<ApiResponse<Order>>('/orders', req).then((r) => r.data),

  getById: (id: string) =>
    api.get<ApiResponse<Order>>(`/orders/${id}`).then((r) => r.data),
}

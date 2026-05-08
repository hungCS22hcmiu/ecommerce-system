import { api } from '@/lib/axios'
import type { ApiResponse } from '@/types/api'
import type { Cart, AddCartItemRequest } from '@/types/cart'

export const cartApi = {
  getCart: () => api.get<ApiResponse<Cart>>('/cart').then((r) => r.data),

  addItem: (req: AddCartItemRequest) =>
    api.post<ApiResponse<Cart>>('/cart/items', req).then((r) => r.data),

  updateItem: (productId: number, quantity: number) =>
    api.put<ApiResponse<Cart>>(`/cart/items/${productId}`, { quantity }).then((r) => r.data),

  removeItem: (productId: number) => api.delete(`/cart/items/${productId}`),

  clearCart: () => api.delete('/cart'),
}

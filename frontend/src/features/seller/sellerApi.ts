import { api } from '@/lib/axios'
import { useAuthStore } from '@/store/authStore'
import type { ApiResponse } from '@/types/api'
import type { Product, ProductDetail } from '@/types/product'
import type { CreateProductBody, UpdateProductBody } from '@/types/seller'

function sellerHeaders() {
  return { 'X-Seller-Id': useAuthStore.getState().userId as string }
}

export const sellerApi = {
  listMyProducts: (params: { status?: string; page?: number; size?: number } = {}) => {
    const userId = useAuthStore.getState().userId as string
    return api
      .get<ApiResponse<Product[]>>('/products', {
        params: { sellerId: userId, size: 20, ...params },
        headers: sellerHeaders(),
      })
      .then((r) => r.data)
  },

  getProductById: (id: number) =>
    api
      .get<ApiResponse<ProductDetail>>(`/products/${id}`, { headers: sellerHeaders() })
      .then((r) => r.data),

  createProduct: (body: CreateProductBody) =>
    api
      .post<ApiResponse<ProductDetail>>('/products', body, { headers: sellerHeaders() })
      .then((r) => r.data),

  updateProduct: (id: number, body: UpdateProductBody) =>
    api
      .put<ApiResponse<ProductDetail>>(`/products/${id}`, body, { headers: sellerHeaders() })
      .then((r) => r.data),

  deleteProduct: (id: number) =>
    api.delete(`/products/${id}`, { headers: sellerHeaders() }),
}

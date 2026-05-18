import { api } from '@/lib/axios'
import type { ApiResponse } from '@/types/api'
import type { Product, ProductDetail, StockLevel, ProductListParams, AISearchResponse } from '@/types/product'

export const productApi = {
  list: (params: ProductListParams = {}) =>
    api.get<ApiResponse<Product[]>>('/products', { params }).then((r) => r.data),

  search: (q: string, page = 0, limit = 20, categoryId?: number, sellerId?: string) =>
    api
      .get<ApiResponse<Product[]>>('/products/search', { params: { q, page, limit, categoryId, sellerId } })
      .then((r) => r.data),

  getById: (id: number) =>
    api.get<ApiResponse<ProductDetail>>(`/products/${id}`).then((r) => r.data),

  getStock: (id: number) =>
    api.get<ApiResponse<StockLevel>>(`/inventory/${id}`).then((r) => r.data),

  aiSearch: (q: string, limit = 20, categoryId?: number, sellerId?: string) =>
    api
      .get<ApiResponse<AISearchResponse>>('/products/ai-search', { params: { q, limit, categoryId, sellerId } })
      .then((r) => r.data),
}

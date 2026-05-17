import { api } from '@/lib/axios'
import type { ApiResponse } from '@/types/api'
import type { Review, CreateReviewRequest, UpdateReviewRequest } from '@/types/review'

export const reviewApi = {
  create: (productId: number, req: CreateReviewRequest) =>
    api.post<ApiResponse<Review>>(`/products/${productId}/reviews`, req).then(r => r.data),

  update: (productId: number, reviewId: number, req: UpdateReviewRequest) =>
    api.put<ApiResponse<Review>>(`/products/${productId}/reviews/${reviewId}`, req).then(r => r.data),

  getByOrderItem: (productId: number, orderItemId: string) =>
    api.get<ApiResponse<Review | null>>(`/products/${productId}/reviews/by-order-item/${orderItemId}`)
       .then(r => r.data),

  list: (productId: number, page = 0) =>
    api.get<ApiResponse<Review[]>>(`/products/${productId}/reviews`, { params: { page, size: 10 } })
       .then(r => r.data),
}

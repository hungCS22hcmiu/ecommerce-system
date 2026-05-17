export interface Review {
  id: number
  productId: number
  customerId: string
  orderItemId: string
  rating: number
  comment?: string
  createdAt: string
  updatedAt: string
}

export interface CreateReviewRequest {
  orderItemId: string
  rating: number
  comment?: string
}

export interface UpdateReviewRequest {
  rating: number
  comment?: string
}

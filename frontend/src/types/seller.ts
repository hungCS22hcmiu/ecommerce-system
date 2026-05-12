export interface CreateProductBody {
  name: string
  description?: string
  price: number
  categoryId?: number
  stockQuantity: number
  images?: Array<{ url: string; altText?: string; sortOrder: number }>
}

export interface UpdateProductBody {
  name?: string
  description?: string
  price?: number
  categoryId?: number
  status?: 'ACTIVE' | 'INACTIVE' | 'DELETED'
  stockQuantity?: number
  images?: Array<{ url: string; altText?: string; sortOrder: number }>
}

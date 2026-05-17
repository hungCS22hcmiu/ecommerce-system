export interface Product {
  id: number
  name: string
  price: number
  categoryId: number
  categoryName: string
  sellerId: string
  status: 'ACTIVE' | 'INACTIVE' | 'DELETED'
  stockAvailable: number
  stockReserved: number
  thumbnailUrl?: string
  createdAt: string
}

export interface ProductImage {
  id: number
  url: string
  altText?: string
  sortOrder: number
}

export interface ProductDetail extends Product {
  description?: string
  stockQuantity: number
  version: number
  images: ProductImage[]
  updatedAt: string
}

export interface StockLevel {
  productId: number
  stockQuantity: number
  stockReserved: number
  availableStock: number
}

export interface ProductListParams {
  page?: number
  limit?: number
  categoryId?: number
  status?: string
}

export interface AISearchResponse {
  query: string
  results: Product[]
  scores: number[] | null
  mode: 'ai' | 'fallback-keyword'
}

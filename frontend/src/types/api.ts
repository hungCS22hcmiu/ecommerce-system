export interface ApiResponse<T> {
  success: true
  data: T
  meta?: PaginationMeta
}

export interface ApiError {
  success: false
  error: {
    code: string
    message: string
    details?: Record<string, string>
  }
}

export interface PaginationMeta {
  page: number
  limit: number
  total: number
  totalPages: number
}

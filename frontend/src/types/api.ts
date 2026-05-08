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
  page: number          // zero-indexed
  size: number
  totalElements: number
  totalPages: number
}

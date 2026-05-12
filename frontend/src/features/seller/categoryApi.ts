import { api } from '@/lib/axios'
import type { ApiResponse } from '@/types/api'
import type { Category } from '@/types/category'

export const categoryApi = {
  list: (q?: string) =>
    api
      .get<ApiResponse<Category[]>>('/categories', { params: q ? { q } : undefined })
      .then((r) => r.data),

  create: (name: string) =>
    api
      .post<ApiResponse<Category>>('/categories', { name })
      .then((r) => r.data),
}

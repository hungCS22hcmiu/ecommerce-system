import { useQuery } from '@tanstack/react-query'
import { productApi } from './productApi'
import type { ProductListParams } from '@/types/product'

export function useProductList(params: ProductListParams = {}) {
  return useQuery({
    queryKey: ['products', params],
    queryFn: () => productApi.list(params),
    staleTime: 3 * 60_000,
  })
}

export function useProductSearch(q: string, page = 0, categoryId?: number, sellerId?: string) {
  return useQuery({
    queryKey: ['products', 'search', q, page, categoryId, sellerId],
    queryFn: () => productApi.search(q, page, 20, categoryId, sellerId),
    enabled: q.trim().length >= 2,
    staleTime: 3 * 60_000,
  })
}

export function useProduct(id: number) {
  return useQuery({
    queryKey: ['product', id],
    queryFn: () => productApi.getById(id),
    staleTime: 30 * 60_000,
    enabled: !!id,
  })
}

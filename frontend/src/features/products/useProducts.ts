import { useQuery, useInfiniteQuery } from '@tanstack/react-query'
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

export function useProductListInfinite(params: { categoryId?: number; limit?: number }) {
  return useInfiniteQuery({
    queryKey: ['products', 'infinite', params],
    queryFn: ({ pageParam }) => productApi.list({ ...params, page: pageParam as number }),
    initialPageParam: 0,
    getNextPageParam: (_lastPage, _allPages, lastPageParam) => {
      const meta = _lastPage.meta
      if (!meta || (lastPageParam as number) + 1 >= meta.totalPages) return undefined
      return (lastPageParam as number) + 1
    },
    enabled: !!params.categoryId,
    staleTime: 3 * 60_000,
  })
}

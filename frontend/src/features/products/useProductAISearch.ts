import { useQuery } from '@tanstack/react-query'
import { productApi } from './productApi'

export function useProductAISearch(q: string, limit = 20, categoryId?: number, sellerId?: string) {
  return useQuery({
    queryKey: ['products', 'ai-search', q, limit, categoryId, sellerId],
    queryFn: () => productApi.aiSearch(q, limit, categoryId, sellerId),
    enabled: q.trim().length >= 2,
    staleTime: 60_000,
  })
}

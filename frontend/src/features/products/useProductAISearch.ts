import { useQuery } from '@tanstack/react-query'
import { productApi } from './productApi'

export function useProductAISearch(q: string, limit = 20, categoryId?: number) {
  return useQuery({
    queryKey: ['products', 'ai-search', q, limit, categoryId],
    queryFn: () => productApi.aiSearch(q, limit, categoryId),
    enabled: q.trim().length >= 2,
    staleTime: 60_000,
  })
}

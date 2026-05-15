import { useQuery } from '@tanstack/react-query'
import { productApi } from './productApi'

export function useProductAISearch(q: string, limit = 20) {
  return useQuery({
    queryKey: ['products', 'ai-search', q, limit],
    queryFn: () => productApi.aiSearch(q, limit),
    enabled: q.trim().length >= 2,
    staleTime: 60_000,
  })
}

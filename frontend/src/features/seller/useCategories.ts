import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { categoryApi } from './categoryApi'

export function useCategories() {
  return useQuery({
    queryKey: ['categories'],
    queryFn: () => categoryApi.list(),
    staleTime: 10 * 60_000,
  })
}

export function useCreateCategory() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => categoryApi.create(name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['categories'] })
    },
  })
}

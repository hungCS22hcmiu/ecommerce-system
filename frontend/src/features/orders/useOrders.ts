import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { orderApi } from './orderApi'
import { useCartStore } from '@/store/cartStore'
import type { CreateOrderRequest } from '@/types/order'

export function useOrder(id: string) {
  return useQuery({
    queryKey: ['order', id],
    queryFn: () => orderApi.getById(id),
    enabled: !!id,
    staleTime: 30 * 60_000,
  })
}

export function useCreateOrder() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (req: CreateOrderRequest) => orderApi.create(req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['cart'] })
      useCartStore.getState().setItemCount(0)
    },
  })
}

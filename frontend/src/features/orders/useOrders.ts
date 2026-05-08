import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { orderApi } from './orderApi'
import { useCartStore } from '@/store/cartStore'
import type { CreateOrderRequest } from '@/types/order'

export function useOrders(page = 0) {
  return useQuery({
    queryKey: ['orders', page],
    queryFn: () => orderApi.list(page),
    staleTime: 30_000,
  })
}

export function useOrderHistory(orderId: string) {
  return useQuery({
    queryKey: ['order', orderId, 'history'],
    queryFn: () => orderApi.getHistory(orderId),
    enabled: !!orderId,
    staleTime: 5 * 60_000,
  })
}

export function useCancelOrder() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => orderApi.cancel(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: ['orders'] })
      queryClient.invalidateQueries({ queryKey: ['order', id] })
    },
  })
}

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

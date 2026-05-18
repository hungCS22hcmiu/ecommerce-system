import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { sellerOrderApi } from './sellerOrderApi'
import type { OrderStatus } from '@/types/order'

export function useSellerOrders(status?: OrderStatus, page = 0) {
  return useQuery({
    queryKey: ['seller-orders', status, page],
    queryFn: () => sellerOrderApi.list({ status, page }),
    staleTime: 30_000,
  })
}

export function useSellerOrder(id: string) {
  return useQuery({
    queryKey: ['seller-order', id],
    queryFn: () => sellerOrderApi.getById(id),
    enabled: !!id,
    staleTime: 30 * 60_000,
  })
}

export function useShipOrder() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => sellerOrderApi.ship(id),
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: ['seller-orders'] })
      qc.invalidateQueries({ queryKey: ['seller-order', id] })
    },
  })
}

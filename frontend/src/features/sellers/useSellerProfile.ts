import { useQuery } from '@tanstack/react-query'
import { sellerProfileApi } from './sellerProfileApi'

export function useSellerProfile(sellerId: string | undefined) {
  return useQuery({
    queryKey: ['seller-profile', sellerId],
    queryFn: () => sellerProfileApi.getById(sellerId!),
    enabled: !!sellerId,
    staleTime: 5 * 60_000,
  })
}

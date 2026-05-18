import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { reviewApi } from './reviewApi'
import type { CreateReviewRequest, UpdateReviewRequest } from '@/types/review'

export function useProductReviews(productId: number, page = 0) {
  return useQuery({
    queryKey: ['products', 'reviews', productId, page],
    queryFn: () => reviewApi.list(productId, page),
    staleTime: 60_000,
  })
}

export function useMyReviewByOrderItem(productId: number, orderItemId: string) {
  return useQuery({
    queryKey: ['reviews', 'my', orderItemId],
    queryFn: () => reviewApi.getByOrderItem(productId, orderItemId),
    staleTime: 60_000,
  })
}

export function useCreateReview(productId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: CreateReviewRequest) => reviewApi.create(productId, req),
    onSuccess: (_, req) => {
      qc.invalidateQueries({ queryKey: ['product', productId] })
      qc.invalidateQueries({ queryKey: ['products', 'reviews', productId] })
      qc.invalidateQueries({ queryKey: ['reviews', 'my', req.orderItemId] })
    },
  })
}

export function useUpdateReview(productId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ reviewId, req }: { reviewId: number; req: UpdateReviewRequest }) =>
      reviewApi.update(productId, reviewId, req),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ['product', productId] })
      qc.invalidateQueries({ queryKey: ['products', 'reviews', productId] })
      qc.invalidateQueries({ queryKey: ['reviews', 'my', data.data.orderItemId] })
    },
  })
}

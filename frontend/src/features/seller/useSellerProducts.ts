import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { sellerApi } from './sellerApi'
import { showToast } from '@/lib/toast'
import { extractApiError } from '@/lib/utils'
import type { CreateProductBody, UpdateProductBody } from '@/types/seller'

interface MyProductsParams {
  status?: string
  page?: number
  size?: number
}

export function useMyProducts(params: MyProductsParams = {}) {
  return useQuery({
    queryKey: ['seller', 'products', params],
    queryFn: () => sellerApi.listMyProducts(params),
    staleTime: 30_000,
  })
}

export function useSellerProduct(id: number) {
  return useQuery({
    queryKey: ['seller', 'product', id],
    queryFn: () => sellerApi.getProductById(id),
    enabled: !!id,
    staleTime: 30_000,
  })
}

export function useCreateProduct() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  return useMutation({
    mutationFn: (body: CreateProductBody) => sellerApi.createProduct(body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['seller', 'products'] })
      showToast('Product created', 'success')
      navigate('/seller/products')
    },
    onError: (err) => {
      showToast(extractApiError(err) ?? 'Failed to create product', 'error')
    },
  })
}

export function useUpdateProduct() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: UpdateProductBody }) =>
      sellerApi.updateProduct(id, body),
    onSuccess: (_data, { id }) => {
      queryClient.invalidateQueries({ queryKey: ['seller', 'products'] })
      queryClient.invalidateQueries({ queryKey: ['product', id] })
      showToast('Product updated', 'success')
      navigate('/seller/products')
    },
    onError: (err) => {
      showToast(extractApiError(err) ?? 'Failed to update product', 'error')
    },
  })
}

export function useDeleteProduct() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (id: number) => sellerApi.deleteProduct(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['seller', 'products'] })
      showToast('Product deleted', 'success')
    },
    onError: (err) => {
      showToast(extractApiError(err) ?? 'Failed to delete product', 'error')
    },
  })
}

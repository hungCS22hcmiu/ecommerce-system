import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { cartApi } from './cartApi'
import { useCartStore } from '@/store/cartStore'
import type { ApiResponse } from '@/types/api'
import type { Cart, AddCartItemRequest } from '@/types/cart'

export function useCart() {
  return useQuery({
    queryKey: ['cart'],
    queryFn: async () => {
      const res = await cartApi.getCart()
      const count = res.data?.items?.reduce((sum, i) => sum + i.quantity, 0) ?? 0
      useCartStore.getState().setItemCount(count)
      return res
    },
    staleTime: 0,
  })
}

export function useCartMutations() {
  const queryClient = useQueryClient()

  const addItem = useMutation({
    mutationFn: (req: AddCartItemRequest) => cartApi.addItem(req),
    onMutate: async () => {
      await queryClient.cancelQueries({ queryKey: ['cart'] })
      const snapshot = queryClient.getQueryData(['cart'])
      useCartStore.getState().increment()
      return { snapshot }
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.snapshot) queryClient.setQueryData(['cart'], ctx.snapshot)
      useCartStore.getState().decrement()
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: ['cart'] }),
  })

  const updateItem = useMutation({
    mutationFn: ({ productId, quantity }: { productId: number; quantity: number }) =>
      cartApi.updateItem(productId, quantity),
    onMutate: async ({ productId, quantity }) => {
      await queryClient.cancelQueries({ queryKey: ['cart'] })
      const snapshot = queryClient.getQueryData<ApiResponse<Cart>>(['cart'])
      queryClient.setQueryData<ApiResponse<Cart>>(['cart'], (old) => {
        if (!old?.data) return old
        const items = old.data.items.map((item) =>
          item.product_id === productId
            ? { ...item, quantity, subtotal: item.unit_price * quantity }
            : item
        )
        const total = items.reduce((sum, i) => sum + i.subtotal, 0)
        const itemCount = items.reduce((sum, i) => sum + i.quantity, 0)
        useCartStore.getState().setItemCount(itemCount)
        return { ...old, data: { ...old.data, items, total } }
      })
      return { snapshot }
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.snapshot) {
        queryClient.setQueryData(['cart'], ctx.snapshot)
        const count =
          (ctx.snapshot as ApiResponse<Cart>)?.data?.items?.reduce(
            (sum, i) => sum + i.quantity,
            0
          ) ?? 0
        useCartStore.getState().setItemCount(count)
      }
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: ['cart'] }),
  })

  const removeItem = useMutation({
    mutationFn: (productId: number) => cartApi.removeItem(productId),
    onMutate: async (productId) => {
      await queryClient.cancelQueries({ queryKey: ['cart'] })
      const snapshot = queryClient.getQueryData<ApiResponse<Cart>>(['cart'])
      queryClient.setQueryData<ApiResponse<Cart>>(['cart'], (old) => {
        if (!old?.data) return old
        const items = old.data.items.filter((i) => i.product_id !== productId)
        const total = items.reduce((sum, i) => sum + i.subtotal, 0)
        const itemCount = items.reduce((sum, i) => sum + i.quantity, 0)
        useCartStore.getState().setItemCount(itemCount)
        return { ...old, data: { ...old.data, items, total } }
      })
      return { snapshot }
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.snapshot) {
        queryClient.setQueryData(['cart'], ctx.snapshot)
        const count =
          (ctx.snapshot as ApiResponse<Cart>)?.data?.items?.reduce(
            (sum, i) => sum + i.quantity,
            0
          ) ?? 0
        useCartStore.getState().setItemCount(count)
      }
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: ['cart'] }),
  })

  const clearCart = useMutation({
    mutationFn: () => cartApi.clearCart(),
    onSuccess: () => {
      useCartStore.getState().setItemCount(0)
      queryClient.invalidateQueries({ queryKey: ['cart'] })
    },
  })

  return { addItem, updateItem, removeItem, clearCart }
}

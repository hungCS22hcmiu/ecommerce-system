import { create } from 'zustand'

interface CartState {
  itemCount: number
  setItemCount: (n: number) => void
  increment: () => void
  decrement: () => void
}

export const useCartStore = create<CartState>((set) => ({
  itemCount: 0,
  setItemCount: (n) => set({ itemCount: n }),
  increment: () => set((s) => ({ itemCount: s.itemCount + 1 })),
  decrement: () => set((s) => ({ itemCount: Math.max(0, s.itemCount - 1) })),
}))

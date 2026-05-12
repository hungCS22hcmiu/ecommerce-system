import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface AuthState {
  accessToken: string | null
  refreshToken: string | null
  userId: string | null
  email: string | null
  role: string | null
  setAuth: (
    tokens: { accessToken: string; refreshToken: string },
    userId: string,
    email: string,
    role: string
  ) => void
  setToken: (accessToken: string) => void
  clearAuth: () => void
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      accessToken: null,
      refreshToken: null,
      userId: null,
      email: null,
      role: null,
      setAuth: (tokens, userId, email, role) =>
        set({
          accessToken: tokens.accessToken,
          refreshToken: tokens.refreshToken,
          userId,
          email,
          role,
        }),
      setToken: (accessToken) => set({ accessToken }),
      clearAuth: () =>
        set({ accessToken: null, refreshToken: null, userId: null, email: null, role: null }),
    }),
    {
      name: 'auth',
      // accessToken stays in memory only (XSS safety)
      partialize: (s) => ({
        refreshToken: s.refreshToken,
        userId: s.userId,
        email: s.email,
        role: s.role,
      }),
    }
  )
)

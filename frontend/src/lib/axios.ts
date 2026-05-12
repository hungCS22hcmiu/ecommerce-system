import axios from 'axios'
import { useAuthStore } from '@/store/authStore'
import { showToast } from '@/lib/toast'

export const api = axios.create({ baseURL: '/api/v1' })

api.interceptors.request.use((config) => {
  const { accessToken, userId } = useAuthStore.getState()
  if (accessToken) config.headers['Authorization'] = `Bearer ${accessToken}`
  if (userId) config.headers['X-User-Id'] = userId
  return config
})

let isRefreshing = false
let failedQueue: { resolve: (token: string) => void; reject: (err: unknown) => void }[] = []

function processQueue(error: unknown, token: string | null) {
  failedQueue.forEach((p) => (token ? p.resolve(token) : p.reject(error)))
  failedQueue = []
}

api.interceptors.response.use(
  (res) => res,
  async (error) => {
    const original = error.config as typeof error.config & { _retry?: boolean }
    const isAuthEndpoint = original.url?.includes('/auth/login') || original.url?.includes('/auth/register')
    if (error.response?.status === 401 && !original._retry && !isAuthEndpoint) {
      if (isRefreshing) {
        return new Promise<string>((resolve, reject) => {
          failedQueue.push({ resolve, reject })
        }).then((token) => {
          original.headers['Authorization'] = `Bearer ${token}`
          return api(original)
        })
      }
      original._retry = true
      isRefreshing = true
      const refreshToken = useAuthStore.getState().refreshToken
      try {
        const { data } = await api.post('/auth/refresh', { refresh_token: refreshToken })
        const newToken: string = data.data.access_token
        useAuthStore.getState().setToken(newToken)
        processQueue(null, newToken)
        original.headers['Authorization'] = `Bearer ${newToken}`
        return api(original)
      } catch (err) {
        processQueue(err, null)
        useAuthStore.getState().clearAuth()
        window.location.href = '/login'
        return Promise.reject(err)
      } finally {
        isRefreshing = false
      }
    }
    return Promise.reject(error)
  }
)

// Show toast only for true network failures (no response received)
api.interceptors.response.use(
  (res) => res,
  (error) => {
    if (!error.response && error.request) {
      showToast('Network error. Please check your connection.', 'error')
    }
    return Promise.reject(error)
  }
)

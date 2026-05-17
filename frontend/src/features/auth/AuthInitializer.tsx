import { useEffect } from 'react'
import { useAuthStore } from '@/store/authStore'
import { api } from '@/lib/axios'

export function AuthInitializer() {
  const setToken = useAuthStore((s) => s.setToken)
  const clearAuth = useAuthStore((s) => s.clearAuth)
  const setIsInitialized = useAuthStore((s) => s.setIsInitialized)

  useEffect(() => {
    const run = () => {
      const { accessToken: token, refreshToken: refresh } = useAuthStore.getState()
      if (!token && refresh) {
        const controller = new AbortController()
        const timeoutId = setTimeout(() => controller.abort(), 5000)

        api
          .post('/auth/refresh', { refresh_token: refresh }, { signal: controller.signal })
          .then(({ data }) => setToken(data.data.access_token))
          .catch(() => clearAuth())
          .finally(() => {
            clearTimeout(timeoutId)
            setIsInitialized(true)
          })
      } else {
        setIsInitialized(true)
      }
    }

    if (useAuthStore.persist.hasHydrated()) {
      run()
    } else {
      const unsub = useAuthStore.persist.onFinishHydration(run)
      return unsub
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  return null
}

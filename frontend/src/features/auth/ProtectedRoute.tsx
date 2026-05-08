import { useState, useEffect } from 'react'
import { Outlet, Navigate, useLocation } from 'react-router-dom'
import { useAuthStore } from '@/store/authStore'
import { api } from '@/lib/axios'

export function ProtectedRoute() {
  const token = useAuthStore((s) => s.accessToken)
  const refreshToken = useAuthStore((s) => s.refreshToken)
  const setToken = useAuthStore((s) => s.setToken)
  const clearAuth = useAuthStore((s) => s.clearAuth)
  const location = useLocation()
  const [checking, setChecking] = useState(!token)

  useEffect(() => {
    if (!token && refreshToken) {
      api
        .post('/auth/refresh', { refresh_token: refreshToken })
        .then(({ data }) => setToken(data.data.access_token))
        .catch(() => clearAuth())
        .finally(() => setChecking(false))
    } else {
      setChecking(false)
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  if (checking) {
    return (
      <div className="min-h-screen bg-surface-base flex items-center justify-center">
        <div className="h-8 w-8 border-2 border-accent border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  if (!token) {
    return (
      <Navigate
        to={`/login?from=${encodeURIComponent(location.pathname)}`}
        replace
      />
    )
  }

  return <Outlet />
}

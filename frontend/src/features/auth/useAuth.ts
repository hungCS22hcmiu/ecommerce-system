import { useMutation } from '@tanstack/react-query'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { authApi } from './authApi'
import { useAuthStore } from '@/store/authStore'

export function useLogin() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()

  return useMutation({
    mutationFn: authApi.login,
    onSuccess: (data) => {
      const { access_token, refresh_token, user } = data.data
      useAuthStore.getState().setAuth(
        { accessToken: access_token, refreshToken: refresh_token },
        user.id,
        user.email
      )
      const from = searchParams.get('from') ?? '/products'
      navigate(from, { replace: true })
    },
  })
}

export function useRegister() {
  const navigate = useNavigate()

  return useMutation({
    mutationFn: authApi.register,
    onSuccess: (_data, variables) => {
      navigate(`/verify-email?email=${encodeURIComponent(variables.email)}`)
    },
  })
}

export function useVerifyEmail() {
  const navigate = useNavigate()

  return useMutation({
    mutationFn: authApi.verifyEmail,
    onSuccess: () => navigate('/login'),
  })
}

export function useResendVerification() {
  return useMutation({ mutationFn: authApi.resendVerification })
}

export function useLogout() {
  const clearAuth = useAuthStore((s) => s.clearAuth)
  const navigate = useNavigate()

  return useMutation({
    mutationFn: authApi.logout,
    onSettled: () => {
      clearAuth()
      navigate('/login', { replace: true })
    },
  })
}

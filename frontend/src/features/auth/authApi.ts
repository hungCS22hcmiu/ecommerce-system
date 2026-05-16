import { api } from '@/lib/axios'
import type { LoginRequest, RegisterRequest, AuthResponse, UserProfile } from '@/types/auth'
import type { ApiResponse } from '@/types/api'

export const authApi = {
  login: (body: LoginRequest) =>
    api.post<ApiResponse<AuthResponse>>('/auth/login', body).then((r) => r.data),

  register: (body: RegisterRequest) =>
    api
      .post<ApiResponse<{ id: string; email: string; first_name: string; last_name: string }>>(
        '/auth/register',
        body
      )
      .then((r) => r.data),

  logout: () => api.post('/auth/logout'),

  verifyEmail: (body: { email: string; code: string }) =>
    api.post('/auth/verify-email', body).then((r) => r.data),

  resendVerification: (body: { email: string }) =>
    api.post('/auth/resend-verification', body).then((r) => r.data),

  forgotPassword: (body: { email: string }) =>
    api.post('/auth/forgot-password', body).then((r) => r.data),

  resetPassword: (body: { email: string; token: string; password: string }) =>
    api.post('/auth/reset-password', body).then((r) => r.data),

  getProfile: () =>
    api.get<ApiResponse<UserProfile>>('/users/profile').then((r) => r.data),
}

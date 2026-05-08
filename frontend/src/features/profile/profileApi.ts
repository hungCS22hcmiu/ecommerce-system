import { api } from '@/lib/axios'
import type { ApiResponse } from '@/types/api'
import type { UserProfile, Address } from '@/types/auth'

export interface UpdateProfileRequest {
  first_name: string
  last_name: string
  phone?: string
}

export interface AddressRequest {
  label?: string
  address_line1: string
  address_line2?: string
  city: string
  state?: string
  country: string
  postal_code?: string
}

export const profileApi = {
  update: (body: UpdateProfileRequest) =>
    api.put<ApiResponse<UserProfile>>('/users/profile', body).then((r) => r.data),

  addAddress: (body: AddressRequest) =>
    api.post<ApiResponse<Address>>('/users/addresses', body).then((r) => r.data),

  updateAddress: (id: string, body: AddressRequest) =>
    api.put<ApiResponse<Address>>(`/users/addresses/${id}`, body).then((r) => r.data),

  deleteAddress: (id: string) => api.delete(`/users/addresses/${id}`),

  setDefault: (id: string) =>
    api.put<ApiResponse<Address>>(`/users/addresses/${id}/default`).then((r) => r.data),
}

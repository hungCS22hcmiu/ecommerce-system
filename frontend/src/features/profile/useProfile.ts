import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { authApi } from '@/features/auth/authApi'
import { profileApi } from './profileApi'
import { showToast } from '@/lib/toast'
import type { UpdateProfileRequest, AddressRequest } from './profileApi'

export function useProfile() {
  return useQuery({
    queryKey: ['profile'],
    queryFn: () => authApi.getProfile(),
    staleTime: 5 * 60_000,
  })
}

export function useUpdateProfile() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: UpdateProfileRequest) => profileApi.update(body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['profile'] })
      showToast('Profile updated successfully.', 'success')
    },
  })
}

export function useAddAddress() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: AddressRequest) => profileApi.addAddress(body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['profile'] })
      showToast('Address added.', 'success')
    },
  })
}

export function useUpdateAddress() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: AddressRequest }) =>
      profileApi.updateAddress(id, body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['profile'] })
      showToast('Address updated.', 'success')
    },
  })
}

export function useDeleteAddress() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => profileApi.deleteAddress(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['profile'] })
    },
  })
}

export function useSetDefaultAddress() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => profileApi.setDefault(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['profile'] })
    },
  })
}

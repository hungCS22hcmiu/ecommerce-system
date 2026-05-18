import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useAuthStore } from '@/store/authStore'
import { notificationApi } from './notificationApi'

export function useNotificationSummary() {
  const accessToken = useAuthStore((s) => s.accessToken)
  return useQuery({
    queryKey: ['notifications'],
    queryFn: () => notificationApi.getSummary(),
    refetchInterval: 30_000,
    enabled: !!accessToken,
    staleTime: 25_000,
  })
}

export function useMarkAllRead() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => notificationApi.markAllRead(),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['notifications'] }),
  })
}

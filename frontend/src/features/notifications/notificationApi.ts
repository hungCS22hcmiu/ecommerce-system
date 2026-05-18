import { api } from '@/lib/axios'
import type { ApiResponse } from '@/types/api'
import type { NotificationSummary } from '@/types/notification'

export const notificationApi = {
  getSummary: () =>
    api.get<ApiResponse<NotificationSummary>>('/orders/notifications').then((r) => r.data),

  markAllRead: () =>
    api.put<ApiResponse<null>>('/orders/notifications/mark-read').then((r) => r.data),
}

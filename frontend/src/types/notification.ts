export interface AppNotification {
  id: string
  orderId: string | null
  productId: number | null
  title: string
  body?: string
  isRead: boolean
  createdAt: string
}

export interface NotificationSummary {
  unreadCount: number
  items: AppNotification[]
}

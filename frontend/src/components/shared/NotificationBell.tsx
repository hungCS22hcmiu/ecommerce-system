import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useNotificationSummary, useMarkAllRead } from '@/features/notifications/useNotifications'
import { Badge } from '@/components/ui/badge'
import { formatDate } from '@/lib/utils'

export function NotificationBell() {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  const navigate = useNavigate()
  const { data } = useNotificationSummary()
  const markAllRead = useMarkAllRead()

  const summary = data?.data
  const unreadCount = summary?.unreadCount ?? 0
  const items = summary?.items ?? []

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [])

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className="relative flex items-center text-fg-muted hover:text-fg-base transition-colors px-2 py-1"
        aria-label="Notifications"
      >
        <svg
          xmlns="http://www.w3.org/2000/svg"
          className="h-5 w-5"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={1.5}
            d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9"
          />
        </svg>
        {unreadCount > 0 && (
          <Badge
            variant="amber"
            className="absolute -top-1 -right-1 text-xs px-1.5 py-0 min-w-[1.25rem] h-5 flex items-center justify-center"
          >
            {unreadCount > 99 ? '99+' : unreadCount}
          </Badge>
        )}
      </button>

      {open && (
        <div className="absolute right-0 top-full mt-2 w-80 bg-surface-raised border border-surface-border rounded-lg shadow-lg z-50 overflow-hidden">
          <div className="flex items-center justify-between px-4 py-3 border-b border-surface-border">
            <span className="text-sm font-semibold text-fg-base">Notifications</span>
            {unreadCount > 0 && (
              <button
                type="button"
                onClick={() => markAllRead.mutate()}
                disabled={markAllRead.isPending}
                className="text-xs text-accent hover:text-accent-dim transition-colors disabled:opacity-40"
              >
                Mark all read
              </button>
            )}
          </div>

          <div className="max-h-80 overflow-y-auto divide-y divide-surface-border">
            {items.length === 0 ? (
              <p className="px-4 py-6 text-sm text-fg-subtle text-center">No notifications yet</p>
            ) : (
              items.map((n) => (
                <button
                  key={n.id}
                  type="button"
                  onClick={() => {
                    setOpen(false)
                    if (n.productId) {
                      navigate(`/products/${n.productId}`)
                    } else if (n.orderId) {
                      navigate(`/orders/${n.orderId}`)
                    }
                  }}
                  className={`w-full text-left px-4 py-3 cursor-pointer hover:bg-surface-overlay transition-colors ${!n.isRead ? 'border-l-2 border-amber-500 bg-amber-500/5' : ''}`}
                >
                  <p className={`text-sm ${!n.isRead ? 'font-semibold text-fg-base' : 'font-medium text-fg-muted'}`}>
                    {n.title}
                  </p>
                  {n.body && (
                    <p className="text-xs text-fg-subtle mt-0.5 leading-relaxed">{n.body}</p>
                  )}
                  <p className="text-xs text-fg-subtle mt-1">{formatDate(n.createdAt)}</p>
                </button>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  )
}

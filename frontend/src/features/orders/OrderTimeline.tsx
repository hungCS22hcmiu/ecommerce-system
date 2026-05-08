import { formatDate } from '@/lib/utils'
import type { OrderStatusHistory, OrderStatus } from '@/types/order'

const dotColor: Record<OrderStatus, string> = {
  PENDING:   'border-zinc-500  bg-zinc-500',
  CONFIRMED: 'border-blue-500  bg-blue-500',
  SHIPPED:   'border-amber-500 bg-amber-500',
  DELIVERED: 'border-emerald-500 bg-emerald-500',
  CANCELLED: 'border-red-500   bg-red-500',
}

interface OrderTimelineProps {
  history: OrderStatusHistory[]
  createdAt?: string
}

export function OrderTimeline({ history, createdAt }: OrderTimelineProps) {
  type Entry = { status: OrderStatus; timestamp: string; reason?: string }

  const entries: Entry[] = []

  if (createdAt) {
    entries.push({ status: 'PENDING', timestamp: createdAt })
  }

  for (const h of history) {
    entries.push({
      status: h.newStatus as OrderStatus,
      timestamp: h.changedAt,
      reason: h.reason,
    })
  }

  if (entries.length === 0) {
    return <p className="text-sm text-fg-subtle">No history yet.</p>
  }

  return (
    <div>
      {entries.map((entry, i) => (
        <div key={`${entry.status}-${i}`} className="flex gap-3">
          <div className="flex flex-col items-center">
            <div
              className={`w-3 h-3 rounded-full mt-1 flex-shrink-0 border-2 ${dotColor[entry.status] ?? 'border-zinc-500 bg-zinc-500'}`}
            />
            {i < entries.length - 1 && (
              <div className="w-0.5 flex-1 bg-surface-border my-1 min-h-[1.5rem]" />
            )}
          </div>
          <div className="pb-5">
            <p className="text-sm font-semibold text-fg-base capitalize">
              {entry.status.charAt(0) + entry.status.slice(1).toLowerCase()}
            </p>
            <p className="text-xs text-fg-subtle">{formatDate(entry.timestamp)}</p>
            {entry.reason && (
              <p className="text-xs text-fg-muted mt-0.5">{entry.reason}</p>
            )}
          </div>
        </div>
      ))}
    </div>
  )
}

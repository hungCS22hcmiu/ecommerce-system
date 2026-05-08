import { Badge } from '@/components/ui/badge'
import type { OrderStatus } from '@/types/order'

type BadgeVariant = 'zinc' | 'blue' | 'amber' | 'emerald' | 'red'

const statusMap: Record<OrderStatus, { label: string; variant: BadgeVariant }> = {
  PENDING:   { label: 'Pending',   variant: 'zinc' },
  CONFIRMED: { label: 'Confirmed', variant: 'blue' },
  SHIPPED:   { label: 'Shipped',   variant: 'amber' },
  DELIVERED: { label: 'Delivered', variant: 'emerald' },
  CANCELLED: { label: 'Cancelled', variant: 'red' },
}

export function StatusBadge({ status }: { status: OrderStatus }) {
  const map = statusMap[status] ?? { label: status, variant: 'zinc' as BadgeVariant }
  return <Badge variant={map.variant}>{map.label}</Badge>
}

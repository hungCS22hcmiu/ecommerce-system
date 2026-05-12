import { Badge } from '@/components/ui/badge'

interface Props {
  status: 'ACTIVE' | 'INACTIVE' | 'DELETED'
}

const variantMap = {
  ACTIVE: 'emerald',
  INACTIVE: 'amber',
  DELETED: 'red',
} as const

export function ProductStatusBadge({ status }: Props) {
  return <Badge variant={variantMap[status]}>{status}</Badge>
}

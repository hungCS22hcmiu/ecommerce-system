import { cn } from '@/lib/utils'

export function Skeleton({ className }: { className?: string }) {
  return <div className={cn('bg-surface-raised animate-pulse rounded-md', className)} />
}

import type { ReactNode } from 'react'

interface EmptyStateProps {
  title: string
  description?: string
  action?: ReactNode
}

export function EmptyState({ title, description, action }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center justify-center py-20 text-center gap-3">
      <p className="text-lg font-semibold text-fg-base">{title}</p>
      {description && <p className="text-sm text-fg-subtle">{description}</p>}
      {action}
    </div>
  )
}

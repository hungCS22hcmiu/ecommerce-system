import { cn } from '@/lib/utils'
import type { HTMLAttributes } from 'react'

interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  variant?: 'default' | 'amber' | 'blue' | 'emerald' | 'red' | 'zinc'
}

const variantClasses = {
  default: 'bg-surface-raised text-fg-muted border border-surface-border',
  amber: 'bg-amber-500/10 text-amber-400 light:text-amber-700 border border-amber-500/30',
  blue: 'bg-blue-500/10 text-blue-400 light:text-blue-700 border border-blue-500/30',
  emerald: 'bg-emerald-500/10 text-emerald-400 light:text-emerald-700 border border-emerald-500/30',
  red: 'bg-red-500/10 text-red-400 light:text-red-700 border border-red-500/30',
  zinc: 'bg-zinc-500/10 text-fg-muted border border-zinc-500/30',
}

export function Badge({ className, variant = 'default', ...props }: BadgeProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium font-mono',
        variantClasses[variant],
        className
      )}
      {...props}
    />
  )
}

import { Link, useSearchParams } from 'react-router-dom'
import { cn } from '@/lib/utils'
import type { PaginationMeta } from '@/types/api'

interface PaginationProps {
  meta: PaginationMeta
}

export function Pagination({ meta }: PaginationProps) {
  const [searchParams] = useSearchParams()
  // meta.page is 0-indexed; URL page is 1-indexed
  const currentPage = meta.page + 1
  const totalPages = meta.totalPages

  if (totalPages <= 1) return null

  const buildHref = (p: number) => {
    const next = new URLSearchParams(searchParams)
    next.set('page', String(p))
    return `?${next.toString()}`
  }

  const pages = getPageRange(currentPage, totalPages)

  return (
    <div className="flex items-center justify-center gap-1 mt-8">
      <PageLink href={buildHref(currentPage - 1)} disabled={currentPage <= 1}>
        ←
      </PageLink>

      {pages.map((p, i) =>
        p === '...' ? (
          <span key={`ellipsis-${i}`} className="px-2 text-zinc-600">…</span>
        ) : (
          <PageLink key={p} href={buildHref(p as number)} active={p === currentPage}>
            {p}
          </PageLink>
        )
      )}

      <PageLink href={buildHref(currentPage + 1)} disabled={currentPage >= totalPages}>
        →
      </PageLink>
    </div>
  )
}

function PageLink({
  href,
  children,
  active,
  disabled,
}: {
  href: string
  children: React.ReactNode
  active?: boolean
  disabled?: boolean
}) {
  if (disabled) {
    return (
      <span className="w-9 h-9 flex items-center justify-center rounded-md text-sm text-zinc-600 cursor-not-allowed">
        {children}
      </span>
    )
  }
  return (
    <Link
      to={href}
      className={cn(
        'w-9 h-9 flex items-center justify-center rounded-md text-sm transition-colors',
        active
          ? 'bg-accent text-surface-base font-semibold'
          : 'text-zinc-400 hover:text-white hover:bg-surface-raised'
      )}
    >
      {children}
    </Link>
  )
}

function getPageRange(current: number, total: number): (number | '...')[] {
  if (total <= 7) return Array.from({ length: total }, (_, i) => i + 1)

  if (current <= 4) return [1, 2, 3, 4, 5, '...', total]
  if (current >= total - 3) return [1, '...', total - 4, total - 3, total - 2, total - 1, total]
  return [1, '...', current - 1, current, current + 1, '...', total]
}

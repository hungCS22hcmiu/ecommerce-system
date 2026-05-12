import { useState, useRef, useEffect } from 'react'
import { useCategories, useCreateCategory } from './useCategories'
import type { Category } from '@/types/category'
import { cn } from '@/lib/utils'

interface Props {
  value: { id: number | null; name: string }
  onChange: (id: number | null, name: string) => void
}

export function CategoryCombobox({ value, onChange }: Props) {
  const [query, setQuery] = useState(value.name)
  const [open, setOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)

  const { data } = useCategories()
  const createCategory = useCreateCategory()

  const categories: Category[] = data?.data ?? []

  const filtered = query.trim()
    ? categories.filter((c) => c.name.toLowerCase().includes(query.toLowerCase()))
    : categories

  const exactMatch = categories.some(
    (c) => c.name.toLowerCase() === query.trim().toLowerCase()
  )
  const showCreate = query.trim().length > 0 && !exactMatch

  // Sync external value.name → local query when form resets (e.g. edit pre-fill)
  useEffect(() => {
    setQuery(value.name)
  }, [value.name])

  // Close on outside click
  useEffect(() => {
    function handle(e: MouseEvent) {
      if (!containerRef.current?.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handle)
    return () => document.removeEventListener('mousedown', handle)
  }, [])

  function selectCategory(cat: Category) {
    setQuery(cat.name)
    onChange(cat.id, cat.name)
    setOpen(false)
  }

  function clearSelection() {
    setQuery('')
    onChange(null, '')
  }

  async function handleCreate() {
    const name = query.trim()
    if (!name) return
    const result = await createCategory.mutateAsync(name)
    const created = result.data
    setQuery(created.name)
    onChange(created.id, created.name)
    setOpen(false)
  }

  return (
    <div ref={containerRef} className="relative">
      <div className="flex gap-2">
        <div className="relative flex-1">
          <input
            value={query}
            onChange={(e) => {
              setQuery(e.target.value)
              onChange(null, e.target.value)
              setOpen(true)
            }}
            onFocus={() => setOpen(true)}
            placeholder="Search or create a category…"
            className="flex h-10 w-full rounded-md border border-surface-border bg-surface-overlay px-3 py-2 text-sm text-fg-base placeholder:text-fg-subtle focus:outline-none focus:ring-1 focus:ring-accent"
          />
          {value.id && (
            <span className="absolute right-2.5 top-1/2 -translate-y-1/2 text-xs text-fg-subtle font-mono">
              #{value.id}
            </span>
          )}
        </div>
        {(value.id || query) && (
          <button
            type="button"
            onClick={clearSelection}
            className="text-xs text-fg-subtle hover:text-fg-base transition-colors px-2"
          >
            Clear
          </button>
        )}
      </div>

      {open && (filtered.length > 0 || showCreate) && (
        <div className="absolute z-50 mt-1 w-full rounded-md border border-surface-border bg-surface-overlay shadow-lg overflow-hidden">
          <ul className="max-h-56 overflow-y-auto py-1">
            {filtered.map((cat) => (
              <li key={cat.id}>
                <button
                  type="button"
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() => selectCategory(cat)}
                  className={cn(
                    'w-full text-left px-3 py-2 text-sm flex items-center justify-between hover:bg-surface-raised transition-colors',
                    value.id === cat.id && 'bg-surface-raised text-accent'
                  )}
                >
                  <span>{cat.name}</span>
                  {cat.parentId && (
                    <span className="text-xs text-fg-subtle ml-2">subcategory</span>
                  )}
                </button>
              </li>
            ))}
            {showCreate && (
              <li className="border-t border-surface-border">
                <button
                  type="button"
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={handleCreate}
                  disabled={createCategory.isPending}
                  className="w-full text-left px-3 py-2 text-sm text-accent hover:bg-surface-raised transition-colors disabled:opacity-50"
                >
                  {createCategory.isPending
                    ? 'Creating…'
                    : `+ Create "${query.trim()}"`}
                </button>
              </li>
            )}
          </ul>
        </div>
      )}
    </div>
  )
}

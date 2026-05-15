import { useState, useEffect, useCallback } from 'react'
import { Input } from '@/components/ui/input'

interface SearchBarProps {
  onSearch: (q: string) => void
  defaultValue?: string
  mode?: 'keyword' | 'ai'
  onModeChange?: (m: 'keyword' | 'ai') => void
}

export function SearchBar({ onSearch, defaultValue = '', mode = 'keyword', onModeChange }: SearchBarProps) {
  const [value, setValue] = useState(defaultValue)
  const stable = useCallback(onSearch, []) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    const t = setTimeout(() => stable(value), 300)
    return () => clearTimeout(t)
  }, [value, stable])

  // Sync if URL-driven default changes (e.g. browser back/forward)
  useEffect(() => {
    setValue(defaultValue)
  }, [defaultValue])

  return (
    <div className="flex flex-col gap-2">
      <Input
        placeholder="Search products…"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        className="w-full text-base h-11"
      />
      {value.trim().length >= 2 && (
        <div className="flex items-center gap-1 text-sm">
          <button
            onClick={() => onModeChange?.('keyword')}
            className={
              mode !== 'ai'
                ? 'font-semibold text-fg-base'
                : 'text-fg-subtle hover:text-fg-base transition-colors'
            }
          >
            Keyword
          </button>
          <span className="text-fg-subtle">|</span>
          <button
            onClick={() => onModeChange?.('ai')}
            className={
              mode === 'ai'
                ? 'font-semibold text-fg-base'
                : 'text-fg-subtle hover:text-fg-base transition-colors'
            }
          >
            ✨ Smart Search
          </button>
        </div>
      )}
    </div>
  )
}

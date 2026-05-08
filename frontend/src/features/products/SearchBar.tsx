import { useState, useEffect, useCallback } from 'react'
import { Input } from '@/components/ui/input'

interface SearchBarProps {
  onSearch: (q: string) => void
  defaultValue?: string
}

export function SearchBar({ onSearch, defaultValue = '' }: SearchBarProps) {
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
    <Input
      placeholder="Search products…"
      value={value}
      onChange={(e) => setValue(e.target.value)}
      className="w-full text-base h-11"
    />
  )
}

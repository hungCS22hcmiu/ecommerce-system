import { useState, useEffect } from 'react'
import { subscribeToast } from '@/lib/toast'
import type { Toast } from '@/lib/toast'

export function ToastContainer() {
  const [toasts, setToasts] = useState<Toast[]>([])

  useEffect(() => {
    return subscribeToast((t) => {
      setToasts((prev) => [...prev, t])
      setTimeout(() => setToasts((prev) => prev.filter((x) => x.id !== t.id)), 4000)
    })
  }, [])

  if (toasts.length === 0) return null

  return (
    <div className="fixed bottom-4 right-4 z-[100] flex flex-col gap-2 max-w-sm">
      {toasts.map((t) => (
        <div
          key={t.id}
          className={`px-4 py-3 rounded-lg text-sm font-medium shadow-lg ${
            t.level === 'success'
              ? 'bg-status-delivered text-white'
              : t.level === 'error'
              ? 'bg-status-failed text-white'
              : 'bg-surface-overlay border border-surface-border text-white'
          }`}
        >
          {t.message}
        </div>
      ))}
    </div>
  )
}

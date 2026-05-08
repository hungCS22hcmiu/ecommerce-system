export type ToastLevel = 'success' | 'error' | 'info'

export interface Toast {
  id: string
  message: string
  level: ToastLevel
}

type Listener = (t: Toast) => void
const listeners: Listener[] = []

export function showToast(message: string, level: ToastLevel = 'info') {
  const toast: Toast = { id: crypto.randomUUID(), message, level }
  listeners.forEach((fn) => fn(toast))
}

export function subscribeToast(fn: Listener) {
  listeners.push(fn)
  return () => {
    const i = listeners.indexOf(fn)
    if (i >= 0) listeners.splice(i, 1)
  }
}

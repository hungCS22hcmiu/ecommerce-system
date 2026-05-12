import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'
import type { AxiosError } from 'axios'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function formatCurrency(amount: number): string {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(amount)
}

export function truncateId(id: string, chars = 8): string {
  return id.replace(/-/g, '').slice(0, chars)
}

export function formatDate(iso: string): string {
  return new Intl.DateTimeFormat('en-US', {
    year: 'numeric', month: 'short', day: 'numeric',
    hour: '2-digit', minute: '2-digit',
  }).format(new Date(iso))
}

export function extractApiError(err: unknown): string | null {
  const axiosErr = err as AxiosError<{ error?: { message?: string } }>
  return axiosErr?.response?.data?.error?.message ?? null
}

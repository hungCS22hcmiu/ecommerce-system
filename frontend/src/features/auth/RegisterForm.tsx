import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useRegister } from './useAuth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import type { AxiosError } from 'axios'
import type { ApiError } from '@/types/api'

export function RegisterForm() {
  const [form, setForm] = useState({
    first_name: '',
    last_name: '',
    email: '',
    password: '',
  })
  const register = useRegister()

  const apiError = register.error
    ? (register.error as AxiosError<ApiError>).response?.data?.error
    : null

  const fieldError = (field: string) => apiError?.details?.[field]

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((prev) => ({ ...prev, [e.target.name]: e.target.value }))

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    register.mutate(form)
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1">
          <label className="text-sm text-fg-muted">First name</label>
          <Input
            name="first_name"
            placeholder="Jane"
            value={form.first_name}
            onChange={handleChange}
            required
          />
          {fieldError('first_name') && (
            <p className="text-xs text-status-failed">{fieldError('first_name')}</p>
          )}
        </div>
        <div className="space-y-1">
          <label className="text-sm text-fg-muted">Last name</label>
          <Input
            name="last_name"
            placeholder="Doe"
            value={form.last_name}
            onChange={handleChange}
            required
          />
          {fieldError('last_name') && (
            <p className="text-xs text-status-failed">{fieldError('last_name')}</p>
          )}
        </div>
      </div>

      <div className="space-y-1">
        <label className="text-sm text-fg-muted">Email</label>
        <Input
          type="email"
          name="email"
          placeholder="you@example.com"
          value={form.email}
          onChange={handleChange}
          required
          autoComplete="email"
        />
        {fieldError('email') && (
          <p className="text-xs text-status-failed">{fieldError('email')}</p>
        )}
      </div>

      <div className="space-y-1">
        <label className="text-sm text-fg-muted">Password</label>
        <Input
          type="password"
          name="password"
          placeholder="••••••••••••"
          value={form.password}
          onChange={handleChange}
          required
          autoComplete="new-password"
        />
        <p className="text-xs text-fg-subtle">
          Min 8 chars · max 72 chars
        </p>
        {fieldError('password') && (
          <p className="text-xs text-status-failed">{fieldError('password')}</p>
        )}
      </div>

      {apiError && !apiError.details && (
        <p className="text-sm text-status-failed">{apiError.message}</p>
      )}

      <Button type="submit" className="w-full" disabled={register.isPending}>
        {register.isPending ? (
          <span className="flex items-center gap-2">
            <span className="h-4 w-4 border-2 border-surface-base border-t-transparent rounded-full animate-spin" />
            Creating account…
          </span>
        ) : (
          'Create Account'
        )}
      </Button>

      <p className="text-center text-sm text-fg-subtle">
        Already have an account?{' '}
        <Link to="/login" className="text-accent hover:text-accent-dim">
          Sign in →
        </Link>
      </p>
    </form>
  )
}

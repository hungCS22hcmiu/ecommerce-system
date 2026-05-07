import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useLogin } from './useAuth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import type { AxiosError } from 'axios'
import type { ApiError } from '@/types/api'

export function LoginForm() {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const login = useLogin()

  const apiError = login.error
    ? (login.error as AxiosError<ApiError>).response?.data?.error
    : null

  const isUnverified = apiError?.code === 'EMAIL_NOT_VERIFIED'

  const errorMessage = () => {
    if (!apiError) return null
    if (isUnverified) return 'Please verify your email before logging in.'
    if (apiError.code === 'ACCOUNT_LOCKED') return 'Account locked. Try again after 15 minutes.'
    if (apiError.code === 'INVALID_CREDENTIALS') return 'Invalid email or password.'
    return apiError.message ?? 'Something went wrong. Please try again.'
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    login.mutate({ email, password })
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="space-y-1">
        <label className="text-sm text-zinc-400">Email</label>
        <Input
          type="email"
          placeholder="you@example.com"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          required
          autoComplete="email"
        />
      </div>

      <div className="space-y-1">
        <label className="text-sm text-zinc-400">Password</label>
        <Input
          type="password"
          placeholder="••••••••••••"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          required
          autoComplete="current-password"
        />
      </div>

      {errorMessage() && (
        <div className="space-y-2">
          <p className="text-sm text-status-failed">{errorMessage()}</p>
          {isUnverified && (
            <Link
              to={`/verify-email?email=${encodeURIComponent(email)}`}
              className="inline-block text-sm text-accent hover:text-accent-dim"
            >
              Verify your email →
            </Link>
          )}
        </div>
      )}

      <Button type="submit" className="w-full" disabled={login.isPending}>
        {login.isPending ? (
          <span className="flex items-center gap-2">
            <span className="h-4 w-4 border-2 border-surface-base border-t-transparent rounded-full animate-spin" />
            Signing in…
          </span>
        ) : (
          'Sign In'
        )}
      </Button>

      <p className="text-center text-sm text-zinc-500">
        No account?{' '}
        <Link to="/register" className="text-accent hover:text-accent-dim">
          Create one →
        </Link>
      </p>
    </form>
  )
}

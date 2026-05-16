import { useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useResetPassword } from '@/features/auth/useAuth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import type { AxiosError } from 'axios'
import type { ApiError } from '@/types/api'

export function ResetPasswordPage() {
  const [searchParams] = useSearchParams()
  const token = searchParams.get('token') ?? ''
  const email = searchParams.get('email') ?? ''
  const [password, setPassword] = useState('')
  const reset = useResetPassword()

  const apiError = reset.error
    ? (reset.error as AxiosError<ApiError>).response?.data?.error
    : null

  const isInvalidToken = apiError?.code === 'INVALID_RESET_TOKEN'

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    reset.mutate({ email, token, password })
  }

  if (!token || !email) {
    return (
      <div className="min-h-screen bg-surface-base flex items-center justify-center px-4">
        <div className="w-full max-w-sm text-center space-y-4">
          <p className="text-fg-muted text-sm">This reset link is invalid or has expired.</p>
          <Link to="/forgot-password" className="text-accent hover:text-accent-dim text-sm">
            Request a new link →
          </Link>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-surface-base flex items-center justify-center px-4">
      <div className="w-full max-w-sm">
        <div className="text-center mb-8">
          <h1 className="font-display text-4xl text-fg-base">SHOP</h1>
        </div>
        <div className="bg-surface-raised border border-surface-border rounded-lg p-8">
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-1">
              <h2 className="text-lg font-display text-fg-base">Set a new password</h2>
              <p className="text-sm text-fg-muted">Resetting password for {email}</p>
            </div>

            <div className="space-y-1">
              <label className="text-sm text-fg-muted">New Password</label>
              <Input
                type="password"
                placeholder="••••••••••••"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                autoComplete="new-password"
                minLength={8}
                maxLength={72}
              />
              <p className="text-xs text-fg-subtle">Min 8 chars · max 72 chars</p>
            </div>

            {apiError && (
              <p className="text-sm text-status-failed">
                {isInvalidToken
                  ? 'This reset link is invalid or has expired. Request a new one.'
                  : apiError.code === 'TOO_MANY_REQUESTS'
                    ? 'Too many attempts. Please request a new reset link.'
                    : (apiError.message ?? 'Something went wrong. Please try again.')}
              </p>
            )}

            {isInvalidToken && (
              <Link
                to="/forgot-password"
                className="inline-block text-sm text-accent hover:text-accent-dim"
              >
                Request a new link →
              </Link>
            )}

            <Button type="submit" className="w-full" disabled={reset.isPending}>
              {reset.isPending ? (
                <span className="flex items-center gap-2">
                  <span className="h-4 w-4 border-2 border-surface-base border-t-transparent rounded-full animate-spin" />
                  Resetting…
                </span>
              ) : (
                'Reset Password'
              )}
            </Button>

            <p className="text-center text-sm text-fg-subtle">
              <Link to="/login" className="text-fg-muted hover:text-fg-base">
                ← Back to Sign In
              </Link>
            </p>
          </form>
        </div>
      </div>
    </div>
  )
}

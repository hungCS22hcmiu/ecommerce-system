import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useForgotPassword } from '@/features/auth/useAuth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import type { AxiosError } from 'axios'
import type { ApiError } from '@/types/api'

export function ForgotPasswordPage() {
  const [email, setEmail] = useState('')
  const [sent, setSent] = useState(false)
  const forgot = useForgotPassword()

  const apiError = forgot.error
    ? (forgot.error as AxiosError<ApiError>).response?.data?.error
    : null

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    forgot.mutate({ email }, { onSuccess: () => setSent(true) })
  }

  return (
    <div className="min-h-screen bg-surface-base flex items-center justify-center px-4">
      <div className="w-full max-w-sm">
        <div className="text-center mb-8">
          <h1 className="font-display text-4xl text-fg-base">SHOP</h1>
        </div>
        <div className="bg-surface-raised border border-surface-border rounded-lg p-8">
          {sent ? (
            <div className="space-y-4 text-center">
              <p className="text-fg-base text-sm">
                If that email is registered, a reset link has been sent. Check your inbox.
              </p>
              <Link to="/login" className="block text-sm text-accent hover:text-accent-dim">
                ← Back to Sign In
              </Link>
            </div>
          ) : (
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="space-y-1">
                <h2 className="text-lg font-display text-fg-base">Reset your password</h2>
                <p className="text-sm text-fg-muted">
                  Enter your email and we'll send you a reset link.
                </p>
              </div>

              <div className="space-y-1">
                <label className="text-sm text-fg-muted">Email</label>
                <Input
                  type="email"
                  placeholder="you@example.com"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  required
                  autoComplete="email"
                />
              </div>

              {apiError && (
                <p className="text-sm text-status-failed">
                  {apiError.code === 'TOO_MANY_REQUESTS'
                    ? 'Please wait a minute before requesting another link.'
                    : (apiError.message ?? 'Something went wrong. Please try again.')}
                </p>
              )}

              <Button type="submit" className="w-full" disabled={forgot.isPending}>
                {forgot.isPending ? (
                  <span className="flex items-center gap-2">
                    <span className="h-4 w-4 border-2 border-surface-base border-t-transparent rounded-full animate-spin" />
                    Sending…
                  </span>
                ) : (
                  'Send Reset Link'
                )}
              </Button>

              <p className="text-center text-sm text-fg-subtle">
                <Link to="/login" className="text-fg-muted hover:text-fg-base">
                  ← Back to Sign In
                </Link>
              </p>
            </form>
          )}
        </div>
      </div>
    </div>
  )
}

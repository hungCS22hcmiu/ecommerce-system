import { useRef, useState, useEffect, type KeyboardEvent, type ClipboardEvent } from 'react'
import { useSearchParams, Link } from 'react-router-dom'
import { useVerifyEmail, useResendVerification } from '@/features/auth/useAuth'
import { Button } from '@/components/ui/button'
import type { AxiosError } from 'axios'
import type { ApiError } from '@/types/api'

export function VerifyEmailPage() {
  const [searchParams] = useSearchParams()
  const email = searchParams.get('email') ?? ''
  const [digits, setDigits] = useState(['', '', '', '', '', ''])
  const inputRefs = useRef<(HTMLInputElement | null)[]>([])
  const [cooldown, setCooldown] = useState(0)

  const verify = useVerifyEmail()
  const resend = useResendVerification()

  useEffect(() => {
    inputRefs.current[0]?.focus()
  }, [])

  useEffect(() => {
    if (cooldown <= 0) return
    const t = setTimeout(() => setCooldown((c) => c - 1), 1000)
    return () => clearTimeout(t)
  }, [cooldown])

  const code = digits.join('')

  const handleChange = (i: number, value: string) => {
    const char = value.replace(/\D/g, '').slice(-1)
    const next = [...digits]
    next[i] = char
    setDigits(next)
    if (char && i < 5) inputRefs.current[i + 1]?.focus()
  }

  const handleKeyDown = (i: number, e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Backspace' && !digits[i] && i > 0) {
      inputRefs.current[i - 1]?.focus()
    }
  }

  const handlePaste = (e: ClipboardEvent<HTMLInputElement>) => {
    e.preventDefault()
    const pasted = e.clipboardData.getData('text').replace(/\D/g, '').slice(0, 6)
    if (!pasted) return
    const next = [...digits]
    pasted.split('').forEach((c, i) => { next[i] = c })
    setDigits(next)
    const focusIdx = Math.min(pasted.length, 5)
    inputRefs.current[focusIdx]?.focus()
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (code.length < 6) return
    verify.mutate({ email, code })
  }

  const handleResend = () => {
    resend.mutate({ email })
    setCooldown(60)
  }

  const verifyError = verify.error
    ? (verify.error as AxiosError<ApiError>).response?.data?.error
    : null

  return (
    <div className="min-h-screen bg-surface-base flex items-center justify-center px-4">
      <div className="w-full max-w-sm">
        <div className="text-center mb-8">
          <h1 className="font-display text-4xl text-fg-base">SHOP</h1>
        </div>

        <div className="bg-surface-raised border border-surface-border rounded-lg p-8">
          <h2 className="text-lg font-semibold text-fg-base mb-1">Verify your email</h2>
          <p className="text-sm text-fg-subtle mb-6">
            Enter the 6-digit code sent to{' '}
            <span className="text-fg-muted font-mono">{email}</span>
          </p>

          <form onSubmit={handleSubmit} className="space-y-6">
            <div className="flex gap-2 justify-between">
              {digits.map((d, i) => (
                <input
                  key={i}
                  ref={(el) => { inputRefs.current[i] = el }}
                  type="text"
                  inputMode="numeric"
                  maxLength={1}
                  value={d}
                  onChange={(e) => handleChange(i, e.target.value)}
                  onKeyDown={(e) => handleKeyDown(i, e)}
                  onPaste={handlePaste}
                  className="w-10 h-12 text-center text-lg font-mono text-fg-base bg-surface-overlay border border-surface-border rounded-md focus:outline-none focus:ring-1 focus:ring-accent caret-accent"
                />
              ))}
            </div>

            {verifyError && (
              <p className="text-sm text-status-failed">
                {verifyError.code === 'INVALID_CODE'
                  ? 'Invalid or expired code. Please try again.'
                  : verifyError.code === 'TOO_MANY_ATTEMPTS'
                  ? 'Too many attempts. Request a new code.'
                  : verifyError.message}
              </p>
            )}

            <Button
              type="submit"
              className="w-full"
              disabled={code.length < 6 || verify.isPending}
            >
              {verify.isPending ? (
                <span className="flex items-center gap-2">
                  <span className="h-4 w-4 border-2 border-surface-base border-t-transparent rounded-full animate-spin" />
                  Verifying…
                </span>
              ) : (
                'Verify Email'
              )}
            </Button>
          </form>

          <div className="mt-4 text-center">
            {resend.isSuccess ? (
              <p className="text-sm text-status-delivered">Code resent!</p>
            ) : (
              <button
                type="button"
                onClick={handleResend}
                disabled={cooldown > 0 || resend.isPending}
                className="text-sm text-fg-subtle hover:text-fg-base disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
              >
                {cooldown > 0 ? `Resend code in ${cooldown}s` : "Didn't receive a code? Resend"}
              </button>
            )}
          </div>

          <p className="mt-4 text-center text-sm text-fg-subtle">
            <Link to="/login" className="text-fg-subtle hover:text-fg-base">
              ← Back to login
            </Link>
          </p>
        </div>
      </div>
    </div>
  )
}

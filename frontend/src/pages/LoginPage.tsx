import { LoginForm } from '@/features/auth/LoginForm'

export function LoginPage() {
  return (
    <div className="min-h-screen bg-surface-base flex items-center justify-center px-4">
      <div className="w-full max-w-sm">
        <div className="text-center mb-8">
          <h1 className="font-display text-4xl text-white">SHOP</h1>
        </div>
        <div className="bg-surface-raised border border-surface-border rounded-lg p-8">
          <LoginForm />
        </div>
      </div>
    </div>
  )
}

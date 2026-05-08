import { RegisterForm } from '@/features/auth/RegisterForm'

export function RegisterPage() {
  return (
    <div className="min-h-screen bg-surface-base flex items-center justify-center px-4">
      <div className="w-full max-w-sm">
        <div className="text-center mb-8">
          <h1 className="font-display text-4xl text-fg-base">SHOP</h1>
        </div>
        <div className="bg-surface-raised border border-surface-border rounded-lg p-8">
          <h2 className="text-lg font-semibold text-fg-base mb-6">Create an account</h2>
          <RegisterForm />
        </div>
      </div>
    </div>
  )
}

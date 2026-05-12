import { Navigate, Outlet } from 'react-router-dom'
import { useAuthStore } from '@/store/authStore'

export function SellerRoute() {
  const role = useAuthStore((s) => s.role)

  if (role !== 'seller') {
    return <Navigate to="/" replace />
  }

  return <Outlet />
}

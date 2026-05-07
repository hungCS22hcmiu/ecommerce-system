import { Link, NavLink } from 'react-router-dom'
import { useAuthStore } from '@/store/authStore'
import { useLogout } from '@/features/auth/useAuth'
import { Button } from '@/components/ui/button'

export function Navbar() {
  const email = useAuthStore((s) => s.email)
  const accessToken = useAuthStore((s) => s.accessToken)
  const logout = useLogout()
  const isLoggedIn = !!accessToken

  return (
    <nav className="bg-surface-base border-b border-surface-border">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="flex items-center justify-between h-14">
          <div className="flex items-center gap-8">
            <Link
              to="/"
              className="font-display text-xl text-white tracking-wide"
            >
              SHOP
            </Link>
            {isLoggedIn && (
              <div className="flex items-center gap-6">
                <NavLink
                  to="/products"
                  className={({ isActive }) =>
                    `text-sm transition-colors ${isActive ? 'text-white' : 'text-zinc-400 hover:text-white'}`
                  }
                >
                  Products
                </NavLink>
                <NavLink
                  to="/orders"
                  className={({ isActive }) =>
                    `text-sm transition-colors ${isActive ? 'text-white' : 'text-zinc-400 hover:text-white'}`
                  }
                >
                  Orders
                </NavLink>
              </div>
            )}
          </div>

          <div className="flex items-center gap-3">
            {isLoggedIn ? (
              <>
                <span className="text-xs text-zinc-500 font-mono hidden sm:block">
                  {email}
                </span>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => logout.mutate()}
                  disabled={logout.isPending}
                >
                  Logout
                </Button>
              </>
            ) : (
              <>
                <Link to="/login">
                  <Button variant="ghost" size="sm">Sign In</Button>
                </Link>
                <Link to="/register">
                  <Button size="sm">Register</Button>
                </Link>
              </>
            )}
          </div>
        </div>
      </div>
    </nav>
  )
}

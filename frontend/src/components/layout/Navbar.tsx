import { useState } from 'react'
import { Link, NavLink } from 'react-router-dom'
import { useAuthStore } from '@/store/authStore'
import { useCartStore } from '@/store/cartStore'
import { useLogout } from '@/features/auth/useAuth'
import { CartDrawer } from '@/features/cart/CartDrawer'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'

export function Navbar() {
  const email = useAuthStore((s) => s.email)
  const accessToken = useAuthStore((s) => s.accessToken)
  const itemCount = useCartStore((s) => s.itemCount)
  const logout = useLogout()
  const isLoggedIn = !!accessToken
  const [cartOpen, setCartOpen] = useState(false)

  return (
    <>
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
                  <NavLink
                    to="/profile"
                    className={({ isActive }) =>
                      `text-sm transition-colors ${isActive ? 'text-white' : 'text-zinc-400 hover:text-white'}`
                    }
                  >
                    Profile
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

                  {/* Cart button */}
                  <button
                    type="button"
                    onClick={() => setCartOpen(true)}
                    className="relative flex items-center gap-1.5 text-zinc-400 hover:text-white transition-colors px-2 py-1"
                  >
                    <svg
                      xmlns="http://www.w3.org/2000/svg"
                      className="h-5 w-5"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={1.5}
                        d="M3 3h2l.4 2M7 13h10l4-8H5.4M7 13L5.4 5M7 13l-2.293 2.293c-.63.63-.184 1.707.707 1.707H17m0 0a2 2 0 100 4 2 2 0 000-4zm-8 2a2 2 0 11-4 0 2 2 0 014 0z"
                      />
                    </svg>
                    {itemCount > 0 && (
                      <Badge variant="amber" className="text-xs px-1.5 py-0 min-w-[1.25rem] h-5 flex items-center justify-center">
                        {itemCount}
                      </Badge>
                    )}
                  </button>

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

      {isLoggedIn && (
        <CartDrawer open={cartOpen} onClose={() => setCartOpen(false)} />
      )}
    </>
  )
}

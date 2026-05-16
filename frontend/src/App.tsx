import { BrowserRouter, Routes, Route, Navigate, Outlet } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { ReactQueryDevtools } from '@tanstack/react-query-devtools'
import { queryClient } from '@/lib/queryClient'
import { ProtectedRoute } from '@/features/auth/ProtectedRoute'
import { SellerRoute } from '@/features/seller/SellerRoute'
import { Navbar } from '@/components/layout/Navbar'
import { LoginPage } from '@/pages/LoginPage'
import { RegisterPage } from '@/pages/RegisterPage'
import { VerifyEmailPage } from '@/pages/VerifyEmailPage'
import { ProductListPage } from '@/pages/ProductListPage'
import { ProductDetailPage } from '@/pages/ProductDetailPage'
import { CartPage } from '@/pages/CartPage'
import { CheckoutPage } from '@/pages/CheckoutPage'
import { OrderConfirmationPage } from '@/pages/OrderConfirmationPage'
import { OrderHistoryPage } from '@/pages/OrderHistoryPage'
import { OrderDetailPage } from '@/pages/OrderDetailPage'
import { ProfilePage } from '@/pages/ProfilePage'
import { SellerDashboardPage } from '@/pages/SellerDashboardPage'
import { SellerCreateProductPage } from '@/pages/SellerCreateProductPage'
import { SellerEditProductPage } from '@/pages/SellerEditProductPage'
import { ToastContainer } from '@/components/ui/Toast'

function Layout() {
  return (
    <div className="min-h-screen bg-surface-base">
      <Navbar />
      <main>
        <Outlet />
      </main>
    </div>
  )
}

export default function App() {
  return (
    <BrowserRouter>
      <QueryClientProvider client={queryClient}>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/register" element={<RegisterPage />} />
          <Route path="/verify-email" element={<VerifyEmailPage />} />
          <Route element={<Layout />}>
            <Route path="/" element={<Navigate to="/products" replace />} />
            <Route path="/products" element={<ProductListPage />} />
            <Route path="/products/:id" element={<ProductDetailPage />} />
          </Route>
          <Route element={<ProtectedRoute />}>
            <Route element={<Layout />}>
              <Route path="/cart" element={<CartPage />} />
              <Route path="/checkout" element={<CheckoutPage />} />
              <Route path="/orders/:id/confirmation" element={<OrderConfirmationPage />} />
              <Route path="/orders" element={<OrderHistoryPage />} />
              <Route path="/orders/:id" element={<OrderDetailPage />} />
              <Route path="/profile" element={<ProfilePage />} />
              <Route element={<SellerRoute />}>
                <Route path="/seller/products" element={<SellerDashboardPage />} />
                <Route path="/seller/products/new" element={<SellerCreateProductPage />} />
                <Route path="/seller/products/:id/edit" element={<SellerEditProductPage />} />
              </Route>
            </Route>
          </Route>
          <Route path="*" element={<Navigate to="/products" replace />} />
        </Routes>
        <ToastContainer />
        <ReactQueryDevtools initialIsOpen={false} />
      </QueryClientProvider>
    </BrowserRouter>
  )
}

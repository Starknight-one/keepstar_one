import { Routes, Route, Navigate } from 'react-router-dom'
import { useAuth } from './features/auth/AuthProvider.jsx'
import LoginPage from './features/auth/LoginPage.jsx'
import SignupPage from './features/auth/SignupPage.jsx'
import DashboardLayout from './features/layout/DashboardLayout.jsx'
import ProductsPage from './features/catalog/ProductsPage.jsx'
import ProductDetailPage from './features/catalog/ProductDetailPage.jsx'
import ImportPage from './features/import/ImportPage.jsx'
import SettingsPage from './features/settings/SettingsPage.jsx'

function ProtectedRoute({ children }) {
  const { user, loading } = useAuth()
  if (loading) return null
  if (!user) return <Navigate to="/login" replace />
  return children
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/signup" element={<SignupPage />} />
      <Route
        path="/"
        element={
          <ProtectedRoute>
            <DashboardLayout />
          </ProtectedRoute>
        }
      >
        <Route index element={<Navigate to="/catalog" replace />} />
        <Route path="catalog" element={<ProductsPage />} />
        <Route path="catalog/:id" element={<ProductDetailPage />} />
        <Route path="import" element={<ImportPage />} />
        <Route path="settings" element={<SettingsPage />} />
      </Route>
    </Routes>
  )
}

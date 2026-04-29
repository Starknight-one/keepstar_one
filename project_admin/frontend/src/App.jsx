import { Routes, Route, Navigate } from 'react-router-dom'
import { useAuth } from './features/auth/AuthProvider.jsx'
import SignInPage from './features/auth/pages/SignInPage.jsx'
import SignUpPage from './features/auth/pages/SignUpPage.jsx'
import ForgotPasswordPage from './features/auth/pages/ForgotPasswordPage.jsx'
import CheckEmailPage from './features/auth/pages/CheckEmailPage.jsx'
import ResetPasswordPage from './features/auth/pages/ResetPasswordPage.jsx'
import PasswordChangedPage from './features/auth/pages/PasswordChangedPage.jsx'
import VerifyEmailPage from './features/auth/pages/VerifyEmailPage.jsx'
import SessionExpiredPage from './features/auth/pages/SessionExpiredPage.jsx'
import OAuthLoadingPage from './features/auth/pages/OAuthLoadingPage.jsx'
import MagicLinkPage from './features/auth/pages/MagicLinkPage.jsx'
import InstallCompletePage from './features/auth/pages/InstallCompletePage.jsx'
import AuthErrorPage from './features/auth/pages/AuthErrorPage.jsx'
import TwoFactorPage from './features/auth/pages/TwoFactorPage.jsx'
import WorkspacePickerPage from './features/auth/pages/WorkspacePickerPage.jsx'
import InviteAcceptPage from './features/auth/pages/InviteAcceptPage.jsx'
import DashboardLayout from './features/layout/DashboardLayout.jsx'
import ProductsPage from './features/catalog/ProductsPage.jsx'
import ProductDetailPage from './features/catalog/ProductDetailPage.jsx'
import CategoryEditor from './features/catalog/CategoryEditor.jsx'
import DetectedAddonsPage from './features/catalog/DetectedAddonsPage.jsx'
import ImportPage from './features/import/ImportPage.jsx'
import IntegrationsPage from './features/integrations/IntegrationsPage.jsx'
import CSVUploadPage from './features/integrations/CSVUploadPage.jsx'
import ShopifyConnectPage from './features/integrations/ShopifyConnectPage.jsx'
import SettingsPage from './features/settings/SettingsPage.jsx'
import ApiKeysPage from './features/settings/ApiKeysPage.jsx'
import CanvasPage from './features/canvas/CanvasPage.jsx'
import WidgetPage from './features/widget/WidgetPage.jsx'
import ConversationsPage from './features/conversations/ConversationsPage.jsx'
import ConversationDetailPage from './features/conversations/ConversationDetailPage.jsx'
import TracesPage from './features/traces/TracesPage.jsx'
import TraceDetail from './features/traces/TraceDetail.jsx'
import SessionDetail from './features/traces/SessionDetail.jsx'
import BillingPage from './features/billing/BillingPage.jsx'

function ProtectedRoute({ children }) {
  const { user, loading } = useAuth()
  if (loading) return null
  if (!user) return <Navigate to="/auth/sign-in" replace />
  return children
}

export default function App() {
  return (
    <Routes>
      {/* New split-layout auth stack. */}
      <Route path="/auth/sign-in" element={<SignInPage />} />
      <Route path="/auth/sign-up" element={<SignUpPage />} />
      <Route path="/auth/forgot-password" element={<ForgotPasswordPage />} />
      <Route path="/auth/check-email" element={<CheckEmailPage />} />
      <Route path="/auth/reset-password" element={<ResetPasswordPage />} />
      <Route path="/auth/password-changed" element={<PasswordChangedPage />} />
      <Route path="/auth/verify-email" element={<VerifyEmailPage />} />
      <Route path="/auth/session-expired" element={<SessionExpiredPage />} />
      <Route path="/auth/google/callback" element={<OAuthLoadingPage />} />
      <Route path="/auth/magic" element={<MagicLinkPage />} />
      <Route path="/auth/install-complete" element={<InstallCompletePage />} />
      <Route path="/auth/error" element={<AuthErrorPage />} />
      <Route path="/auth/2fa" element={<TwoFactorPage />} />
      <Route path="/auth/pick-workspace" element={<ProtectedRoute><WorkspacePickerPage /></ProtectedRoute>} />
      <Route path="/auth/accept-invite" element={<InviteAcceptPage />} />

      {/* Legacy → new. Keep for one release to not break bookmarks. */}
      <Route path="/login" element={<Navigate to="/auth/sign-in" replace />} />
      <Route path="/signup" element={<Navigate to="/auth/sign-up" replace />} />
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
        <Route path="catalog/categories" element={<CategoryEditor />} />
        <Route path="catalog/detected-addons" element={<DetectedAddonsPage />} />
        <Route path="catalog/:id" element={<ProductDetailPage />} />
        <Route path="import" element={<ImportPage />} />
        <Route path="integrations" element={<IntegrationsPage />} />
        <Route path="integrations/csv" element={<CSVUploadPage />} />
        <Route path="integrations/shopify" element={<ShopifyConnectPage />} />
        <Route path="widget" element={<WidgetPage />} />
        <Route path="conversations" element={<ConversationsPage />} />
        <Route path="conversations/:sessionId" element={<ConversationDetailPage />} />
        <Route path="traces" element={<TracesPage />} />
        <Route path="traces/session/:sessionId" element={<SessionDetail />} />
        <Route path="traces/:id" element={<TraceDetail />} />
        <Route path="canvas" element={<CanvasPage />} />
        <Route path="settings" element={<SettingsPage />} />
        <Route path="settings/api-keys" element={<ApiKeysPage />} />
        <Route path="billing" element={<BillingPage />} />
      </Route>
    </Routes>
  )
}

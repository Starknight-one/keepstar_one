import { api } from '../../../shared/api/apiClient.js'

export const authApi = {
  config: () => api.get('/auth/config'),
  login: (email, password) => api.post('/auth/login', { email, password }),
  signup: (email, password, companyName) =>
    api.post('/auth/signup', { email, password, companyName }),
  me: () => api.get('/auth/me'),
  googleStart: () => api.post('/auth/google/start', {}),
  googleCallback: (code, state) => api.post('/auth/google/callback', { code, state }),
  telegramCallback: (payload) => api.post('/auth/telegram/callback', payload),
  telegramStart: () => api.post('/auth/telegram/start', {}),
  telegramOIDCCallback: (code, state) => api.post('/auth/telegram/oidc/callback', { code, state }),
  magicConsume: (code) => api.post('/auth/magic', { code }),

  // Pending-shop-link: Shopify install completed without an owner email, so
  // no magic-link was sent. Frontend persists the token in sessionStorage at
  // /auth/install-complete and consumes it on the next workspace-picker
  // landing (after the user signs in via standard methods). On success the
  // backend attaches the orphan Shopify tenant to the now-authenticated user.
  consumePendingShopLink: (token) =>
    api.post('/auth/shop-pending-link/consume', { token }),

  // 2FA — pre-session routes accept a pre-2FA bearer token (from login).
  setupTOTP: () => api.post('/auth/2fa/setup/totp', {}),
  confirmTOTP: (code) => api.post('/auth/2fa/confirm/totp', { code }),
  disableTOTP: () => api.post('/auth/2fa/disable/totp', {}),
  verifyTOTP: (code, pre2faToken) =>
    api.post('/auth/2fa/verify/totp', { code }, { headers: { Authorization: `Bearer ${pre2faToken}` } }),
  sendEmail2FA: (pre2faToken) =>
    api.post('/auth/2fa/send/email', {}, { headers: { Authorization: `Bearer ${pre2faToken}` } }),
  verifyEmail2FA: (code, pre2faToken) =>
    api.post('/auth/2fa/verify/email', { code }, { headers: { Authorization: `Bearer ${pre2faToken}` } }),

  // Workspaces — list all tenants the current user is a member of, and
  // switch the active tenant (re-issues a session with tid = chosen).
  listTenants: () => api.get('/auth/tenants'),
  selectTenant: (tenantID) => api.post('/auth/tenants/select', { tenant_id: tenantID }),

  // Active sessions (refresh-token families) for Settings → Sessions.
  // Each row carries parsed device + geo + a current_session marker so the UI
  // can highlight "this device" and let the user kill stale sessions from
  // other browsers / devices.
  listSessions: () => api.get('/auth/sessions'),
  revokeSession: (id) => api.del(`/auth/sessions/${encodeURIComponent(id)}`),

  // Invitations. Create runs protected (current user is the inviter).
  // Preview + accept are public — the token in the URL is the credential.
  createInvite: (email, role) => api.post('/auth/invitations', { email, role }),
  previewInvite: (token) => api.get(`/auth/invitations/${encodeURIComponent(token)}`),
  acceptInvite: (token, password) =>
    api.post(`/auth/invitations/${encodeURIComponent(token)}/accept`, password ? { password } : {}),
}

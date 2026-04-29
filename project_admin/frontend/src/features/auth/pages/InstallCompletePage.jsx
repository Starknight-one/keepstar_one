import React from 'react'
import { useSearchParams } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'

// InstallCompletePage is the landing for the Shopify App Store install flow.
// At this point we've already provisioned a tenant + admin_user from the shop
// owner email and emailed a magic-link. The merchant has no password to type
// and no other identity (Google/Telegram) — putting them on the generic
// sign-in page is confusing. This page tells them what to do next: open
// their inbox and click the link.
export default function InstallCompletePage() {
  const [params] = useSearchParams()
  const shop = params.get('shop') || ''

  return (
    <AuthShell>
      <h1>Almost there</h1>
      <p>
        Your Shopify store {shop ? <strong>{shop}</strong> : ''} is connected to Keepstar One.
      </p>
      <p>
        We've emailed a sign-in link to your shop owner address. Open your inbox and click
        the link to finish setting up your workspace. The link expires in 24 hours.
      </p>
      <p style={{ fontSize: 14, color: '#5F6368', marginTop: 24 }}>
        Didn't get the email? Check your spam folder. If it's still missing, ping support
        and we'll resend it manually.
      </p>
    </AuthShell>
  )
}

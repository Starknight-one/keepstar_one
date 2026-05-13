import React, { useEffect } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'
import PillButton from '../layout/PillButton.jsx'

// Key under which we stash the pending-shop-link token between the install
// landing page and whatever auth method the merchant picks. Picked up by
// WorkspacePickerPage on the post-auth landing and POSTed to
// /auth/shop-pending-link/consume to attach the orphan Shopify tenant.
const PENDING_SHOP_LINK_KEY = 'keepstar.pending_shop_link'

// InstallCompletePage is the landing for the Shopify App Store install flow.
// Two shapes:
//
//   1. Happy path (?shop=…) — the backend already provisioned a tenant +
//      admin_user from the shop owner email and emailed a magic-link. The
//      merchant just needs to open their inbox.
//
//   2. Pending-link path (?pending_link=TOKEN&shop=…) — the shop returned no
//      owner email, so no magic-link could be sent. We stash the token in
//      sessionStorage and ask the merchant to sign in via standard methods;
//      after auth, WorkspacePickerPage consumes the token and attaches the
//      orphan tenant.
export default function InstallCompletePage() {
  const [params] = useSearchParams()
  const shop = params.get('shop') || ''
  const pendingLink = params.get('pending_link') || ''

  useEffect(() => {
    if (pendingLink) {
      sessionStorage.setItem(PENDING_SHOP_LINK_KEY, pendingLink)
    }
  }, [pendingLink])

  if (pendingLink) {
    return (
      <AuthShell>
        <h1>One more step</h1>
        <p>
          Your Shopify store {shop ? <strong>{shop}</strong> : ''} is connected — but we
          couldn't read its owner email, so we can't email you a sign-in link.
        </p>
        <p>
          Sign in with any of the methods below and we'll link the store to your account
          automatically.
        </p>

        <div style={{ display: 'flex', flexDirection: 'column', gap: 10, marginTop: 24 }}>
          <Link to="/auth/sign-in" style={{ textDecoration: 'none' }}>
            <PillButton variant="primary" block>Sign in to Keepstar</PillButton>
          </Link>
          <Link to="/auth/sign-up" style={{ textDecoration: 'none' }}>
            <PillButton variant="secondary" block>Create a new account</PillButton>
          </Link>
        </div>

        <p style={{ fontSize: 13, color: '#5F6368', marginTop: 24 }}>
          The Sign in / Sign up pages also offer Google and Telegram. Whichever method
          you pick, this Shopify store will be attached to that Keepstar account.
        </p>
      </AuthShell>
    )
  }

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

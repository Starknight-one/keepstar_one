import { useState } from 'react'
import Button from '../../shared/ui/Button.jsx'
import './integrations.css'

export default function ShopifyConnectPage() {
  const [shop, setShop] = useState('')
  const [error, setError] = useState('')

  function handleConnect(e) {
    e.preventDefault()
    setError('')
    const normalized = shop.trim().toLowerCase()
    if (!/^[a-z0-9][a-z0-9-]*\.myshopify\.com$/.test(normalized)) {
      setError('Shop must look like your-store.myshopify.com')
      return
    }
    // Full-page redirect — the install route is authenticated, so the
    // current session cookie/bearer ships along with the request.
    window.location.href = `/admin/api/integrations/shopify/install?shop=${encodeURIComponent(normalized)}`
  }

  return (
    <div>
      <h1 className="page-title">Connect Shopify</h1>
      <p className="text-secondary">Enter your shop domain — we&apos;ll redirect to Shopify for install approval.</p>

      <form className="shopify-connect-panel" onSubmit={handleConnect} style={{ marginTop: 24 }}>
        <label style={{ display: 'block', fontSize: 13, fontWeight: 500, marginBottom: 8 }}>Shop domain</label>
        <input
          type="text"
          value={shop}
          onChange={(e) => setShop(e.target.value)}
          placeholder="your-store.myshopify.com"
          autoFocus
        />
        {error && <div className="auth-error" style={{ marginBottom: 16 }}>{error}</div>}
        <div style={{ display: 'flex', gap: 12 }}>
          <Button type="submit">Install Keepstar</Button>
        </div>
        <p className="text-secondary" style={{ fontSize: 12, marginTop: 16 }}>
          We request access to products, listings, inventory, locations, metaobjects, and orders. Scopes match the Shopify App configuration on our side.
        </p>
      </form>
    </div>
  )
}

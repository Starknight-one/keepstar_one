import { useState, useEffect } from 'react'
import { api } from '../../shared/api/apiClient.js'
import Spinner from '../../shared/ui/Spinner.jsx'
import { Copy, Check, Code } from 'lucide-react'
import './widget.css'

export default function WidgetPage() {
  const [tenant, setTenant] = useState(null)
  const [widgetUrl, setWidgetUrl] = useState('')
  const [chatApiUrl, setChatApiUrl] = useState('')
  const [loading, setLoading] = useState(true)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    Promise.all([
      api.get('/tenant'),
      api.get('/widget-config'),
    ])
      .then(([t, wc]) => {
        setTenant(t)
        setWidgetUrl(wc.widgetUrl || '')
        setChatApiUrl(wc.chatApiUrl || '')
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  if (loading) return <div className="center-spinner"><Spinner /></div>

  const slug = tenant?.slug || 'your-tenant'
  const scriptSrc = widgetUrl ? `${widgetUrl}/widget.js` : 'https://YOUR_CHAT_SERVER/widget.js'
  const apiUrl = chatApiUrl || 'https://YOUR_CHAT_SERVER/api/v1'
  const embedCode = `<script>\nvar s=document.createElement('script');\ns.src='${scriptSrc}';\ns.setAttribute('data-tenant','${slug}');\ns.setAttribute('data-api','${apiUrl}');\ndocument.body.appendChild(s);\n</script>`

  function handleCopy() {
    navigator.clipboard.writeText(embedCode).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  return (
    <div>
      <h1 className="page-title">Widget</h1>
      <p className="widget-subtitle">
        Embed the AI chat widget on your website with a single line of code.
      </p>

      <div className="widget-section">
        <h2 className="widget-section-title">
          <Code size={18} /> Embed Code
        </h2>
        <p className="widget-hint">
          Paste this snippet before the closing <code>&lt;/body&gt;</code> tag on any page.
        </p>
        <div className="widget-code-block">
          <pre><code>{embedCode}</code></pre>
          <button className="widget-copy-btn" onClick={handleCopy}>
            {copied ? <><Check size={14} /> Copied</> : <><Copy size={14} /> Copy</>}
          </button>
        </div>
      </div>

      <div className="widget-section">
        <h2 className="widget-section-title">Your Tenant</h2>
        <div className="widget-info-row">
          <span className="widget-info-label">Slug</span>
          <code className="widget-info-value">{tenant?.slug || '—'}</code>
        </div>
        <div className="widget-info-row">
          <span className="widget-info-label">Name</span>
          <span className="widget-info-value">{tenant?.name || '—'}</span>
        </div>
      </div>

      <div className="widget-section">
        <h2 className="widget-section-title">How It Works</h2>
        <ol className="widget-steps">
          <li>Copy the embed code above</li>
          <li>Paste it into your website's HTML</li>
          <li>A chat button appears in the bottom-right corner</li>
          <li>Your customers click it to chat with AI about your products</li>
        </ol>
      </div>
    </div>
  )
}

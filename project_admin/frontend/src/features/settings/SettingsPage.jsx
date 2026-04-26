import { useState, useEffect } from 'react'
import { api } from '../../shared/api/apiClient.js'
import Input from '../../shared/ui/Input.jsx'
import Button from '../../shared/ui/Button.jsx'
import Spinner from '../../shared/ui/Spinner.jsx'
import './settings.css'

const COUNTRIES = [
  { code: '', label: 'Select country' },
  { code: 'RU', label: 'Russia' },
  { code: 'KZ', label: 'Kazakhstan' },
  { code: 'BY', label: 'Belarus' },
  { code: 'UZ', label: 'Uzbekistan' },
  { code: 'US', label: 'United States' },
  { code: 'GB', label: 'United Kingdom' },
  { code: 'DE', label: 'Germany' },
  { code: 'FR', label: 'France' },
]

const CURRENCIES = [
  'USD', 'EUR', 'GBP', 'CAD', 'AUD', 'JPY', 'CHF',
  'SEK', 'NOK', 'DKK', 'PLN', 'MXN', 'BRL',
]

export default function SettingsPage() {
  const [settings, setSettings] = useState(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState('')

  useEffect(() => {
    api.get('/settings')
      .then(setSettings)
      .catch(() => setSettings({}))
      .finally(() => setLoading(false))
  }, [])

  async function handleSave(e) {
    e.preventDefault()
    setSaving(true)
    setMessage('')
    try {
      await api.put('/settings', settings)
      setMessage('Settings saved')
    } catch (err) {
      setMessage(err.message)
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <div className="center-spinner"><Spinner /></div>

  return (
    <div>
      <h1 className="page-title">Settings</h1>
      <div style={{ marginBottom: 16 }}>
        <a href="/settings/api-keys" className="settings-tab-link">API access →</a>
      </div>
      <form className="settings-form" onSubmit={handleSave}>
        <div className="settings-section">
          <h2 className="settings-section-title">Geography</h2>
          <div className="settings-row">
            <div className="input-group">
              <label className="input-label">Country</label>
              <select
                className="input"
                value={settings?.geoCountry || ''}
                onChange={(e) => setSettings({ ...settings, geoCountry: e.target.value })}
              >
                {COUNTRIES.map((c) => (
                  <option key={c.code} value={c.code}>{c.label}</option>
                ))}
              </select>
            </div>
            <Input
              label="Region"
              value={settings?.geoRegion || ''}
              onChange={(e) => setSettings({ ...settings, geoRegion: e.target.value })}
              placeholder="e.g. Moscow"
            />
          </div>
        </div>

        <div className="settings-section">
          <h2 className="settings-section-title">Currency</h2>
          <div className="input-group">
            <label className="input-label">Default currency for imports</label>
            <select
              className="input"
              value={settings?.currency || 'USD'}
              onChange={(e) => setSettings({ ...settings, currency: e.target.value })}
            >
              {CURRENCIES.map((c) => (
                <option key={c} value={c}>{c}</option>
              ))}
            </select>
          </div>
        </div>

        <div className="settings-section">
          <h2 className="settings-section-title">Chat Widget</h2>
          <label className="settings-toggle">
            <input
              type="checkbox"
              checked={settings?.widgetEnabled || false}
              onChange={(e) => setSettings({ ...settings, widgetEnabled: e.target.checked })}
            />
            <span>Show chat widget on website</span>
          </label>
        </div>

        <div className="settings-section">
          <h2 className="settings-section-title">Enrichment</h2>
          <label className="settings-toggle">
            <input
              type="checkbox"
              checked={settings?.enrichCrossData || false}
              onChange={(e) => setSettings({ ...settings, enrichCrossData: e.target.checked })}
            />
            <span>Enrich products with data from other tenants</span>
          </label>
        </div>

        {message && (
          <div className={message === 'Settings saved' ? 'auth-success' : 'auth-error'}>
            {message}
          </div>
        )}

        <Button type="submit" disabled={saving}>
          {saving ? 'Saving...' : 'Save settings'}
        </Button>
      </form>
    </div>
  )
}

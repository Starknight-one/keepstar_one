import { Bell, Shield } from 'lucide-react'
import Toggle from '../../shared/ui/Toggle'

export default function PreferencesCards({ preferences, onSave }) {
  function update(partial) {
    onSave({ ...preferences, ...partial })
  }

  return (
    <section className="prefs">
      <div className="card prefs-card">
        <header className="prefs-head">
          <Bell size={16} />
          <h3>Usage Alerts</h3>
        </header>
        <PrefRow
          label={`Notify at ${preferences.alertThreshold}% token usage`}
          checked={preferences.alertThreshold >= 0 && preferences.alertThreshold <= 100 && preferences.alertThreshold !== 0}
          onChange={(v) => update({ alertThreshold: v ? 80 : 0 })}
        />
        <PrefRow
          label="Send alerts to Slack channel"
          checked={preferences.alertSlack}
          onChange={(v) => update({ alertSlack: v })}
        />
      </div>

      <div className="card prefs-card">
        <header className="prefs-head">
          <Shield size={16} />
          <h3>Overdraft Protection</h3>
        </header>
        <p className="prefs-desc">
          Keep your chat assistant running when tokens run out. Up to 20% extra usage at a premium rate.
        </p>
        <PrefRow
          label="Enable overdraft (up to 20%)"
          checked={preferences.overdraftEnabled}
          onChange={(v) => update({ overdraftEnabled: v })}
        />
        <PrefRow
          label="Notify when overdraft is triggered"
          checked={preferences.overdraftNotify}
          onChange={(v) => update({ overdraftNotify: v })}
        />
      </div>
    </section>
  )
}

function PrefRow({ label, checked, onChange }) {
  return (
    <div className="prefs-row">
      <span className="prefs-row-label">{label}</span>
      <Toggle checked={checked} onChange={onChange} />
    </div>
  )
}

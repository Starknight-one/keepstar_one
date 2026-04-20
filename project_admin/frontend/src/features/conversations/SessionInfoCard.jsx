export default function SessionInfoCard({ info }) {
  const rows = [
    ['Preset', info.preset || '—'],
    ['Layout', info.layout || '—'],
    ['Items shown', info.itemsShown ?? '—'],
    ['Ops', info.ops ?? '—'],
    ['Cart shown', info.cartShown ? 'Yes' : 'No'],
  ]
  return (
    <div className="card">
      <div className="card-title">Session info</div>
      <div className="session-info-rows">
        {rows.map(([k, v]) => (
          <div className="session-info-row" key={k}>
            <span className="session-info-key">{k}</span>
            <span className="session-info-val">{v}</span>
          </div>
        ))}
      </div>
    </div>
  )
}

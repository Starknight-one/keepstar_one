import React from 'react'

export default function AuthImagePanel({ src, alt = '' }) {
  return (
    <aside className="auth-shell__panel">
      {src ? (
        <img src={src} alt={alt} className="auth-shell__panel-image" />
      ) : (
        <div className="auth-shell__panel-fallback">Keepstar One</div>
      )}
    </aside>
  )
}

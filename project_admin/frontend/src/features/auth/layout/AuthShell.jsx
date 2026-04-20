import React from 'react'
import AuthImagePanel from './AuthImagePanel.jsx'
import './auth-v2.css'

export default function AuthShell({ imageSrc, children }) {
  return (
    <div className="auth-v2">
      <div className="auth-shell">
        <main className="auth-shell__form">
          <div className="auth-shell__form-inner">
            <div className="auth-brand">
              <div className="auth-brand__dot" />
              <span>Keepstar One</span>
            </div>
            {children}
          </div>
        </main>
        <AuthImagePanel src={imageSrc} />
      </div>
    </div>
  )
}

import React from 'react';
import './ErrorState.css';

export default function ErrorState({ title = 'Something went wrong', description = "We couldn't load the results.\nPlease try again.", onRetry }) {
  return (
    <div className="ui-error-state">
      <div className="ui-error-icon">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#EA4335" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <circle cx="12" cy="12" r="10" />
          <line x1="12" y1="8" x2="12" y2="12" />
          <line x1="12" y1="16" x2="12.01" y2="16" />
        </svg>
      </div>
      <div className="ui-error-title">{title}</div>
      <div className="ui-error-desc">{description}</div>
      {onRetry && (
        <button className="ui-error-btn" onClick={onRetry}>Retry</button>
      )}
    </div>
  );
}

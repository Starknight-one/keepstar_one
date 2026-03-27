import React from 'react';
import './StatusPill.css';

export default function StatusPill({ count, label = 'results' }) {
  return (
    <div className="ui-status-pill">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <circle cx="11" cy="11" r="8" /><path d="m21 21-4.3-4.3" />
      </svg>
      <span>{count} {label}</span>
    </div>
  );
}

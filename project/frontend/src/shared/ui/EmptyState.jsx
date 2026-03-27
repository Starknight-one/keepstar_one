import React from 'react';
import './EmptyState.css';

const ICONS = {
  'search-x': (
    <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#9AA0A6" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="11" cy="11" r="8" /><path d="m21 21-4.3-4.3" /><path d="M8 8l6 6" /><path d="m14 8-6 6" />
    </svg>
  ),
  'shopping-bag': (
    <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#9AA0A6" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M6 2 3 6v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V6l-3-4Z" /><path d="M3 6h18" /><path d="M16 10a4 4 0 0 1-8 0" />
    </svg>
  ),
  heart: (
    <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#9AA0A6" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M19 14c1.49-1.46 3-3.21 3-5.5A5.5 5.5 0 0 0 16.5 3c-1.76 0-3 .5-4.5 2-1.5-1.5-2.74-2-4.5-2A5.5 5.5 0 0 0 2 8.5c0 2.3 1.5 4.05 3 5.5l7 7Z" />
    </svg>
  ),
};

export default function EmptyState({ icon = 'search-x', title = 'Nothing found', description, actionLabel, onAction }) {
  return (
    <div className="ui-empty-state">
      <div className="ui-empty-icon">
        {ICONS[icon] || ICONS['search-x']}
      </div>
      <div className="ui-empty-title">{title}</div>
      {description && <div className="ui-empty-desc">{description}</div>}
      {actionLabel && onAction && (
        <button className="ui-empty-btn" onClick={onAction}>{actionLabel}</button>
      )}
    </div>
  );
}

import './BackButton.css';

export function BackButton({ onClick, visible }) {
  if (!visible) return null;

  return (
    <button className="back-button" onClick={onClick} aria-label="Back">
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <path d="m12 19-7-7 7-7" /><path d="M19 12H5" />
      </svg>
      <span className="back-text">Back</span>
    </button>
  );
}

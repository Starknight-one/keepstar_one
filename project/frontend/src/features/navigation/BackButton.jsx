import './BackButton.css';

export function BackButton({ onClick, visible }) {
  if (!visible) return null;

  return (
    <button className="back-button" onClick={onClick}>
      <span className="back-arrow">&#8592;</span>
      <span className="back-text">Back</span>
    </button>
  );
}

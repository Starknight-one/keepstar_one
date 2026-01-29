export function FullscreenOverlay({ isOpen, onClose, children }) {
  if (!isOpen) return null;

  const handleBackdropClick = (e) => {
    if (e.target === e.currentTarget) {
      onClose();
    }
  };

  return (
    <div className="fullscreen-overlay" onClick={handleBackdropClick}>
      <div className="overlay-content">
        {children}
      </div>
    </div>
  );
}

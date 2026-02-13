import { useEffect, useRef } from 'react';
import './Stepper.css';

function truncateLabel(label, max = 24) {
  if (!label) return '';
  return label.length > max ? label.slice(0, max) + '\u2026' : label;
}

export function Stepper({ history, currentIndex, goTo }) {
  const activeRef = useRef(null);

  useEffect(() => {
    if (activeRef.current) {
      activeRef.current.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    }
  }, [currentIndex]);

  if (!history || history.length === 0) return null;

  const isLast = (index) => index === history.length - 1;

  return (
    <nav className="stepper">
        {history.map((entry, index) => {
          let state;
          if (index < currentIndex) state = 'past';
          else if (index === currentIndex) state = 'active';
          else state = 'future';

          return (
            <button
              key={index}
              ref={state === 'active' ? activeRef : null}
              className={`stepper-step stepper-step--${state}`}
              onClick={() => goTo(index)}
            >
              {/* Indicator column: dot/circle + line */}
              <div className="stepper-track">
                <div className={`stepper-dot stepper-dot--${state}`}>
                  {state === 'active' && <div className="stepper-dot-inner" />}
                </div>
                {!isLast(index) && (
                  <div className={`stepper-line stepper-line--${state}`} />
                )}
              </div>
              {/* Content column */}
              <div className="stepper-content">
                <span className="stepper-label">{truncateLabel(entry.label)}</span>
              </div>
            </button>
          );
        })}
    </nav>
  );
}

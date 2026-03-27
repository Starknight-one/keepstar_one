import React from 'react';
import './SelectorGroup.css';

export default function SelectorGroup({ options = [], value, onChange, disabled = false }) {
  return (
    <div className={`ui-selector-group${disabled ? ' ui-selector-group--disabled' : ''}`}>
      {options.map((opt) => (
        <button
          key={opt}
          className={`ui-selector-opt${opt === value ? ' ui-selector-opt--active' : ''}`}
          onClick={() => onChange?.(opt)}
          disabled={disabled}
        >
          {opt}
        </button>
      ))}
    </div>
  );
}

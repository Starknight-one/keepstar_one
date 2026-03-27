import React from 'react';
import './ColorSelector.css';

export default function ColorSelector({ colors = [], selected, onChange }) {
  return (
    <div className="ui-color-selector">
      {colors.map((c) => (
        <button
          key={c.value}
          className={`ui-color-swatch${c.value === selected ? ' ui-color-swatch--active' : ''}`}
          style={{ background: c.hex }}
          onClick={() => onChange?.(c.value)}
          title={c.value}
        />
      ))}
    </div>
  );
}

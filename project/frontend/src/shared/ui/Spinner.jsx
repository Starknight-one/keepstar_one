import React from 'react';
import './Spinner.css';

export default function Spinner({ size = 32 }) {
  return <div className="ui-spinner" style={{ width: size, height: size }} />;
}

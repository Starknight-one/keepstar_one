import React from 'react';
import { Play } from 'lucide-react';
import Confetti from './Confetti';
import './Sections.css';

export default function ProductDemo() {
  return (
    <div className="demo-vsl-wrapper" style={{ position: 'relative', overflow: 'hidden' }}>
      <Confetti seed={10} count={6} opacity={0.35} />
      <div className="demo-vsl-container">
        <div className="demo-vsl-content">
          <div className="vsl-play-btn">
            <Play size={28} fill="#1A1A1A" color="#1A1A1A" />
          </div>
          <div className="vsl-label">Watch how it works</div>
          <div className="vsl-duration">2:34</div>
        </div>
      </div>
    </div>
  );
}

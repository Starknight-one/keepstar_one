import React from 'react';
import { User, LayoutGrid, Code, Target, TrendingUp, Zap, Database, Globe } from 'lucide-react';
import Confetti from './Confetti';

export default function IconStrip() {
  const icons = [User, LayoutGrid, Code, Target, TrendingUp, Zap, Database, Globe];

  return (
    <section id="product" className="icon-strip-section container" style={{ position: 'relative', overflow: 'hidden' }}>
      <Confetti seed={20} count={6} opacity={0.3} />
      <div className="icon-row">
        {icons.map((Icon, idx) => (
          <div key={idx} className="icon-circle">
            <Icon size={24} color="#5F6368" />
          </div>
        ))}
      </div>
      
      <div className="big-statement-wrapper">
        <h2 className="big-statement">
          Every visitor sees a different page — built from your products, tailored to their questions, assembled in real time.
        </h2>
      </div>
    </section>
  );
}

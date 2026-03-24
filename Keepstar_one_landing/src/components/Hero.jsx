import React from 'react';
import './Hero.css';

export default function Hero({ onDemo }) {
  const shapes = [
    { type: 'ellipse', color: 'bg-blue', w: 12, top: '15%', left: '12%', delay: '' },
    { type: 'ellipse', color: 'bg-red', w: 8, top: '10%', left: '24%', delay: 'delay-100' },
    { type: 'ellipse', color: 'bg-yellow', w: 10, top: '20%', left: '36%', delay: 'delay-200' },
    { type: 'ellipse', color: 'bg-blue', w: 6, top: '8%', left: '48%', delay: '' },
    { type: 'ellipse', color: 'bg-green', w: 14, top: '18%', left: '62%', delay: 'delay-100' },
    { type: 'ellipse', color: 'bg-red', w: 8, top: '11%', left: '76%', delay: 'delay-200' },
    { type: 'ellipse', color: 'bg-blue', w: 10, top: '23%', left: '86%', delay: 'delay-300' },
    { type: 'rect', color: 'bg-red', w: 16, h: 6, top: '25%', left: '19%', rot: 'rotate(25deg)', delay: '' },
    { type: 'rect', color: 'bg-blue', w: 14, h: 6, top: '13%', left: '41%', rot: 'rotate(-15deg)', delay: 'delay-100' },
    { type: 'rect', color: 'bg-yellow', w: 18, h: 6, top: '6%', left: '57%', rot: 'rotate(40deg)', delay: 'delay-200' },
    { type: 'rect', color: 'bg-green', w: 12, h: 6, top: '25%', left: '72%', rot: 'rotate(-30deg)', delay: '' },
    { type: 'ellipse', color: 'bg-yellow', w: 8, top: '51%', left: '10%', delay: 'delay-100' },
    { type: 'ellipse', color: 'bg-green', w: 10, top: '64%', left: '27%', delay: '' },
    { type: 'rect', color: 'bg-blue', w: 16, h: 6, top: '76%', left: '45%', rot: 'rotate(20deg)', delay: 'delay-200' },
    { type: 'ellipse', color: 'bg-red', w: 12, top: '70%', left: '69%', delay: 'delay-100' },
    { type: 'rect', color: 'bg-yellow', w: 14, h: 6, top: '61%', left: '83%', rot: 'rotate(-25deg)', delay: '' },
    { type: 'ellipse', color: 'bg-blue', w: 6, top: '83%', left: '6%', delay: 'delay-300' },
    { type: 'ellipse', color: 'bg-green', w: 10, top: '44%', left: '90%', delay: 'delay-200' },
    { type: 'rect', color: 'bg-red', w: 20, h: 6, top: '44%', left: '31%', rot: 'rotate(55deg)', delay: 'delay-100' },
    { type: 'ellipse', color: 'bg-yellow', w: 8, top: '89%', left: '54%', delay: '' }
  ];

  return (
    <section className="hero-section">
      {shapes.map((s, i) => (
        <div 
          key={i} 
          className={`floating-shape ${s.type === 'ellipse' ? 'shape-ellipse' : 'shape-rect'} ${s.color} ${s.delay}`}
          style={{
            top: s.top, 
            left: s.left, 
            width: s.w, 
            height: s.h || s.w, 
            transform: s.rot || 'none'
          }}
        />
      ))}

      <div className="hero-content">
        <div className="badge">Now in public beta</div>
        
        <h1 className="hero-headline">
          Every visitor gets<br/>their own website
        </h1>
        
        <p className="hero-subhead">
          An AI chat that generates fully personalized pages in seconds.<br/>
          Replace your static site with a dynamic experience that puts<br/>
          every visitor into a conversion funnel.
        </p>

        <div className="hero-actions">
          <button className="btn-hero btn-hero-primary" onClick={onDemo}>Start free trial</button>
          <button className="btn-hero btn-hero-secondary" onClick={onDemo}>Watch demo</button>
        </div>
      </div>
    </section>
  );
}

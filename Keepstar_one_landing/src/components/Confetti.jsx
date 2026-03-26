import React from 'react';

const colors = ['bg-blue', 'bg-red', 'bg-yellow', 'bg-green'];
const delays = ['', 'delay-100', 'delay-200', 'delay-300'];

// Deterministic pseudo-random from seed
function seededRandom(seed) {
  let s = seed;
  return () => {
    s = (s * 16807 + 0) % 2147483647;
    return s / 2147483647;
  };
}

function generateShapes(seed, count = 8) {
  const rng = seededRandom(seed);
  const shapes = [];
  for (let i = 0; i < count; i++) {
    const isRect = rng() > 0.6;
    const w = Math.round(6 + rng() * 12);
    shapes.push({
      type: isRect ? 'rect' : 'ellipse',
      color: colors[Math.floor(rng() * colors.length)],
      w,
      h: isRect ? 6 : w,
      top: `${Math.round(5 + rng() * 85)}%`,
      left: `${Math.round(3 + rng() * 90)}%`,
      delay: delays[Math.floor(rng() * delays.length)],
      rot: isRect ? `rotate(${Math.round(-45 + rng() * 90)}deg)` : undefined,
    });
  }
  return shapes;
}

export default function Confetti({ seed = 1, count = 8, opacity = 0.45 }) {
  const shapes = generateShapes(seed, count);

  return (
    <>
      {shapes.map((s, i) => (
        <div
          key={i}
          className={`floating-shape ${s.type === 'ellipse' ? 'shape-ellipse' : 'shape-rect'} ${s.color} ${s.delay}`}
          style={{
            top: s.top,
            left: s.left,
            width: s.w,
            height: s.h,
            opacity,
            transform: s.rot || 'none',
          }}
        />
      ))}
    </>
  );
}

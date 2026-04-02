import { useState } from 'react';
import { LayoutTreeRenderer } from './LayoutTreeRenderer';
import { normalizeImages } from './templateUtils';
import { ImageCarousel } from './ImageCarousel';
import './GenericCardV2Template.css';

/**
 * GenericCardV2Template — read-only replay version.
 * No actions (like, cart), no click handlers.
 *
 * Props:
 *   - atomsV2: AtomV2[] — v2 atoms with textStyle/wrapper
 *   - layout: LayoutNode — tree layout
 *   - size: widget size
 *   - direction: 'horizontal' | 'vertical'
 *   - entityRef: { type, id }
 *   - states: { hover, active } — CSS variable overrides
 */
export function GenericCardV2Template({ atomsV2 = [], layout, size = 'medium', direction, entityRef, states }) {
  const [currentImageIndex, setCurrentImageIndex] = useState(0);

  // Read-only replay: no actions
  const liked = false;
  const cartQty = 0;

  const entityId = entityRef?.id;

  // Extract hero images from atomsV2 (slot=hero, type=image)
  const heroAtoms = atomsV2.filter(a => a.slot === 'hero' && (a.type === 'image'));
  const images = heroAtoms.length > 0 ? normalizeImages(heroAtoms[0].value) : [];

  // Build hover/active CSS variables from states
  const stateStyles = buildStateStyles(states);

  // Only apply horizontal CSS class in fallback mode (no layout tree).
  // When layout tree exists, direction is handled by applyHorizontalDirection in engine.
  const directionClass = (!layout && direction === 'horizontal') ? 'generic-card-horizontal' : '';

  // Apply shadow/borderRadius from root layout node to the card container
  const cardStyle = { ...stateStyles, position: 'relative' };
  if (layout?.shadow) {
    const shadowTokens = { none: 'none', sm: '0 1px 3px rgba(0,0,0,0.1)', md: '0 4px 12px rgba(0,0,0,0.1)', lg: '0 8px 24px rgba(0,0,0,0.12)' };
    cardStyle.boxShadow = shadowTokens[layout.shadow] || layout.shadow;
  }
  if (layout?.borderRadius) {
    const radiusTokens = { none: '0', sm: '4px', md: '8px', lg: '12px', xl: '16px', full: '9999px' };
    cardStyle.borderRadius = radiusTokens[layout.borderRadius] || layout.borderRadius;
    cardStyle.overflow = 'hidden';
  }

  return (
    <div
      className={`generic-card generic-card-v2 size-${size} ${directionClass}`}
      style={cardStyle}
    >
      {/* Hero images: only render separately when there's NO layout tree (fallback mode).
          When layout tree exists, it handles hero rendering via the hero node. */}
      {!layout && images.length > 0 && (
        <div className="generic-card-media" style={{ position: 'relative' }}>
          <ImageCarousel
            images={images}
            currentIndex={currentImageIndex}
            onIndexChange={setCurrentImageIndex}
          />
        </div>
      )}
      <div className="generic-card-content">
        {layout ? (
          <LayoutTreeRenderer node={layout} atoms={atomsV2} />
        ) : (
          // Fallback: render atomsV2 sequentially
          atomsV2.filter(a => a.slot !== 'hero').map((atom, i) => (
            <span key={i} className="atom-v2-fallback">{String(atom.value)}</span>
          ))
        )}
      </div>
    </div>
  );
}

function buildStateStyles(states) {
  if (!states) return undefined;
  const vars = {};
  if (states.hover) {
    Object.entries(states.hover).forEach(([key, value]) => {
      vars[`--hover-${key}`] = value;
    });
  }
  if (states.active) {
    Object.entries(states.active).forEach(([key, value]) => {
      vars[`--active-${key}`] = value;
    });
  }
  return Object.keys(vars).length > 0 ? vars : undefined;
}

import { useState } from 'react';
import { LayoutTreeRenderer } from './LayoutTreeRenderer';
import { normalizeImages } from './templateUtils';
import { ImageCarousel } from './ImageCarousel';
import { useActions } from '../../../features/actions/ActionContext';

/**
 * GenericCardV2Template — renders widgets using v2 LayoutNode tree + AtomV2.
 * Falls back to v1 zone-based GenericCardTemplate if no layout is present.
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
  const { toggleLike, isLiked, addToCart, getCartQuantity } = useActions();
  const entityId = entityRef?.id;
  const liked = entityId ? isLiked(entityId) : false;
  const cartQty = entityId ? getCartQuantity(entityId) : 0;

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
      {/* Like button overlay when layout tree handles hero rendering */}
      {layout && entityId && (
        <button
          className={`product-card-favorite${liked ? ' liked' : ''}`}
          style={{ position: 'absolute', top: 8, right: 8, zIndex: 2 }}
          onClick={(e) => { e.stopPropagation(); toggleLike(entityId); }}
          aria-label={liked ? 'Unlike' : 'Like'}
        >
          <svg viewBox="0 0 24 24" fill={liked ? 'currentColor' : 'none'} stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z" />
          </svg>
        </button>
      )}
      {/* Hero images: only render separately when there's NO layout tree (fallback mode).
          When layout tree exists, it handles hero rendering via the hero node. */}
      {!layout && images.length > 0 && (
        <div className="generic-card-media" style={{ position: 'relative' }}>
          <ImageCarousel
            images={images}
            currentIndex={currentImageIndex}
            onIndexChange={setCurrentImageIndex}
          />
          {entityId && (
            <button
              className={`product-card-favorite${liked ? ' liked' : ''}`}
              onClick={(e) => { e.stopPropagation(); toggleLike(entityId); }}
              aria-label={liked ? 'Unlike' : 'Like'}
            >
              <svg viewBox="0 0 24 24" fill={liked ? 'currentColor' : 'none'} stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z" />
              </svg>
            </button>
          )}
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
        {entityId && (
          <button
            className={`product-card-cart-btn${cartQty > 0 ? ' in-cart' : ''}`}
            onClick={(e) => { e.stopPropagation(); addToCart(entityRef.type, entityId); }}
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="9" cy="21" r="1" /><circle cx="20" cy="21" r="1" />
              <path d="M1 1h4l2.68 13.39a2 2 0 0 0 2 1.61h9.72a2 2 0 0 0 2-1.61L23 6H6" />
            </svg>
            {cartQty > 0 ? `In cart (${cartQty})` : 'Add to cart'}
          </button>
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

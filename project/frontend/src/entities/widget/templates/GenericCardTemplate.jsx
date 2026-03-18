import { useState } from 'react';
import { AtomRenderer } from '../../atom/AtomRenderer';
import { normalizeImages } from './templateUtils';
import { ImageCarousel } from './ImageCarousel';
import { useActions } from '../../../features/actions/ActionContext';
import './GenericCardTemplate.css';

/**
 * GenericCard — universal template that renders atoms in backend-defined order.
 * If zones[] are present, uses zone-based layout. Otherwise falls back to legacy layout.
 */
export function GenericCardTemplate({ atoms = [], zones, size = 'medium', direction, entityRef }) {
  if (zones && zones.length > 0) {
    return <ZoneLayout atoms={atoms} zones={zones} size={size} direction={direction} entityRef={entityRef} />;
  }
  return <LegacyLayout atoms={atoms} size={size} direction={direction} entityRef={entityRef} />;
}

/** Zone-based layout — backend decides grouping, frontend only applies CSS classes */
function ZoneLayout({ atoms, zones, size, direction, entityRef }) {
  const [currentImageIndex, setCurrentImageIndex] = useState(0);
  const { toggleLike, isLiked, addToCart, getCartQuantity } = useActions();
  const entityId = entityRef?.id;
  const liked = entityId ? isLiked(entityId) : false;
  const cartQty = entityId ? getCartQuantity(entityId) : 0;

  const heroZones = zones.filter(z => z.type === 'hero');
  const contentZones = zones.filter(z => z.type !== 'hero');

  // Collect hero images from hero zone atoms
  const heroAtoms = heroZones.flatMap(z => z.atomIndices.map(i => atoms[i]).filter(Boolean));
  const images = heroAtoms.length > 0 ? normalizeImages(heroAtoms[0].value) : [];

  const directionClass = direction === 'horizontal' ? 'generic-card-horizontal' : '';

  return (
    <div className={`generic-card size-${size} ${directionClass}`}>
      {images.length > 0 && (
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
        {contentZones.map((zone, zi) => (
          <ZoneRenderer key={zi} zone={zone} atoms={atoms} />
        ))}
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

/** Renders a single zone with the appropriate CSS class */
function ZoneRenderer({ zone, atoms }) {
  const [expanded, setExpanded] = useState(false);

  if (zone.type === 'collapsed') {
    return (
      <div className="zone-collapsed">
        {expanded && (
          <div className="zone-flow">
            {zone.atomIndices.map(idx => (
              <AtomRenderer key={idx} atom={atoms[idx]} />
            ))}
          </div>
        )}
        <button
          className="zone-fold-toggle"
          onClick={() => setExpanded(!expanded)}
        >
          {expanded ? 'Скрыть' : zone.foldLabel || 'Показать ещё'}
        </button>
      </div>
    );
  }

  return (
    <div className={`zone-${zone.type}`}>
      {zone.atomIndices.map(idx => (
        <AtomRenderer key={idx} atom={atoms[idx]} />
      ))}
    </div>
  );
}

/** Legacy layout — image on top, content below (backward compat) */
function LegacyLayout({ atoms, size, direction, entityRef }) {
  const [currentImageIndex, setCurrentImageIndex] = useState(0);
  const { toggleLike, isLiked, addToCart, getCartQuantity } = useActions();
  const entityId = entityRef?.id;
  const liked = entityId ? isLiked(entityId) : false;
  const cartQty = entityId ? getCartQuantity(entityId) : 0;

  const imageAtoms = atoms.filter(a => a.type === 'image');
  const contentAtoms = atoms.filter(a => a.type !== 'image');
  const images = imageAtoms.length > 0 ? normalizeImages(imageAtoms[0].value) : [];

  const directionClass = direction === 'horizontal' ? 'generic-card-horizontal' : '';

  return (
    <div className={`generic-card size-${size} ${directionClass}`}>
      {images.length > 0 && (
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
        {contentAtoms.map((atom, i) => (
          <AtomRenderer key={i} atom={atom} />
        ))}
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

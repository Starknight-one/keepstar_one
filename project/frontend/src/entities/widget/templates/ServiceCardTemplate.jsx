import { useState } from 'react';
import { AtomRenderer } from '../../atom/AtomRenderer';
import { groupAtomsBySlot, normalizeImages } from './templateUtils';
import { ImageCarousel } from './ImageCarousel';
import './ServiceCardTemplate.css';

// Slot names match backend domain.AtomSlot
const SLOTS = {
  HERO: 'hero',
  BADGE: 'badge',
  TITLE: 'title',
  PRIMARY: 'primary',
  PRICE: 'price',
  SECONDARY: 'secondary',
};

export function ServiceCardTemplate({ atoms = [], size = 'medium' }) {
  const [expanded, setExpanded] = useState(false);
  const [currentImageIndex, setCurrentImageIndex] = useState(0);

  // Group atoms by slot
  const slots = groupAtomsBySlot(atoms);

  const heroAtoms = slots[SLOTS.HERO] || [];
  const badgeAtoms = slots[SLOTS.BADGE] || [];
  const titleAtoms = slots[SLOTS.TITLE] || [];
  const primaryAtoms = slots[SLOTS.PRIMARY] || [];
  const priceAtoms = slots[SLOTS.PRICE] || [];
  const secondaryAtoms = slots[SLOTS.SECONDARY] || [];

  const hasSecondary = secondaryAtoms.length > 0;

  // Get images from hero slot (can be array or single value)
  const images = heroAtoms.length > 0 ? normalizeImages(heroAtoms[0].value) : [];

  return (
    <div className={`service-card-template size-${size}`}>
      {/* Image Area */}
      {images.length > 0 ? (
        <div className="service-card-images">
          <ImageCarousel
            images={images}
            currentIndex={currentImageIndex}
            onIndexChange={setCurrentImageIndex}
          />
          {/* Badge overlay */}
          {badgeAtoms.length > 0 && (
            <div className="service-card-badge-container">
              <AtomRenderer atom={badgeAtoms[0]} />
            </div>
          )}
        </div>
      ) : (
        <div className="service-card-icon">
          <span className="service-icon">🛠️</span>
        </div>
      )}

      {/* Content Area */}
      <div className="service-card-content">
        {/* Title */}
        {titleAtoms.length > 0 && (
          <div className="service-card-title">
            <AtomRenderer atom={titleAtoms[0]} />
          </div>
        )}

        {/* Primary Attributes (duration, provider, rating) */}
        {primaryAtoms.length > 0 && (
          <div className="service-card-primary">
            {primaryAtoms.map((atom, i) => (
              <AtomChip key={i} atom={atom} />
            ))}
          </div>
        )}

        {/* Price */}
        {priceAtoms.length > 0 && (
          <div className="service-card-price">
            <AtomRenderer atom={priceAtoms[0]} />
          </div>
        )}

        {/* Expand Button & Secondary */}
        {hasSecondary && (
          <>
            <button
              className="service-card-expand"
              onClick={() => setExpanded(!expanded)}
            >
              {expanded ? 'Hide details' : 'Show details'}
            </button>

            {expanded && (
              <div className="service-card-secondary">
                {secondaryAtoms.map((atom, i) => (
                  <div key={i} className="service-card-secondary-item">
                    {atom.meta?.label && (
                      <span className="secondary-label">{atom.meta.label}:</span>
                    )}
                    <AtomRenderer atom={atom} />
                  </div>
                ))}
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}

function AtomChip({ atom }) {
  const value = atom.value;
  // Use atom.display (new) or fallback to meta.display (legacy)
  const display = atom.display || atom.meta?.display;

  // Duration display with clock icon
  if (atom.meta?.label === 'duration' || (typeof value === 'string' && value.includes('min'))) {
    return (
      <div className="service-card-chip service-card-duration">
        ⏱️ <AtomRenderer atom={atom} />
      </div>
    );
  }

  // Provider display
  if (atom.meta?.label === 'provider') {
    return (
      <div className="service-card-chip service-card-provider">
        👤 <AtomRenderer atom={atom} />
      </div>
    );
  }

  // Rating display - check subtype (new) or type (legacy)
  if (atom.subtype === 'rating' || atom.type === 'rating') {
    return (
      <div className="service-card-chip service-card-rating">
        <AtomRenderer atom={atom} />
      </div>
    );
  }

  // Caption display
  if (display === 'caption') {
    return (
      <span className="service-card-caption">
        <AtomRenderer atom={atom} />
      </span>
    );
  }

  // Default chip display
  return (
    <div className="service-card-chip">
      <AtomRenderer atom={atom} />
    </div>
  );
}

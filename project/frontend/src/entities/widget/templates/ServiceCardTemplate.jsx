import { useState } from 'react';
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
      {/* Hero: Image (optional for services) */}
      {images.length > 0 && (
        <div className="service-card-images">
          <ImageCarousel
            images={images}
            currentIndex={currentImageIndex}
            onIndexChange={setCurrentImageIndex}
          />
          {/* Badge overlay */}
          {badgeAtoms.length > 0 && (
            <BadgeOverlay atom={badgeAtoms[0]} />
          )}
        </div>
      )}

      {/* Service Icon (when no image) */}
      {images.length === 0 && (
        <div className="service-card-icon">
          <span className="service-icon">🛠️</span>
        </div>
      )}

      {/* Title */}
      {titleAtoms.length > 0 && (
        <div className="service-card-title">
          {titleAtoms[0].value}
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
          {priceAtoms[0].meta?.currency || '$'}{priceAtoms[0].value}
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
                  <span className="secondary-value">{atom.value}</span>
                </div>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}

// Group atoms by their slot field
function groupAtomsBySlot(atoms) {
  const slots = {};
  for (const atom of atoms) {
    const slot = atom.slot || 'primary'; // Default to primary if no slot
    if (!slots[slot]) {
      slots[slot] = [];
    }
    slots[slot].push(atom);
  }
  return slots;
}

// Normalize image value to array
function normalizeImages(value) {
  if (Array.isArray(value)) return value;
  if (typeof value === 'string') return [value];
  return [];
}

function ImageCarousel({ images, currentIndex, onIndexChange }) {
  if (!images || images.length === 0) return null;

  const handleDotClick = (index) => {
    onIndexChange(index);
  };

  const handleImageClick = () => {
    onIndexChange((currentIndex + 1) % images.length);
  };

  return (
    <div className="image-carousel">
      <img
        src={images[currentIndex]}
        alt=""
        className="carousel-image"
        onClick={handleImageClick}
      />
      {images.length > 1 && (
        <div className="carousel-dots">
          {images.map((_, index) => (
            <button
              key={index}
              className={`carousel-dot ${index === currentIndex ? 'active' : ''}`}
              onClick={() => handleDotClick(index)}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function BadgeOverlay({ atom }) {
  const variant = atom.meta?.variant || 'default';
  return (
    <div className={`service-card-badge variant-${variant}`}>
      {atom.value}
    </div>
  );
}

function AtomChip({ atom }) {
  const value = atom.value;

  // Duration display with clock icon
  if (atom.meta?.label === 'duration' || (typeof value === 'string' && value.includes('min'))) {
    return (
      <div className="service-card-chip service-card-duration">
        ⏱️ {value}
      </div>
    );
  }

  // Provider display
  if (atom.meta?.label === 'provider') {
    return (
      <div className="service-card-chip service-card-provider">
        👤 {value}
      </div>
    );
  }

  // Rating display
  if (atom.type === 'rating') {
    const stars = Math.round(Number(value) || 0);
    return (
      <div className="service-card-chip service-card-rating">
        {'★'.repeat(Math.min(stars, 5))}{'☆'.repeat(Math.max(0, 5 - stars))}
      </div>
    );
  }

  // Default chip display
  return (
    <div className="service-card-chip">
      {value}
    </div>
  );
}

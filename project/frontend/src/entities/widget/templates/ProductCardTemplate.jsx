import { useState } from 'react';
import './ProductCardTemplate.css';

// Slot names match backend domain.AtomSlot
const SLOTS = {
  HERO: 'hero',
  BADGE: 'badge',
  TITLE: 'title',
  PRIMARY: 'primary',
  PRICE: 'price',
  SECONDARY: 'secondary',
};

export function ProductCardTemplate({ atoms = [], size = 'medium', onSelect }) {
  const [expanded, setExpanded] = useState(false);
  const [currentImageIndex, setCurrentImageIndex] = useState(0);
  const [selectedValues, setSelectedValues] = useState({});

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

  const handleSelectorClick = (atomIndex, value) => {
    setSelectedValues((prev) => ({ ...prev, [atomIndex]: value }));
    onSelect?.(atomIndex, value);
  };

  return (
    <div className={`product-card-template size-${size}`}>
      {/* Hero: Image Carousel */}
      {images.length > 0 && (
        <div className="product-card-images">
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

      {/* Title */}
      {titleAtoms.length > 0 && (
        <div className="product-card-title">
          {titleAtoms[0].value}
        </div>
      )}

      {/* Primary Attributes */}
      {primaryAtoms.length > 0 && (
        <div className="product-card-primary">
          {primaryAtoms.map((atom, i) => (
            <AtomChip
              key={i}
              atom={atom}
              selected={selectedValues[i]}
              onSelect={(value) => handleSelectorClick(i, value)}
            />
          ))}
        </div>
      )}

      {/* Price */}
      {priceAtoms.length > 0 && (
        <div className="product-card-price">
          {priceAtoms[0].meta?.currency || '$'}{priceAtoms[0].value}
        </div>
      )}

      {/* Expand Button & Secondary */}
      {hasSecondary && (
        <>
          <button
            className="product-card-expand"
            onClick={() => setExpanded(!expanded)}
          >
            {expanded ? 'Hide details' : 'Show details'}
          </button>

          {expanded && (
            <div className="product-card-secondary">
              {secondaryAtoms.map((atom, i) => (
                <div key={i} className="product-card-secondary-item">
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
    <div className={`product-card-badge variant-${variant}`}>
      {atom.value}
    </div>
  );
}

function AtomChip({ atom, selected, onSelect }) {
  const display = atom.meta?.display || 'chip';
  const value = atom.value;

  // Selector display - for arrays (sizes, colors)
  if (display === 'selector' && Array.isArray(value)) {
    return (
      <div className="product-card-selector">
        {atom.meta?.label && (
          <span className="selector-label">{atom.meta.label}:</span>
        )}
        <div className="selector-options">
          {value.map((option) => (
            <button
              key={option}
              className={`selector-option ${selected === option ? 'selected' : ''}`}
              onClick={() => onSelect(option)}
            >
              {option}
            </button>
          ))}
        </div>
      </div>
    );
  }

  // Rating display
  if (atom.type === 'rating') {
    const stars = Math.round(Number(value) || 0);
    return (
      <div className="product-card-chip product-card-rating">
        {'★'.repeat(Math.min(stars, 5))}{'☆'.repeat(Math.max(0, 5 - stars))}
      </div>
    );
  }

  // Text display - no border
  if (display === 'text') {
    return (
      <span className="product-card-text">
        {atom.meta?.label && <span className="text-label">{atom.meta.label}:</span>}
        <span className="text-value">{value}</span>
      </span>
    );
  }

  // Default chip display
  return (
    <div className="product-card-chip">
      {value}
    </div>
  );
}

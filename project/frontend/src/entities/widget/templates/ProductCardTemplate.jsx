import { useState } from 'react';
import { AtomRenderer } from '../../atom/AtomRenderer';
import { groupAtomsBySlot, normalizeImages } from './templateUtils';
import { ImageCarousel } from './ImageCarousel';
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
      {/* Image Area */}
      <div className="product-card-images">
        {images.length > 0 ? (
          <ImageCarousel
            images={images}
            currentIndex={currentImageIndex}
            onIndexChange={setCurrentImageIndex}
          />
        ) : (
          <div className="image-placeholder" />
        )}

        {/* Badge overlay */}
        {badgeAtoms.length > 0 && (
          <div className="product-card-badge-container">
            <AtomRenderer atom={badgeAtoms[0]} />
          </div>
        )}
      </div>

      {/* Content Area */}
      <div className="product-card-content">
        {/* Title */}
        {titleAtoms.length > 0 && (
          <div className="product-card-title">
            <AtomRenderer atom={titleAtoms[0]} />
          </div>
        )}

        {/* Primary Attributes (rating, brand chips) */}
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
            <AtomRenderer atom={priceAtoms[0]} />
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

function AtomChip({ atom, selected, onSelect }) {
  // Use atom.display (new) or fallback to atom.meta?.display (legacy)
  const display = atom.display || atom.meta?.display || 'chip';
  const value = atom.value;

  // Selector display - for arrays (sizes, colors)
  if ((display === 'selector' || display === 'tag') && Array.isArray(value)) {
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

  // Rating display - check subtype (new) or type (legacy)
  if (atom.subtype === 'rating' || atom.type === 'rating') {
    return (
      <div className="product-card-chip product-card-rating">
        <span className="star-icon">★</span>
        <span className="rating-value">{atom.value}</span>
      </div>
    );
  }

  // Text display - no border
  if (display === 'text' || display === 'caption') {
    return (
      <span className="product-card-text">
        {atom.meta?.label && <span className="text-label">{atom.meta.label}:</span>}
        <AtomRenderer atom={atom} />
      </span>
    );
  }

  // Tag/chip display - use AtomRenderer
  return (
    <div className="product-card-chip">
      <AtomRenderer atom={atom} />
    </div>
  );
}

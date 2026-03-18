import { useState } from 'react';
import { AtomRenderer } from '../../atom/AtomRenderer';
import { groupAtomsBySlot, normalizeImages } from './templateUtils';
import { ImageCarousel } from './ImageCarousel';
import { useActions } from '../../../features/actions/ActionContext';
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

export function ProductCardTemplate({ atoms = [], size = 'medium', onSelect, entityRef }) {
  const [expanded, setExpanded] = useState(false);
  const [currentImageIndex, setCurrentImageIndex] = useState(0);
  const [selectedValues, setSelectedValues] = useState({});
  const { toggleLike, isLiked, addToCart, getCartQuantity } = useActions();
  const entityId = entityRef?.id;
  const liked = entityId ? isLiked(entityId) : false;
  const cartQty = entityId ? getCartQuantity(entityId) : 0;

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

        {/* Favorite button */}
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

        {/* Add to Cart */}
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

import { useState } from 'react';
import { AtomRenderer } from '../../atom/AtomRenderer';
import { groupAtomsBySlot, normalizeImages } from './templateUtils';
import { useActions } from '../../../features/actions/ActionContext';
import './ProductDetailTemplate.css';

// Slot names match backend domain.AtomSlot
const SLOTS = {
  GALLERY: 'gallery',
  TITLE: 'title',
  PRIMARY: 'primary',
  PRICE: 'price',
  STOCK: 'stock',
  DESCRIPTION: 'description',
  TAGS: 'tags',
  SPECS: 'specs',
};

export function ProductDetailTemplate({ atoms = [], entityRef }) {
  const [currentImageIndex, setCurrentImageIndex] = useState(0);
  const { toggleLike, isLiked, addToCart, getCartQuantity } = useActions();
  const entityId = entityRef?.id;
  const liked = entityId ? isLiked(entityId) : false;
  const cartQty = entityId ? getCartQuantity(entityId) : 0;

  // Group atoms by slot
  const slots = groupAtomsBySlot(atoms);

  const galleryAtoms = slots[SLOTS.GALLERY] || [];
  const titleAtoms = slots[SLOTS.TITLE] || [];
  const primaryAtoms = slots[SLOTS.PRIMARY] || [];
  const priceAtoms = slots[SLOTS.PRICE] || [];
  const stockAtoms = slots[SLOTS.STOCK] || [];
  const descriptionAtoms = slots[SLOTS.DESCRIPTION] || [];
  const tagsAtoms = slots[SLOTS.TAGS] || [];
  const specsAtoms = slots[SLOTS.SPECS] || [];

  // Get images from gallery slot
  const images = galleryAtoms.length > 0 ? normalizeImages(galleryAtoms[0].value) : [];

  return (
    <div className="product-detail-template">
      <div className="product-detail-layout">
        {/* Left: Gallery */}
        <div className="product-detail-gallery">
          <ImageGallery
            images={images}
            currentIndex={currentImageIndex}
            onIndexChange={setCurrentImageIndex}
          />
        </div>

        {/* Right: Info */}
        <div className="product-detail-info">
          {/* Title - use AtomRenderer */}
          {titleAtoms.length > 0 && (
            <h1 className="product-detail-title">
              <AtomRenderer atom={titleAtoms[0]} />
            </h1>
          )}

          {/* Primary Attributes (brand, category, rating) */}
          {primaryAtoms.length > 0 && (
            <div className="product-detail-primary">
              {primaryAtoms.map((atom, i) => (
                <AtomChip key={i} atom={atom} />
              ))}
            </div>
          )}

          {/* Price - use AtomRenderer */}
          {priceAtoms.length > 0 && (
            <div className="product-detail-price">
              <AtomRenderer atom={priceAtoms[0]} />
            </div>
          )}

          {/* Action buttons */}
          {entityId && (
            <div className="detail-actions-row">
              <button
                className={`detail-cart-btn${cartQty > 0 ? ' in-cart' : ''}`}
                onClick={(e) => { e.stopPropagation(); addToCart(entityRef.type, entityId); }}
              >
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <circle cx="9" cy="21" r="1" /><circle cx="20" cy="21" r="1" />
                  <path d="M1 1h4l2.68 13.39a2 2 0 0 0 2 1.61h9.72a2 2 0 0 0 2-1.61L23 6H6" />
                </svg>
                {cartQty > 0 ? `In cart (${cartQty})` : 'Add to cart'}
              </button>
              <button
                className={`detail-favorite-btn${liked ? ' liked' : ''}`}
                onClick={(e) => { e.stopPropagation(); toggleLike(entityId); }}
                aria-label={liked ? 'Unlike' : 'Like'}
              >
                <svg viewBox="0 0 24 24" fill={liked ? 'currentColor' : 'none'} stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z" />
                </svg>
              </button>
            </div>
          )}

          {/* Stock */}
          <StockIndicator atoms={stockAtoms} />

          {/* Description - use AtomRenderer */}
          {descriptionAtoms.length > 0 && (
            <div className="product-detail-description">
              <h3>Description</h3>
              <p><AtomRenderer atom={descriptionAtoms[0]} /></p>
            </div>
          )}

          {/* Tags */}
          <TagsList atoms={tagsAtoms} />

          {/* Specs */}
          <SpecsTable atoms={specsAtoms} />
        </div>
      </div>
    </div>
  );
}

function ImageGallery({ images, currentIndex, onIndexChange }) {
  if (!images || images.length === 0) {
    return (
      <div className="gallery-placeholder">
        <span>No image available</span>
      </div>
    );
  }

  return (
    <div className="gallery-container">
      <div className="gallery-main">
        <img
          src={images[currentIndex]}
          alt=""
          className="gallery-main-image"
        />
      </div>
      {images.length > 1 && (
        <div className="gallery-thumbnails">
          {images.map((img, index) => (
            <button
              key={index}
              className={`gallery-thumbnail ${index === currentIndex ? 'active' : ''}`}
              onClick={() => onIndexChange(index)}
            >
              <img src={img} alt="" />
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

function StockIndicator({ atoms }) {
  if (!atoms || atoms.length === 0) return null;

  const stockValue = atoms[0].value;
  const isInStock = stockValue > 0;

  return (
    <div className={`product-detail-stock ${isInStock ? 'in-stock' : 'out-of-stock'}`}>
      {isInStock ? `In stock: ${stockValue}` : 'Out of stock'}
    </div>
  );
}

function TagsList({ atoms }) {
  if (!atoms || atoms.length === 0) return null;

  const tags = atoms[0].value;
  if (!tags || !Array.isArray(tags) || tags.length === 0) return null;

  return (
    <div className="product-detail-tags">
      <h3>Tags</h3>
      <div className="tags-list">
        {tags.map((tag, i) => (
          <span key={i} className="tag-chip">{tag}</span>
        ))}
      </div>
    </div>
  );
}

function SpecsTable({ atoms }) {
  if (!atoms || atoms.length === 0) return null;

  const attributes = atoms[0].value;
  if (!attributes || typeof attributes !== 'object') return null;

  const entries = Object.entries(attributes);
  if (entries.length === 0) return null;

  return (
    <div className="product-detail-specs">
      <h3>Specifications</h3>
      <table className="specs-table">
        <tbody>
          {entries.map(([key, value]) => (
            <tr key={key}>
              <td className="spec-key">{key}</td>
              <td className="spec-value">{String(value)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function AtomChip({ atom }) {
  // Use atom.display (new) or fallback to meta.display (legacy)
  const display = atom.display || atom.meta?.display;

  // Rating display - check subtype (new) or type (legacy)
  if (atom.subtype === 'rating' || atom.type === 'rating') {
    return (
      <div className="product-detail-chip product-detail-rating">
        <AtomRenderer atom={atom} />
      </div>
    );
  }

  // Tag display
  if (display === 'tag' || display === 'tag-active') {
    return (
      <div className="product-detail-chip">
        <AtomRenderer atom={atom} />
      </div>
    );
  }

  // Default chip display
  return (
    <div className="product-detail-chip">
      <AtomRenderer atom={atom} />
    </div>
  );
}

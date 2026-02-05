import { useState } from 'react';
import { AtomRenderer } from '../../atom/AtomRenderer';
import './ServiceDetailTemplate.css';

// Slot names match backend domain.AtomSlot
const SLOTS = {
  GALLERY: 'gallery',
  TITLE: 'title',
  PRIMARY: 'primary',
  PRICE: 'price',
  STOCK: 'stock',
  DESCRIPTION: 'description',
  SPECS: 'specs',
};

export function ServiceDetailTemplate({ atoms = [] }) {
  const [currentImageIndex, setCurrentImageIndex] = useState(0);

  // Group atoms by slot
  const slots = groupAtomsBySlot(atoms);

  const galleryAtoms = slots[SLOTS.GALLERY] || [];
  const titleAtoms = slots[SLOTS.TITLE] || [];
  const primaryAtoms = slots[SLOTS.PRIMARY] || [];
  const priceAtoms = slots[SLOTS.PRICE] || [];
  const stockAtoms = slots[SLOTS.STOCK] || [];
  const descriptionAtoms = slots[SLOTS.DESCRIPTION] || [];
  const specsAtoms = slots[SLOTS.SPECS] || [];

  // Get images from gallery slot
  const images = galleryAtoms.length > 0 ? normalizeImages(galleryAtoms[0].value) : [];
  const hasImages = images.length > 0;

  return (
    <div className="service-detail-template">
      <div className="service-detail-layout">
        {/* Left: Gallery or Icon */}
        <div className="service-detail-gallery">
          {hasImages ? (
            <ImageGallery
              images={images}
              currentIndex={currentImageIndex}
              onIndexChange={setCurrentImageIndex}
            />
          ) : (
            <div className="service-detail-icon-large">
              <span className="service-icon-large">&#128736;</span>
            </div>
          )}
        </div>

        {/* Right: Info */}
        <div className="service-detail-info">
          {/* Title - use AtomRenderer */}
          {titleAtoms.length > 0 && (
            <h1 className="service-detail-title">
              <AtomRenderer atom={titleAtoms[0]} />
            </h1>
          )}

          {/* Primary Attributes (provider, duration, rating) */}
          {primaryAtoms.length > 0 && (
            <div className="service-detail-primary">
              {primaryAtoms.map((atom, i) => (
                <AtomChip key={i} atom={atom} />
              ))}
            </div>
          )}

          {/* Price - use AtomRenderer */}
          {priceAtoms.length > 0 && (
            <div className="service-detail-price">
              <AtomRenderer atom={priceAtoms[0]} />
            </div>
          )}

          {/* Availability */}
          <AvailabilityIndicator atoms={stockAtoms} />

          {/* Description - use AtomRenderer */}
          {descriptionAtoms.length > 0 && (
            <div className="service-detail-description">
              <h3>About this service</h3>
              <p><AtomRenderer atom={descriptionAtoms[0]} /></p>
            </div>
          )}

          {/* Specs */}
          <SpecsTable atoms={specsAtoms} />
        </div>
      </div>
    </div>
  );
}

// Group atoms by their slot field
function groupAtomsBySlot(atoms) {
  const slots = {};
  for (const atom of atoms) {
    const slot = atom.slot || 'primary';
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

function ImageGallery({ images, currentIndex, onIndexChange }) {
  if (!images || images.length === 0) return null;

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

function AvailabilityIndicator({ atoms }) {
  if (!atoms || atoms.length === 0) return null;

  const availability = atoms[0].value;
  const isAvailable = availability === 'available' || availability === 'Available';

  return (
    <div className={`service-detail-availability ${isAvailable ? 'available' : 'busy'}`}>
      {isAvailable ? 'Available' : availability || 'Busy'}
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
    <div className="service-detail-specs">
      <h3>Details</h3>
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
  const value = atom.value;
  // Use atom.display (new) or fallback to meta.display (legacy)
  const display = atom.display || atom.meta?.display;

  // Duration display with clock icon
  if (atom.meta?.label === 'duration' || (typeof value === 'string' && value.includes('min'))) {
    return (
      <div className="service-detail-chip service-detail-duration">
        &#9201; <AtomRenderer atom={atom} />
      </div>
    );
  }

  // Provider display
  if (atom.meta?.label === 'provider') {
    return (
      <div className="service-detail-chip service-detail-provider">
        &#128100; <AtomRenderer atom={atom} />
      </div>
    );
  }

  // Rating display - check subtype (new) or type (legacy)
  if (atom.subtype === 'rating' || atom.type === 'rating') {
    return (
      <div className="service-detail-chip service-detail-rating">
        <AtomRenderer atom={atom} />
      </div>
    );
  }

  // Tag display
  if (display === 'tag' || display === 'caption') {
    return (
      <div className="service-detail-chip">
        <AtomRenderer atom={atom} />
      </div>
    );
  }

  // Default chip display
  return (
    <div className="service-detail-chip">
      <AtomRenderer atom={atom} />
    </div>
  );
}

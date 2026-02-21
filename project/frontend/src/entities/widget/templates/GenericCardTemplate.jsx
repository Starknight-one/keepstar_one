import { useState } from 'react';
import { AtomRenderer } from '../../atom/AtomRenderer';
import { normalizeImages } from './templateUtils';
import { ImageCarousel } from './ImageCarousel';
import './GenericCardTemplate.css';

/**
 * GenericCard — universal template that renders atoms in backend-defined order.
 * Frontend only decides: images on top, content below.
 */
export function GenericCardTemplate({ atoms = [], size = 'medium', direction }) {
  const [currentImageIndex, setCurrentImageIndex] = useState(0);

  // Split: image atoms go to media zone, rest to content zone
  const imageAtoms = atoms.filter(a => a.type === 'image');
  const contentAtoms = atoms.filter(a => a.type !== 'image');

  // Get images from first image atom
  const images = imageAtoms.length > 0 ? normalizeImages(imageAtoms[0].value) : [];

  const directionClass = direction === 'horizontal' ? 'generic-card-horizontal' : '';

  return (
    <div className={`generic-card size-${size} ${directionClass}`}>
      {images.length > 0 && (
        <div className="generic-card-media">
          <ImageCarousel
            images={images}
            currentIndex={currentImageIndex}
            onIndexChange={setCurrentImageIndex}
          />
        </div>
      )}
      <div className="generic-card-content">
        {contentAtoms.map((atom, i) => (
          <AtomRenderer key={i} atom={atom} />
        ))}
      </div>
    </div>
  );
}

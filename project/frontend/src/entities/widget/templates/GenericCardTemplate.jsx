import { useState } from 'react';
import { AtomRenderer } from '../../atom/AtomRenderer';
import { normalizeImages } from './templateUtils';
import { ImageCarousel } from './ImageCarousel';
import './GenericCardTemplate.css';

/**
 * GenericCard — universal template that renders atoms in backend-defined order.
 * If zones[] are present, uses zone-based layout. Otherwise falls back to legacy layout.
 */
export function GenericCardTemplate({ atoms = [], zones, size = 'medium', direction }) {
  if (zones && zones.length > 0) {
    return <ZoneLayout atoms={atoms} zones={zones} size={size} direction={direction} />;
  }
  return <LegacyLayout atoms={atoms} size={size} direction={direction} />;
}

/** Zone-based layout — backend decides grouping, frontend only applies CSS classes */
function ZoneLayout({ atoms, zones, size, direction }) {
  const [currentImageIndex, setCurrentImageIndex] = useState(0);

  const heroZones = zones.filter(z => z.type === 'hero');
  const contentZones = zones.filter(z => z.type !== 'hero');

  // Collect hero images from hero zone atoms
  const heroAtoms = heroZones.flatMap(z => z.atomIndices.map(i => atoms[i]).filter(Boolean));
  const images = heroAtoms.length > 0 ? normalizeImages(heroAtoms[0].value) : [];

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
        {contentZones.map((zone, zi) => (
          <ZoneRenderer key={zi} zone={zone} atoms={atoms} />
        ))}
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
function LegacyLayout({ atoms, size, direction }) {
  const [currentImageIndex, setCurrentImageIndex] = useState(0);

  const imageAtoms = atoms.filter(a => a.type === 'image');
  const contentAtoms = atoms.filter(a => a.type !== 'image');
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

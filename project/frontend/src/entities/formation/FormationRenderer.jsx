import { useState, useEffect, useRef } from 'react';
import { FormationMode } from './formationModel';
import { WidgetRenderer } from '../widget/WidgetRenderer';
import { ComparisonTemplate } from '../widget/templates/ComparisonTemplate';
import './Formation.css';

const BATCH_SIZE = 12;

export function FormationRenderer({ formation, onWidgetClick }) {
  if (!formation || !formation.widgets?.length) {
    return null;
  }

  const { mode, grid, widgets } = formation;

  // Comparison mode: pass all widgets to ComparisonTemplate
  if (mode === 'comparison' || mode === FormationMode.COMPARISON) {
    return (
      <div className="formation-comparison">
        <ComparisonTemplate widgets={widgets} onWidgetClick={onWidgetClick} />
      </div>
    );
  }

  return (
    <WidgetList
      mode={mode}
      cols={grid?.cols || 2}
      widgets={widgets}
      onWidgetClick={onWidgetClick}
    />
  );
}

function WidgetList({ mode, cols, widgets, onWidgetClick }) {
  const [visibleCount, setVisibleCount] = useState(BATCH_SIZE);
  const sentinelRef = useRef(null);

  // Reset visible count when widgets change (new search)
  useEffect(() => {
    setVisibleCount(BATCH_SIZE);
  }, [widgets]);

  // IntersectionObserver for lazy loading
  useEffect(() => {
    if (visibleCount >= widgets.length || !sentinelRef.current) return;

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setVisibleCount((prev) => Math.min(prev + BATCH_SIZE, widgets.length));
        }
      },
      { threshold: 0.1 }
    );

    observer.observe(sentinelRef.current);
    return () => observer.disconnect();
  }, [widgets.length, visibleCount]);

  const layoutClass = getLayoutClass(mode, cols);
  const visibleWidgets = widgets.slice(0, visibleCount);
  const hasMore = visibleCount < widgets.length;

  return (
    <div className="formation-wrapper">
      {widgets.length > 1 && (
        <div className="formation-status">
          {hasMore
            ? `${visibleCount} из ${widgets.length}`
            : `${widgets.length} товаров`}
        </div>
      )}
      <div className={layoutClass}>
        {visibleWidgets.map((widget) => (
          <WidgetRenderer
            key={widget.id}
            widget={widget}
            onClick={onWidgetClick}
          />
        ))}
        {hasMore && (
          <div ref={sentinelRef} className="formation-sentinel" />
        )}
      </div>
    </div>
  );
}

function getLayoutClass(mode, cols) {
  switch (mode) {
    case FormationMode.GRID:
    case 'grid':
      return `formation-grid cols-${cols}`;

    case FormationMode.CAROUSEL:
    case 'carousel':
      return 'formation-carousel';

    case FormationMode.SINGLE:
    case 'single':
      return 'formation-single';

    case FormationMode.LIST:
    case 'list':
    default:
      return 'formation-list';
  }
}

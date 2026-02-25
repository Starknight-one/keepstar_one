import { useState, useEffect, useRef } from 'react';
import { FormationMode } from './formationModel';
import { WidgetRenderer } from '../widget/WidgetRenderer';
import { ComparisonTemplate } from '../widget/templates/ComparisonTemplate';
import './Formation.css';

const BATCH_SIZE = 12;

export function FormationRenderer({ formation, onWidgetClick, onLoadMore }) {
  if (!formation || !formation.widgets?.length) {
    return null;
  }

  const { mode, grid, widgets, sections, pagination } = formation;

  // Composed formation: render each section separately
  if (sections?.length > 0) {
    return (
      <div className="formation-composed">
        {sections.map((section, i) => (
          <div key={i} className="formation-section">
            {section.label && (
              <div className="formation-section-label">{section.label}</div>
            )}
            <FormationRenderer
              formation={{
                mode: section.mode,
                grid: section.grid,
                widgets: section.widgets,
              }}
              onWidgetClick={onWidgetClick}
            />
          </div>
        ))}
      </div>
    );
  }

  // Comparison mode: pass all widgets to ComparisonTemplate
  if (mode === 'comparison' || mode === FormationMode.COMPARISON) {
    return (
      <div className="formation-comparison">
        <ComparisonTemplate widgets={widgets} onWidgetClick={onWidgetClick} />
      </div>
    );
  }

  // Table mode: render via ComparisonTemplate with table styling
  if (mode === 'table' || mode === FormationMode.TABLE) {
    return (
      <div className="formation-table">
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
      pagination={pagination}
      onLoadMore={onLoadMore}
    />
  );
}

function WidgetList({ mode, cols, widgets, onWidgetClick, pagination, onLoadMore }) {
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

  // Server-side pagination: call onLoadMore when all local widgets visible
  const hasServerMore = pagination?.hasMore;
  useEffect(() => {
    if (!hasMore && hasServerMore && onLoadMore) {
      onLoadMore(pagination.offset + pagination.limit);
    }
  }, [hasMore, hasServerMore, onLoadMore, pagination]);

  // Status text
  const total = pagination?.total || widgets.length;
  const statusText = hasMore
    ? `${visibleCount} из ${total}`
    : pagination
      ? `${widgets.length} из ${total} товаров`
      : `${widgets.length} товаров`;

  return (
    <div className="formation-wrapper">
      {total > 1 && (
        <div className="formation-status">
          {statusText}
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

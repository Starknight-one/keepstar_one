import { FormationMode } from './formationModel';
import { WidgetRenderer } from './WidgetRenderer';
import { ComparisonTemplate } from './ComparisonTemplate';
import './Formation.css';

/**
 * FormationRenderer — read-only replay version.
 * No onWidgetClick, no onLoadMore, no lazy loading.
 * Shows ALL widgets at once.
 */
export function FormationRenderer({ formation }) {
  if (!formation || (!formation.widgets?.length && !formation.sections?.length)) {
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
        <ComparisonTemplate widgets={widgets} />
      </div>
    );
  }

  // Table mode: render via ComparisonTemplate with table styling
  if (mode === 'table' || mode === FormationMode.TABLE) {
    return (
      <div className="formation-table">
        <ComparisonTemplate widgets={widgets} />
      </div>
    );
  }

  const total = pagination?.total || widgets.length;
  const cols = grid?.cols || 2;
  const layoutClass = getLayoutClass(mode, cols);

  return (
    <div className="formation-wrapper">
      {total > 1 && (
        <div className="replay-status-pill">{total} items</div>
      )}
      <div className={layoutClass}>
        {widgets.map((widget) => (
          <WidgetRenderer
            key={widget.id}
            widget={widget}
          />
        ))}
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

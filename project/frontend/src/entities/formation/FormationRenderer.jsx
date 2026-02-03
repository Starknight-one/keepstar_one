import { FormationMode } from './formationModel';
import { WidgetRenderer } from '../widget/WidgetRenderer';
import './Formation.css';

export function FormationRenderer({ formation, onWidgetClick }) {
  if (!formation || !formation.widgets?.length) {
    return null;
  }

  const { mode, grid, widgets } = formation;
  const cols = grid?.cols || 2;

  const layoutClass = getLayoutClass(mode, cols);

  return (
    <div className={layoutClass}>
      {widgets.map((widget) => (
        <WidgetRenderer
          key={widget.id}
          widget={widget}
          onClick={onWidgetClick}
        />
      ))}
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

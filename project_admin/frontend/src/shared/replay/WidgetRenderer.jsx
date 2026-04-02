import { WidgetType } from './widgetModel';
import { GenericCardV2Template } from './GenericCardV2Template';
import './Widget.css';

/**
 * WidgetRenderer — read-only replay version.
 * No onClick, no clickable wrapper, no mouse tracking.
 */
export function WidgetRenderer({ widget }) {
  // Template-based rendering (new system)
  if (widget.template) {
    const content = renderTemplate(widget);
    const placeClass = widget.meta?.place ? `widget-place-${widget.meta.place}` : '';

    if (placeClass) {
      return <div className={placeClass}>{content}</div>;
    }
    return content;
  }

  // Legacy type-based rendering — minimal fallback
  const sizeClass = widget.size ? `size-${widget.size}` : 'size-medium';
  return (
    <div className={`widget ${sizeClass}`}>
      {widget.atoms?.map((atom, i) => (
        <span key={i}>{String(atom.value ?? '')}</span>
      ))}
    </div>
  );
}

function renderTemplate(widget) {
  const entityRef = widget.entityRef || null;

  // V2 routing: if widget has layout or atomsV2, use v2 renderer
  if (widget.layout || widget.atomsV2) {
    return (
      <GenericCardV2Template
        atomsV2={widget.atomsV2 || []}
        layout={widget.layout}
        size={widget.size}
        direction={widget.meta?.direction}
        entityRef={entityRef}
        states={widget.states}
      />
    );
  }

  // Fallback for any template name — route through V2
  return (
    <GenericCardV2Template
      atomsV2={widget.atomsV2 || widget.atoms || []}
      layout={widget.layout}
      size={widget.size}
      direction={widget.meta?.direction}
      entityRef={entityRef}
      states={widget.states}
    />
  );
}

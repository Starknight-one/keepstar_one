import { useRef } from 'react';
import { WidgetType } from './widgetModel';
import { GenericCardV2Template } from './templates/GenericCardV2Template';
import './Widget.css';

export function WidgetRenderer({ widget, onClick }) {
  const mouseDownPos = useRef(null);

  // Template-based rendering (new system)
  if (widget.template) {
    const content = renderTemplate(widget);
    const placeClass = widget.meta?.place ? `widget-place-${widget.meta.place}` : '';

    // Make widget clickable if it has entityRef and onClick handler
    if (onClick && widget.entityRef) {
      const handleMouseDown = (e) => {
        mouseDownPos.current = { x: e.clientX, y: e.clientY };
      };
      const handleClick = (e) => {
        // Don't navigate if user selected text
        const selection = window.getSelection();
        if (selection && selection.toString().length > 0) return;
        // Don't navigate if user dragged (mouse moved significantly)
        if (mouseDownPos.current) {
          const dx = Math.abs(e.clientX - mouseDownPos.current.x);
          const dy = Math.abs(e.clientY - mouseDownPos.current.y);
          if (dx > 5 || dy > 5) return;
        }
        onClick(widget.entityRef.type, widget.entityRef.id);
      };
      return (
        <div
          className={`widget-clickable ${placeClass}`.trim()}
          onMouseDown={handleMouseDown}
          onClick={handleClick}
        >
          {content}
        </div>
      );
    }

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

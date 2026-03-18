import { WidgetType } from './widgetModel';
import { AtomRenderer } from '../atom/AtomRenderer';
import { ProductCardTemplate, ServiceCardTemplate, ProductDetailTemplate, ServiceDetailTemplate, GenericCardTemplate } from './templates';
import { GenericCardV2Template } from './templates/GenericCardV2Template';
import './Widget.css';

export function WidgetRenderer({ widget, onClick }) {
  // Template-based rendering (new system)
  if (widget.template) {
    const content = renderTemplate(widget);
    const placeClass = widget.meta?.place ? `widget-place-${widget.meta.place}` : '';

    // Make widget clickable if it has entityRef and onClick handler
    if (onClick && widget.entityRef) {
      const handleClick = () => {
        onClick(widget.entityRef.type, widget.entityRef.id);
      };
      return (
        <div className={`widget-clickable ${placeClass}`.trim()} onClick={handleClick}>
          {content}
        </div>
      );
    }

    if (placeClass) {
      return <div className={placeClass}>{content}</div>;
    }
    return content;
  }

  // Legacy type-based rendering (backward compatibility)
  const sizeClass = widget.size ? `size-${widget.size}` : 'size-medium';

  switch (widget.type) {
    case WidgetType.PRODUCT_CARD:
      return <ProductCard widget={widget} sizeClass={sizeClass} />;

    case WidgetType.TEXT_BLOCK:
      return <TextBlock widget={widget} sizeClass={sizeClass} />;

    case WidgetType.QUICK_REPLIES:
      return <QuickReplies widget={widget} />;

    default:
      return <DefaultWidget widget={widget} sizeClass={sizeClass} />;
  }
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

  switch (widget.template) {
    case 'GenericCard':
      return <GenericCardTemplate atoms={widget.atoms} zones={widget.zones} size={widget.size} direction={widget.meta?.direction} entityRef={entityRef} />;
    case 'ProductCard':
      return <ProductCardTemplate atoms={widget.atoms} size={widget.size} entityRef={entityRef} />;
    case 'ServiceCard':
      return <ServiceCardTemplate atoms={widget.atoms} size={widget.size} entityRef={entityRef} />;
    case 'ProductDetail':
      return <ProductDetailTemplate atoms={widget.atoms} entityRef={entityRef} />;
    case 'ServiceDetail':
      return <ServiceDetailTemplate atoms={widget.atoms} entityRef={entityRef} />;
    default:
      return <DefaultWidget widget={widget} sizeClass="size-medium" />;
  }
}

function ProductCard({ widget, sizeClass }) {
  return (
    <div className={`widget widget-product-card ${sizeClass}`}>
      {widget.atoms.map((atom, i) => (
        <AtomRenderer key={i} atom={atom} />
      ))}
    </div>
  );
}

function TextBlock({ widget, sizeClass }) {
  return (
    <div className={`widget widget-text-block ${sizeClass}`}>
      {widget.atoms.map((atom, i) => (
        <AtomRenderer key={i} atom={atom} />
      ))}
    </div>
  );
}

function QuickReplies({ widget }) {
  return (
    <div className="widget-quick-replies">
      {widget.atoms.map((atom, i) => (
        <AtomRenderer key={i} atom={atom} />
      ))}
    </div>
  );
}

function DefaultWidget({ widget, sizeClass }) {
  return (
    <div className={`widget ${sizeClass}`}>
      {widget.atoms?.map((atom, i) => (
        <AtomRenderer key={i} atom={atom} />
      ))}
    </div>
  );
}

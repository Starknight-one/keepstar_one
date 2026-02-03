import { WidgetType } from './widgetModel';
import { AtomRenderer } from '../atom/AtomRenderer';
import { ProductCardTemplate, ServiceCardTemplate, ProductDetailTemplate, ServiceDetailTemplate } from './templates';
import './Widget.css';

export function WidgetRenderer({ widget, onClick }) {
  // Template-based rendering (new system)
  if (widget.template) {
    const content = renderTemplate(widget);

    // Make widget clickable if it has entityRef and onClick handler
    if (onClick && widget.entityRef) {
      const handleClick = () => {
        onClick(widget.entityRef.type, widget.entityRef.id);
      };
      return (
        <div className="widget-clickable" onClick={handleClick}>
          {content}
        </div>
      );
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
  switch (widget.template) {
    case 'ProductCard':
      return <ProductCardTemplate atoms={widget.atoms} size={widget.size} />;
    case 'ServiceCard':
      return <ServiceCardTemplate atoms={widget.atoms} size={widget.size} />;
    case 'ProductDetail':
      return <ProductDetailTemplate atoms={widget.atoms} />;
    case 'ServiceDetail':
      return <ServiceDetailTemplate atoms={widget.atoms} />;
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

import { WidgetType } from './widgetModel';
import { AtomRenderer } from '../atom/AtomRenderer';
import './Widget.css';

export function WidgetRenderer({ widget }) {
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

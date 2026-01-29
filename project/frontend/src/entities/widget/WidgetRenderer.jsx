import { WidgetType } from './widgetModel';
import { AtomRenderer } from '../atom/AtomRenderer';

// Renders any widget based on its type
export function WidgetRenderer({ widget }) {
  switch (widget.type) {
    case WidgetType.PRODUCT_CARD:
      return <ProductCard widget={widget} />;

    case WidgetType.TEXT_BLOCK:
      return <TextBlock widget={widget} />;

    case WidgetType.QUICK_REPLIES:
      return <QuickReplies widget={widget} />;

    default:
      return <DefaultWidget widget={widget} />;
  }
}

function ProductCard({ widget }) {
  return (
    <div className="widget-product-card">
      {widget.atoms.map((atom, i) => (
        <AtomRenderer key={i} atom={atom} />
      ))}
    </div>
  );
}

function TextBlock({ widget }) {
  return (
    <div className="widget-text-block">
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

function DefaultWidget({ widget }) {
  return (
    <div className="widget-default">
      {widget.atoms?.map((atom, i) => (
        <AtomRenderer key={i} atom={atom} />
      ))}
    </div>
  );
}

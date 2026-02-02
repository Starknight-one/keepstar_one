import { MessageRole } from './messageModel';
import { FormationRenderer } from '../formation/FormationRenderer';
import { WidgetRenderer } from '../widget/WidgetRenderer';

export function MessageBubble({ message, hideFormation }) {
  const isUser = message.role === MessageRole.USER;

  return (
    <div className={`message-bubble ${isUser ? 'user' : 'assistant'}`}>
      {message.content && (
        <div className="message-content">{message.content}</div>
      )}

      {/* Legacy widgets support (without formation) */}
      {!hideFormation && message.widgets?.length > 0 && !message.formation && (
        <div className={`message-widgets formation-${message.formationType || 'list'}`}>
          {message.widgets.map((widget) => (
            <WidgetRenderer key={widget.id} widget={widget} />
          ))}
        </div>
      )}

      {/* New formation support */}
      {!hideFormation && message.formation && (
        <FormationRenderer formation={message.formation} />
      )}
    </div>
  );
}

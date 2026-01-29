import { MessageRole } from './messageModel';
import { WidgetRenderer } from '../widget/WidgetRenderer';

export function MessageBubble({ message }) {
  const isUser = message.role === MessageRole.USER;

  return (
    <div className={`message-bubble ${isUser ? 'user' : 'assistant'}`}>
      {message.content && (
        <div className="message-content">{message.content}</div>
      )}

      {message.widgets?.length > 0 && (
        <div className={`message-widgets formation-${message.formation?.type || 'list'}`}>
          {message.widgets.map((widget) => (
            <WidgetRenderer key={widget.id} widget={widget} />
          ))}
        </div>
      )}
    </div>
  );
}

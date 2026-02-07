import { MessageRole } from './messageModel';
import { FormationRenderer } from '../formation/FormationRenderer';
import { WidgetRenderer } from '../widget/WidgetRenderer';

export function MessageBubble({ message, hideFormation }) {
  const isUser = message.role === MessageRole.USER;

  // Text-only formations (e.g. "nothing found") should always show inline in the chat bubble
  const isTextOnly = message.formation?.widgets?.every(w => w.type === 'text_block');
  const shouldHideFormation = hideFormation && !isTextOnly;

  return (
    <div className={`message-bubble ${isUser ? 'user' : 'assistant'}`}>
      {message.content && (
        <div className="message-content">{message.content}</div>
      )}

      {/* Legacy widgets support (without formation) */}
      {!shouldHideFormation && message.widgets?.length > 0 && !message.formation && (
        <div className={`message-widgets formation-${message.formationType || 'list'}`}>
          {message.widgets.map((widget) => (
            <WidgetRenderer key={widget.id} widget={widget} />
          ))}
        </div>
      )}

      {/* New formation support */}
      {!shouldHideFormation && message.formation && (
        <FormationRenderer formation={message.formation} />
      )}
    </div>
  );
}

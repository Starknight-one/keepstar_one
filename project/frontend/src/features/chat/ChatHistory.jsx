import { MessageBubble } from '../../entities/message/MessageBubble';

export function ChatHistory({ messages, isLoading, hideFormation }) {
  return (
    <div className="chat-history">
      {messages.map((message) => (
        <MessageBubble key={message.id} message={message} hideFormation={hideFormation} />
      ))}
      {isLoading && <div className="chat-loading">Thinking...</div>}
    </div>
  );
}

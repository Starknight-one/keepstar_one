import { MessageBubble } from '../../entities/message/MessageBubble';

export function ChatHistory({ messages, isLoading }) {
  return (
    <div className="chat-history">
      {messages.map((message) => (
        <MessageBubble key={message.id} message={message} />
      ))}
      {isLoading && <div className="chat-loading">Thinking...</div>}
    </div>
  );
}

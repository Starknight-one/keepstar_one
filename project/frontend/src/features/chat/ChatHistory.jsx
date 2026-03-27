import { useEffect, useRef } from 'react';
import { MessageBubble } from '../../entities/message/MessageBubble';
import { TypingIndicator } from '../../shared/ui';

export function ChatHistory({ messages, isLoading, hideFormation }) {
  const bottomRef = useRef(null);

  // Auto-scroll to bottom when messages change
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, isLoading]);

  return (
    <div className="chat-history">
      <div className="chat-history-spacer" />
      {messages.map((message) => (
        <MessageBubble key={message.id} message={message} hideFormation={hideFormation} />
      ))}
      {isLoading && <TypingIndicator />}
      <div ref={bottomRef} />
    </div>
  );
}

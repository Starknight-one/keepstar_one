import { useChatMessages } from './useChatMessages';
import { useChatSubmit } from './useChatSubmit';
import { ChatHistory } from './ChatHistory';
import { ChatInput } from './ChatInput';

export function ChatPanel() {
  const chat = useChatMessages();
  const { submit } = useChatSubmit(chat);

  return (
    <div className="chat-panel">
      <ChatHistory messages={chat.messages} isLoading={chat.isLoading} />
      <ChatInput onSubmit={submit} disabled={chat.isLoading} />
      {chat.error && <div className="chat-error">{chat.error}</div>}
    </div>
  );
}

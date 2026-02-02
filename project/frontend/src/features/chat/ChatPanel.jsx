import { useEffect } from 'react';
import { useChatMessages } from './useChatMessages';
import { useChatSubmit } from './useChatSubmit';
import { ChatHistory } from './ChatHistory';
import { ChatInput } from './ChatInput';
import { getSession } from '../../shared/api/apiClient';
import { MessageRole } from '../../entities/message/messageModel';
import './ChatPanel.css';

const SESSION_STORAGE_KEY = 'chatSessionId';

export function ChatPanel({ onClose }) {
  const {
    sessionId,
    messages,
    isLoading,
    error,
    addMessage,
    setMessages,
    setLoading,
    setError,
    setSessionId
  } = useChatMessages();

  const { submit } = useChatSubmit({
    sessionId,
    addMessage,
    setLoading,
    setError,
    setSessionId
  });

  // Load session history on mount
  useEffect(() => {
    const loadSession = async () => {
      const savedSessionId = localStorage.getItem(SESSION_STORAGE_KEY);
      if (!savedSessionId) return;

      setLoading(true);
      try {
        const session = await getSession(savedSessionId);

        if (session && session.status === 'active') {
          setSessionId(session.id);

          // Convert messages to frontend format
          const loadedMessages = session.messages.map((msg) => ({
            id: msg.id,
            role: msg.role === 'user' ? MessageRole.USER : MessageRole.ASSISTANT,
            content: msg.content,
            timestamp: new Date(msg.sentAt),
          }));

          setMessages(loadedMessages);
        } else {
          // Session expired or not found, clear localStorage
          localStorage.removeItem(SESSION_STORAGE_KEY);
        }
      } catch (err) {
        console.error('Failed to load session:', err);
        localStorage.removeItem(SESSION_STORAGE_KEY);
      } finally {
        setLoading(false);
      }
    };

    loadSession();
  }, [setLoading, setSessionId, setMessages]);

  return (
    <div className="chat-container">
      <div className="chat-header">
        <h3>Chat</h3>
        <button className="close-btn" onClick={onClose}>✕</button>
      </div>

      <ChatHistory messages={messages} isLoading={isLoading} />
      <ChatInput onSubmit={submit} disabled={isLoading} />
      {error && <div className="chat-error">{error}</div>}
    </div>
  );
}

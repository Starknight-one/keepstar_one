import { useEffect, useState, useCallback, useRef } from 'react';
import { useChatMessages } from './useChatMessages';
import { useChatSubmit } from './useChatSubmit';
import { ChatHistory } from './ChatHistory';
import { ChatInput } from './ChatInput';
import { expandView, goBack, getSession, initSession } from '../../shared/api/apiClient';
import { saveSessionCache, loadSessionCache, clearSessionCache } from './sessionCache';
import { MessageRole } from '../../entities/message/messageModel';
import './ChatPanel.css';

export function ChatPanel({ onClose, onFormationReceived, onNavigationStateChange, hideFormation }) {
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

  const [canGoBack, setCanGoBack] = useState(false);
  const lastFormationRef = useRef(null);

  const { submit } = useChatSubmit({
    sessionId,
    addMessage,
    setLoading,
    setError,
    setSessionId,
    onFormationReceived: useCallback((formation) => {
      lastFormationRef.current = formation;
      onFormationReceived?.(formation);
    }, [onFormationReceived]),
  });

  // Navigation handlers
  const handleExpand = useCallback(async (entityType, entityId) => {
    if (!sessionId) return;
    try {
      const result = await expandView(sessionId, entityType, entityId);
      if (result.formation) {
        onFormationReceived?.(result.formation);
      }
      setCanGoBack(result.stackSize > 0);
    } catch (err) {
      console.error('Expand failed:', err);
    }
  }, [sessionId, onFormationReceived]);

  const handleBack = useCallback(async () => {
    if (!sessionId) return;
    try {
      const result = await goBack(sessionId);
      if (result.formation) {
        onFormationReceived?.(result.formation);
      }
      setCanGoBack(result.canGoBack);
    } catch (err) {
      console.error('Back navigation failed:', err);
    }
  }, [sessionId, onFormationReceived]);

  // Expose navigation functions to parent
  useEffect(() => {
    onNavigationStateChange?.({
      canGoBack,
      onExpand: handleExpand,
      onBack: handleBack,
    });
  }, [canGoBack, handleExpand, handleBack, onNavigationStateChange]);

  // Restore session from browser cache instantly, or init new session
  useEffect(() => {
    const cached = loadSessionCache();
    if (cached) {
      setSessionId(cached.sessionId);
      if (cached.messages?.length > 0) {
        setMessages(cached.messages);
      }
      if (cached.formation) {
        lastFormationRef.current = cached.formation;
        onFormationReceived?.(cached.formation);
      }

      // Async validate — if session is dead on backend, clear everything
      getSession(cached.sessionId).then(session => {
        if (!session || session.status !== 'active') {
          clearSessionCache();
          setSessionId(null);
          setMessages([]);
          lastFormationRef.current = null;
          onFormationReceived?.(null);
        }
      }).catch(() => {
        // Network error — keep cache, don't block
      });
      return;
    }

    // No cached session — init a new one with greeting
    initSession().then(data => {
      setSessionId(data.sessionId);
      addMessage({
        id: 'greeting',
        role: MessageRole.ASSISTANT,
        content: data.greeting,
        timestamp: new Date(),
      });
    }).catch(() => {
      // Init failed — chat still works, session will be created on first query
    });
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Persist session cache after messages change
  useEffect(() => {
    if (sessionId && messages.length > 0) {
      saveSessionCache({ sessionId, messages, formation: lastFormationRef.current });
    }
  }, [sessionId, messages]);

  return (
    <div className="chat-container">
      <div className="chat-header">
        <h3>Chat</h3>
        <button className="close-btn" onClick={onClose}>✕</button>
      </div>

      <ChatHistory messages={messages} isLoading={isLoading} hideFormation={hideFormation} />
      <ChatInput onSubmit={submit} disabled={isLoading} />
      {error && <div className="chat-error">{error}</div>}
    </div>
  );
}

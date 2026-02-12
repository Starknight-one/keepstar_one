import { useEffect, useCallback, useRef } from 'react';
import { useChatMessages } from './useChatMessages';
import { useChatSubmit } from './useChatSubmit';
import { ChatHistory } from './ChatHistory';
import { ChatInput } from './ChatInput';
import { useFormationStack } from './model/useFormationStack';
import { syncExpand, syncBack } from './api/backgroundSync';
import { expandView, getSession, initSession } from '../../shared/api/apiClient';
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

  const lastFormationRef = useRef(null);
  const adjacentFormationsRef = useRef(null);

  // Formation stack for instant back navigation
  // Destructure to get stable function refs (push/pop/clear have [] deps)
  const { push: stackPush, pop: stackPop, clear: stackClear, canGoBack, stack: formationStackArray } = useFormationStack();

  const { submit } = useChatSubmit({
    sessionId,
    addMessage,
    setLoading,
    setError,
    setSessionId,
    onFormationReceived: useCallback((formation, adjacentFormations) => {
      // Push current formation to stack before replacing (new branch of decision tree)
      if (lastFormationRef.current) {
        stackPush(lastFormationRef.current);
      }
      lastFormationRef.current = formation;
      // Store adjacent formations for instant expand (Phase 2)
      adjacentFormationsRef.current = adjacentFormations || null;
      onFormationReceived?.(formation);
    }, [onFormationReceived, stackPush]),
  });

  // Navigation handlers
  const handleExpand = useCallback(async (entityType, entityId) => {
    if (!sessionId) return;

    // Phase 2: Check adjacentFormations cache for instant expand
    const key = `${entityType}:${entityId}`;
    const cached = adjacentFormationsRef.current?.[key];
    if (cached) {
      // Instant: push current formation, render cached detail
      stackPush(lastFormationRef.current);
      lastFormationRef.current = cached;
      onFormationReceived?.(cached);
      // Fire-and-forget sync to keep backend in sync
      syncExpand(sessionId, entityType, entityId);
      return;
    }

    // Fallback: API call (no cached adjacent formation)
    stackPush(lastFormationRef.current);
    try {
      const result = await expandView(sessionId, entityType, entityId);
      if (result.formation) {
        lastFormationRef.current = result.formation;
        onFormationReceived?.(result.formation);
      }
    } catch (err) {
      // Rollback: pop from stack on failure
      stackPop();
      console.error('Expand failed:', err);
    }
  }, [sessionId, onFormationReceived, stackPush, stackPop]);

  const handleBack = useCallback(() => {
    if (!canGoBack) return;
    // Instant: pop from stack, no await
    const prev = stackPop();
    if (prev) {
      lastFormationRef.current = prev;
      onFormationReceived?.(prev);
    }
    // Fire-and-forget sync to keep backend in sync
    if (sessionId) {
      syncBack(sessionId);
    }
  }, [sessionId, onFormationReceived, canGoBack, stackPop]);

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
      // Restore formation stack from cache
      if (cached.formationStack?.length > 0) {
        cached.formationStack.forEach((f) => stackPush(f));
      }

      // Async validate — if session is dead on backend, clear everything
      getSession(cached.sessionId).then(session => {
        if (!session || session.status !== 'active') {
          clearSessionCache();
          setSessionId(null);
          setMessages([]);
          lastFormationRef.current = null;
          stackClear();
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
      saveSessionCache({
        sessionId,
        messages,
        formation: lastFormationRef.current,
        formationStack: formationStackArray,
      });
    }
  }, [sessionId, messages, formationStackArray]);

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

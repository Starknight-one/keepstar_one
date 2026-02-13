import { useEffect, useCallback, useRef } from 'react';
import { useChatMessages } from './useChatMessages';
import { useChatSubmit } from './useChatSubmit';
import { ChatHistory } from './ChatHistory';
import { ChatInput } from './ChatInput';
import { useFormationHistory } from '../navigation/useFormationHistory';
import { Stepper } from '../navigation/Stepper';
import { fillFormation } from './model/fillFormation';
import { syncExpand, syncBack } from './api/backgroundSync';
import { expandView, getSession, initSession } from '../../shared/api/apiClient';
import { log } from '../../shared/logger';
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
  const adjacentTemplatesRef = useRef(null);
  const entitiesRef = useRef(null);
  const lastQueryRef = useRef('');

  // Formation history \u2014 chronological trail, always appends, never goes backwards
  const {
    history: formationHistory,
    currentIndex: historyIndex,
    push: historyPush,
    clear: historyClear,
    canGoBack,
  } = useFormationHistory();

  // Keep a ref to always have fresh history for closures
  const historyRef = useRef(formationHistory);
  historyRef.current = formationHistory;

  const { submit: rawSubmit } = useChatSubmit({
    sessionId,
    addMessage,
    setLoading,
    setError,
    setSessionId,
    onFormationReceived: useCallback((formation, adjacentTemplates, entities) => {
      // Use ref \u2014 always has the latest query text, avoids stale closure
      const label = lastQueryRef.current || 'Query';
      // Push new formation into history (trims forward entries if in middle)
      historyPush(formation, label);
      lastFormationRef.current = formation;
      // Store adjacent templates + entities for instant expand
      adjacentTemplatesRef.current = adjacentTemplates || null;
      entitiesRef.current = entities || null;
      onFormationReceived?.(formation);
    }, [onFormationReceived, historyPush]),
  });

  // Wrap submit to capture query text in ref before sending
  const submit = useCallback((text) => {
    lastQueryRef.current = text;
    rawSubmit(text);
  }, [rawSubmit]);

  // Navigation handlers
  const handleExpand = useCallback(async (entityType, entityId) => {
    if (!sessionId) return;

    // Instant path: fill template with entity data on the client
    const template = adjacentTemplatesRef.current?.[entityType];
    const entitiesData = entitiesRef.current;
    if (template && entitiesData) {
      const list = entityType === 'product' ? entitiesData.products : entitiesData.services;
      const entity = list?.find(e => e.id === entityId);
      if (entity) {
        const filled = fillFormation(template, entity, entityType);
        if (filled) {
          const label = `\u2192 ${entity.name || entityType}`;
          historyPush(filled, label);
          lastFormationRef.current = filled;
          onFormationReceived?.(filled);
          // Fire-and-forget sync to keep backend in sync
          syncExpand(sessionId, entityType, entityId);
          return;
        }
      }
    }

    // Fallback: API call (no template or entity not found)
    try {
      const result = await expandView(sessionId, entityType, entityId);
      if (result.formation) {
        const label = `\u2192 ${entityType}`;
        historyPush(result.formation, label);
        lastFormationRef.current = result.formation;
        onFormationReceived?.(result.formation);
      }
    } catch (err) {
      log.error('Expand failed:', err);
    }
  }, [sessionId, onFormationReceived, historyPush]);

  const handleBack = useCallback(() => {
    if (!canGoBack) return;
    // Trail model: "back" = push previous formation as a NEW step
    const h = historyRef.current;
    const prevEntry = h[h.length - 2];
    if (prevEntry) {
      const stepNum = String(h.length - 1).padStart(2, '0');
      const label = `\u2190 ${stepNum} ${prevEntry.label}`;
      historyPush(prevEntry.formation, label);
      lastFormationRef.current = prevEntry.formation;
      onFormationReceived?.(prevEntry.formation);
    }
    // Fire-and-forget sync to keep backend in sync
    if (sessionId) {
      syncBack(sessionId);
    }
  }, [sessionId, onFormationReceived, canGoBack, historyPush]);

  // Stepper goTo handler \u2014 trail model: clicking a past step = push it as a new step
  const handleStepperGoTo = useCallback((index) => {
    const entry = historyRef.current[index];
    if (!entry) return;
    if (index === historyRef.current.length - 1) return;
    const stepNum = String(index + 1).padStart(2, '0');
    historyPush(entry.formation, `\u2190 ${stepNum} ${entry.label}`);
    lastFormationRef.current = entry.formation;
    onFormationReceived?.(entry.formation);
  }, [historyPush, onFormationReceived]);

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
      // Restore formation history from cache (clear first to avoid HMR duplication)
      if (cached.formationHistory?.length > 0) {
        historyClear();
        cached.formationHistory.forEach((entry) => historyPush(entry.formation, entry.label));
      }
      // Restore adjacent templates + entities for instant expand after F5
      adjacentTemplatesRef.current = cached.adjacentTemplates || null;
      entitiesRef.current = cached.entities || null;

      // Async validate \u2014 if session is dead on backend, clear everything
      getSession(cached.sessionId).then(session => {
        if (!session || session.status !== 'active') {
          clearSessionCache();
          setSessionId(null);
          setMessages([]);
          lastFormationRef.current = null;
          adjacentTemplatesRef.current = null;
          entitiesRef.current = null;
          historyClear();
          onFormationReceived?.(null);
        }
      }).catch((err) => {
        log.warn('Session validation failed:', err);
      });
      return;
    }

    // No cached session \u2014 init a new one with greeting
    initSession().then(data => {
      setSessionId(data.sessionId);
      addMessage({
        id: 'greeting',
        role: MessageRole.ASSISTANT,
        content: data.greeting,
        timestamp: new Date(),
      });
    }).catch((err) => {
      log.warn('Session init failed:', err);
    });
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Persist session cache after messages change
  useEffect(() => {
    if (sessionId && messages.length > 0) {
      saveSessionCache({
        sessionId,
        messages,
        formation: lastFormationRef.current,
        formationHistory,
        historyIndex,
        adjacentTemplates: adjacentTemplatesRef.current,
        entities: entitiesRef.current,
      });
    }
  }, [sessionId, messages, formationHistory, historyIndex]);

  return (
    <div className="chat-container">
      <div className="chat-header">
        <button className="gradient-circle-btn" onClick={onClose} aria-label="Close chat">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M18 6 6 18" /><path d="M6 6 18 18" />
          </svg>
        </button>
      </div>
      <div className="chat-spacer" />
      <ChatHistory messages={messages} isLoading={isLoading} hideFormation={hideFormation} />
      <ChatInput onSubmit={submit} disabled={isLoading} />
      <Stepper history={formationHistory} currentIndex={historyIndex} goTo={handleStepperGoTo} />
      <div className="chat-spacer" />
      {error && <div className="chat-error">{error}</div>}
    </div>
  );
}

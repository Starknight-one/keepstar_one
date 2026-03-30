import { useCallback, useRef, useEffect } from 'react';
import { sendPipelineQuery } from '../../shared/api/apiClient';
import { MessageRole } from '../../entities/message/messageModel';

const SESSION_STORAGE_KEY = 'chatSessionId';

// Helper for Russian word declension
function getProductWord(count) {
  const lastTwo = count % 100;
  const lastOne = count % 10;
  if (lastTwo >= 11 && lastTwo <= 19) return 'товаров';
  if (lastOne === 1) return 'товар';
  if (lastOne >= 2 && lastOne <= 4) return 'товара';
  return 'товаров';
}

export function useChatSubmit({ sessionId, addMessage, setLoading, setError, setSessionId, onFormationReceived, lastFormationRef }) {
  // useRef to avoid stale closure — callback doesn't depend on sessionId re-renders
  const sessionIdRef = useRef(sessionId);
  useEffect(() => { sessionIdRef.current = sessionId; }, [sessionId]);

  // Request versioning: monotonic counter prevents stale responses from overwriting fresh state
  const requestIdRef = useRef(0);
  // AbortController for cancelling in-flight requests when a new one is submitted
  const abortRef = useRef(null);

  const submit = useCallback(async (text) => {
    if (!text.trim()) return;

    // Abort any in-flight request — new request always wins
    if (abortRef.current) {
      abortRef.current.abort();
    }

    const thisRequestId = ++requestIdRef.current;
    const controller = new AbortController();
    abortRef.current = controller;

    // Add user message
    addMessage({
      id: Date.now().toString(),
      role: MessageRole.USER,
      content: text,
      timestamp: new Date(),
    });

    setLoading(true);
    setError(null);

    try {
      // Build screen context from current formation for Agent2
      let screenContext = null;
      const currentFormation = lastFormationRef?.current;
      if (currentFormation) {
        const w0 = currentFormation.widgets?.[0];
        screenContext = {
          mode: currentFormation.mode || 'grid',
          widgetCount: currentFormation.widgets?.length || 0,
          fields: w0?.atoms?.map(a => a.fieldName).filter(Boolean) || [],
        };
      }

      const response = await sendPipelineQuery(sessionIdRef.current, text, screenContext, controller.signal);

      // Stale response guard: discard if a newer request was issued
      if (thisRequestId !== requestIdRef.current) return;

      // Save sessionId to localStorage if new
      if (response.sessionId && response.sessionId !== sessionIdRef.current) {
        sessionIdRef.current = response.sessionId;
        localStorage.setItem(SESSION_STORAGE_KEY, response.sessionId);
        setSessionId(response.sessionId);
      }

      // Notify parent about formation + adjacent templates + entities for instant expand
      if (response.formation && onFormationReceived) {
        onFormationReceived(response.formation, response.adjacentTemplates || null, response.entities || null);
      }

      // Add assistant message with formation
      const widgets = response.formation?.widgets || [];
      const widgetCount = widgets.length;
      const isTextOnly = widgets.every(w => w.type === 'text_block');
      addMessage({
        id: (Date.now() + 1).toString(),
        role: MessageRole.ASSISTANT,
        content: (widgetCount > 0 && !isTextOnly) ? `Нашёл ${widgetCount} ${getProductWord(widgetCount)}` : '',
        formation: response.formation,
        timestamp: new Date(),
      });
    } catch (err) {
      if (err.name === 'AbortError') return; // Request was cancelled by a newer one
      if (thisRequestId !== requestIdRef.current) return; // Stale error

      setError(err.message);
      addMessage({
        id: (Date.now() + 1).toString(),
        role: MessageRole.ASSISTANT,
        content: 'Sorry, something went wrong. Please try again.',
        timestamp: new Date(),
      });
    } finally {
      if (thisRequestId === requestIdRef.current) {
        setLoading(false);
        abortRef.current = null;
      }
    }
  }, [addMessage, setLoading, setError, setSessionId, onFormationReceived]);

  return { submit };
}

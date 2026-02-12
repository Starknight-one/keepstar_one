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

export function useChatSubmit({ sessionId, addMessage, setLoading, setError, setSessionId, onFormationReceived }) {
  // useRef to avoid stale closure — callback doesn't depend on sessionId re-renders
  const sessionIdRef = useRef(sessionId);
  useEffect(() => { sessionIdRef.current = sessionId; }, [sessionId]);

  const submittingRef = useRef(false);

  const submit = useCallback(async (text) => {
    if (!text.trim()) return;
    if (submittingRef.current) return; // Prevent duplicate requests
    submittingRef.current = true;

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
      const response = await sendPipelineQuery(sessionIdRef.current, text);

      // Save sessionId to localStorage if new
      if (response.sessionId && response.sessionId !== sessionIdRef.current) {
        sessionIdRef.current = response.sessionId;
        localStorage.setItem(SESSION_STORAGE_KEY, response.sessionId);
        setSessionId(response.sessionId);
      }

      // Notify parent about formation (for external rendering)
      // Pass adjacentFormations as second arg for instant expand (Phase 2)
      if (response.formation && onFormationReceived) {
        onFormationReceived(response.formation, response.adjacentFormations || null);
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
      setError(err.message);
      // Add error message
      addMessage({
        id: (Date.now() + 1).toString(),
        role: MessageRole.ASSISTANT,
        content: 'Sorry, something went wrong. Please try again.',
        timestamp: new Date(),
      });
    } finally {
      setLoading(false);
      submittingRef.current = false;
    }
  }, [addMessage, setLoading, setError, setSessionId, onFormationReceived]);

  return { submit };
}

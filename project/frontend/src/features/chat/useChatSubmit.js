import { useCallback } from 'react';
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
  const submit = useCallback(async (text) => {
    if (!text.trim()) return;

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
      const response = await sendPipelineQuery(sessionId, text);

      // Save sessionId to localStorage if new
      if (response.sessionId && response.sessionId !== sessionId) {
        localStorage.setItem(SESSION_STORAGE_KEY, response.sessionId);
        setSessionId(response.sessionId);
      }

      // Notify parent about formation (for external rendering)
      if (response.formation && onFormationReceived) {
        onFormationReceived(response.formation);
      }

      // Add assistant message with formation
      const widgetCount = response.formation?.widgets?.length || 0;
      addMessage({
        id: (Date.now() + 1).toString(),
        role: MessageRole.ASSISTANT,
        content: widgetCount > 0 ? `Нашёл ${widgetCount} ${getProductWord(widgetCount)}` : '',
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
    }
  }, [sessionId, addMessage, setLoading, setError, setSessionId, onFormationReceived]);

  return { submit };
}

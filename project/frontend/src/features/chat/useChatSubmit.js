import { useCallback } from 'react';
import { sendPipelineQuery } from '../../shared/api/apiClient';
import { MessageRole } from '../../entities/message/messageModel';

const SESSION_STORAGE_KEY = 'chatSessionId';

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
      addMessage({
        id: (Date.now() + 1).toString(),
        role: MessageRole.ASSISTANT,
        content: '', // No text content, just formation
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

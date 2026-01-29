import { useCallback } from 'react';
import { sendChatMessage } from '../../shared/api/apiClient';
import { MessageRole } from '../../entities/message/messageModel';

export function useChatSubmit({ sessionId, addMessage, setLoading, setError, setSessionId }) {
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
      const response = await sendChatMessage(sessionId, text);

      if (response.sessionId) {
        setSessionId(response.sessionId);
      }

      // Add assistant message
      addMessage({
        id: Date.now().toString(),
        role: MessageRole.ASSISTANT,
        content: response.response?.text || '',
        widgets: response.response?.widgets || [],
        formation: response.response?.formation,
        timestamp: new Date(),
      });
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }, [sessionId, addMessage, setLoading, setError, setSessionId]);

  return { submit };
}

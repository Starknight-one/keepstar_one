import { useCallback } from 'react';
import { sendChatMessage } from '../../shared/api/apiClient';
import { MessageRole } from '../../entities/message/messageModel';

export function useChatSubmit({ addMessage, setLoading, setError }) {
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
      const response = await sendChatMessage(text);

      // Add assistant message
      addMessage({
        id: (Date.now() + 1).toString(),
        role: MessageRole.ASSISTANT,
        content: response.response,
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
  }, [addMessage, setLoading, setError]);

  return { submit };
}

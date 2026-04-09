import { useState, useCallback } from 'react';
import { createInitialChatState } from './chatModel';

export function useChatMessages() {
  const [state, setState] = useState(createInitialChatState);

  const addMessage = useCallback((message) => {
    setState((prev) => ({
      ...prev,
      messages: [...prev.messages, message],
    }));
  }, []);

  const setMessages = useCallback((messages) => {
    setState((prev) => ({
      ...prev,
      messages,
    }));
  }, []);

  const setLoading = useCallback((isLoading) => {
    setState((prev) => ({ ...prev, isLoading }));
  }, []);

  const setError = useCallback((error) => {
    setState((prev) => ({ ...prev, error }));
  }, []);

  const setSessionId = useCallback((sessionId) => {
    setState((prev) => ({ ...prev, sessionId }));
  }, []);

  return {
    ...state,
    addMessage,
    setMessages,
    setLoading,
    setError,
    setSessionId,
  };
}

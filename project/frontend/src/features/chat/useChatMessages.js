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

  // updateMessage: merge `patch` into the message with matching id.
  // Used by streaming pipeline to mutate an assistant placeholder as
  // widget_provisional / formation_complete events arrive.
  const updateMessage = useCallback((id, patch) => {
    setState((prev) => ({
      ...prev,
      messages: prev.messages.map((m) => (m.id === id ? { ...m, ...patch } : m)),
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
    updateMessage,
    setMessages,
    setLoading,
    setError,
    setSessionId,
  };
}

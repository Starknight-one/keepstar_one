import { useState, useCallback, useRef } from 'react';

/**
 * Index-based formation history for full back/forward navigation.
 * Replaces useFormationStack — instead of push/pop, stores full history
 * with a currentIndex pointer (like browser history).
 */
export function useFormationHistory(initialHistory = [], initialIndex = -1) {
  const [history, setHistory] = useState(initialHistory);
  const [currentIndex, setCurrentIndex] = useState(initialIndex);
  const historyRef = useRef(initialHistory);
  const indexRef = useRef(initialIndex);

  const push = useCallback((formation, label) => {
    if (!formation) return;
    const entry = { formation, label: label || '' };
    // If we're in the middle of history, trim forward entries (browser behavior)
    const trimmed = historyRef.current.slice(0, indexRef.current + 1);
    const next = [...trimmed, entry];
    const nextIndex = next.length - 1;
    historyRef.current = next;
    indexRef.current = nextIndex;
    setHistory(next);
    setCurrentIndex(nextIndex);
  }, []);

  const goTo = useCallback((index) => {
    if (index < 0 || index >= historyRef.current.length) return;
    indexRef.current = index;
    setCurrentIndex(index);
  }, []);

  const getCurrent = useCallback(() => {
    if (indexRef.current < 0 || indexRef.current >= historyRef.current.length) return null;
    return historyRef.current[indexRef.current].formation;
  }, []);

  const clear = useCallback(() => {
    historyRef.current = [];
    indexRef.current = -1;
    setHistory([]);
    setCurrentIndex(-1);
  }, []);

  const canGoBack = currentIndex > 0;
  const canGoForward = currentIndex < history.length - 1;

  return { history, currentIndex, push, goTo, getCurrent, clear, canGoBack, canGoForward };
}

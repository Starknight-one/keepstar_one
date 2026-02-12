import { useState, useCallback, useRef } from 'react';

/**
 * Formation stack for instant back navigation.
 * Stores previous formations so back = pop from stack (no backend round-trip).
 */
export function useFormationStack(initialStack = []) {
  const [stack, setStack] = useState(initialStack);
  const stackRef = useRef(initialStack);

  const push = useCallback((formation) => {
    if (!formation) return;
    stackRef.current = [...stackRef.current, formation];
    setStack(stackRef.current);
  }, []);

  const pop = useCallback(() => {
    if (stackRef.current.length === 0) return null;
    const prev = stackRef.current[stackRef.current.length - 1];
    stackRef.current = stackRef.current.slice(0, -1);
    setStack(stackRef.current);
    return prev;
  }, []);

  const clear = useCallback(() => {
    stackRef.current = [];
    setStack([]);
  }, []);

  const canGoBack = stack.length > 0;

  return { push, pop, clear, canGoBack, stack };
}

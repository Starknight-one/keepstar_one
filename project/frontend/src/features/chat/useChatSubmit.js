import { useCallback, useRef, useEffect } from 'react';
import { streamPipelineQuery } from '../../shared/api/apiClient';
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

export function useChatSubmit({ sessionId, addMessage, updateMessage, setLoading, setError, setSessionId, onFormationReceived, lastFormationRef }) {
  const sessionIdRef = useRef(sessionId);
  useEffect(() => { sessionIdRef.current = sessionId; }, [sessionId]);

  const requestIdRef = useRef(0);
  const abortRef = useRef(null);

  const submit = useCallback((text) => {
    if (!text.trim()) return;

    // Cancel any in-flight stream
    if (abortRef.current) {
      abortRef.current.abort();
    }

    const thisRequestId = ++requestIdRef.current;

    addMessage({
      id: Date.now().toString(),
      role: MessageRole.USER,
      content: text,
      timestamp: new Date(),
    });

    // Assistant placeholder — gets its formation mutated as stream arrives.
    const assistantId = (Date.now() + 1).toString();
    const provisionalFormation = {
      mode: 'grid',
      widgets: [],
      isStreaming: true,
    };
    addMessage({
      id: assistantId,
      role: MessageRole.ASSISTANT,
      content: '',
      formation: provisionalFormation,
      timestamp: new Date(),
    });

    setLoading(true);
    setError(null);

    // Build screen context from current formation
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

    // Accumulated provisional widgets
    const widgetsAcc = [];

    const controller = streamPipelineQuery(sessionIdRef.current, text, screenContext, {
      onSessionInit: (ev) => {
        if (thisRequestId !== requestIdRef.current) return;
        if (ev.sessionId && ev.sessionId !== sessionIdRef.current) {
          sessionIdRef.current = ev.sessionId;
          localStorage.setItem(SESSION_STORAGE_KEY, ev.sessionId);
          setSessionId(ev.sessionId);
        }
      },
      onWidgetProvisional: (ev) => {
        if (thisRequestId !== requestIdRef.current) return;
        if (Array.isArray(ev.widgets)) {
          widgetsAcc.push(...ev.widgets);
        }
        const liveFormation = {
          mode: 'grid',
          widgets: [...widgetsAcc],
          isStreaming: true,
        };
        updateMessage(assistantId, { formation: liveFormation });
        if (onFormationReceived) {
          onFormationReceived(liveFormation, null, null);
        }
      },
      onFormationComplete: (ev) => {
        if (thisRequestId !== requestIdRef.current) return;
        const finalFormation = ev.formation || { mode: 'grid', widgets: widgetsAcc };
        const widgets = finalFormation.widgets || [];
        const widgetCount = widgets.length + (finalFormation.sections?.reduce((s, sec) => s + (sec.widgets?.length || 0), 0) || 0);
        const isTextOnly = widgets.every(w => w.type === 'text_block');
        updateMessage(assistantId, {
          content: (widgetCount > 0 && !isTextOnly) ? `Нашёл ${widgetCount} ${getProductWord(widgetCount)}` : '',
          formation: finalFormation,
        });
        if (onFormationReceived) {
          onFormationReceived(finalFormation, null, null);
        }
        setLoading(false);
        abortRef.current = null;
      },
      onError: (err) => {
        if (thisRequestId !== requestIdRef.current) return;
        setError(err.message);
        updateMessage(assistantId, {
          content: 'Sorry, something went wrong. Please try again.',
          formation: null,
        });
        setLoading(false);
        abortRef.current = null;
      },
    });

    abortRef.current = controller;
  }, [addMessage, updateMessage, setLoading, setError, setSessionId, onFormationReceived]);

  return { submit };
}

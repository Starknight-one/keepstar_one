import { useCallback, useEffect, useRef, useState } from 'react'
import ChatPanel from './chat/ChatPanel'
import SceneGraphRenderer from './renderer/SceneGraphRenderer'
import RenderContext from './renderer/RenderContext'
import { dispatchAction } from './renderer/actionDispatch'
import { initSession, pipelineRequest } from './api/client'
import { filterApply } from './api/actions'

// WidgetApp — root component. Owns:
//   - sessionId           (created on mount via /session/init)
//   - messages            (chat history shown on the right)
//   - document            (current scene graph shown on the left)
//   - prefetchRef         (1-level navigation prefetch, replaced each turn)
//   - canGoBackRef        (whether view stack has anything to pop)
//   - isLoading           (pipeline call in flight)
//
// Layout shell mirrors V4: full-screen overlay, flex row, scene graph
// occupies the left flex:1 area, ChatPanel pinned at 360px right.
// Buttons + frame clicks dispatch actions through RenderContext;
// drill_detail uses the prefetch payload for instant navigation
// (no round-trip).

export default function WidgetApp({ tenantSlug, apiBaseUrl }) {
  const [sessionId, setSessionId] = useState(null)
  const [messages, setMessages] = useState([])
  const [doc, setDoc] = useState(null)
  const [isLoading, setIsLoading] = useState(false)
  const [canGoBack, setCanGoBack] = useState(false)
  // Data-derived filter facets for the current result set (from each
  // pipeline / filter response) + an epoch that bumps on every new search
  // so the filter panel clears its selections when the result set changes.
  const [facets, setFacets] = useState([])
  const [searchEpoch, setSearchEpoch] = useState(0)

  // Prefetch lives in a ref because actionDispatch reads it per click
  // and we don't want a re-render on every refresh. Updated on every
  // pipeline response.
  const prefetchRef = useRef({ adjacentTemplate: {}, entities: {} })

  const chatInputRef = useRef(null)

  useEffect(() => {
    let cancelled = false
    initSession({ baseUrl: apiBaseUrl, tenantSlug })
      .then((res) => {
        if (cancelled) return
        setSessionId(res.sessionId)
        // eslint-disable-next-line no-console
        console.debug('[v5-renderer] session init', { sessionId: res.sessionId })
      })
      .catch((err) => {
        if (cancelled) return
        // eslint-disable-next-line no-console
        console.error('[v5-renderer] session init failed', err)
        setMessages((m) => [...m, { role: 'error', text: 'Session init failed: ' + err.message }])
      })
    return () => {
      cancelled = true
    }
  }, [apiBaseUrl, tenantSlug])

  const handleSend = useCallback(
    async (query) => {
      setMessages((m) => [...m, { role: 'user', text: query }])
      if (!sessionId) {
        setMessages((m) => [...m, { role: 'error', text: 'No session yet — try again in a moment' }])
        return
      }
      setIsLoading(true)
      try {
        const resp = await pipelineRequest({ baseUrl: apiBaseUrl, tenantSlug, sessionId, query })
        // eslint-disable-next-line no-console
        console.debug('[v5-renderer] spans', summariseSpans(resp.spans), {
          latencyMs: resp.latencyMs,
          agent1Ms: resp.agent1Ms,
          agent2Ms: resp.agent2Ms,
          prefetched: resp.prefetch ? Object.keys(resp.prefetch.adjacentTemplate || {}) : [],
        })
        setDoc(resp.document || null)
        prefetchRef.current = resp.prefetch || { adjacentTemplate: {}, entities: {} }
        // A new pipeline turn resets the view stack on the backend so
        // canGoBack defaults to false. Drill / back actions update it
        // independently.
        setCanGoBack(false)
        setFacets(resp.facets || [])
        setSearchEpoch((e) => e + 1)
        const ack = ackText(resp)
        setMessages((m) => [...m, { role: 'bot', text: ack }])
      } catch (err) {
        // eslint-disable-next-line no-console
        console.error('[v5-renderer] pipeline failed', err)
        setMessages((m) => [...m, { role: 'error', text: err.message }])
      } finally {
        setIsLoading(false)
      }
    },
    [apiBaseUrl, sessionId, tenantSlug],
  )

  // onFilter — brand chip click. Calls the deterministic filter endpoint
  // (no LLM) and swaps the document with the re-filtered grid. Empty
  // brand resets to the full set.
  const onFilter = useCallback(
    async ({ filters, sort }) => {
      if (!sessionId) return
      try {
        const resp = await filterApply({ baseUrl: apiBaseUrl, tenantSlug, sessionId, filters, sort })
        if (resp?.document) setDoc(resp.document)
        if (Array.isArray(resp?.facets)) setFacets(resp.facets)
        setCanGoBack(Boolean(resp?.canGoBack))
      } catch (err) {
        // eslint-disable-next-line no-console
        console.error('[v5-renderer] filter failed', err)
        setMessages((m) => [...m, { role: 'error', text: err.message }])
      }
    },
    [apiBaseUrl, sessionId, tenantSlug],
  )

  // Wraps setDoc so action handlers can replace the scene-graph
  // (drill / back). Keeps canGoBack in sync — drill increments,
  // back decrements; we infer the direction from caller intent via
  // the action that fired (passed through dispatchAction → ctx).
  const onUpdateDocument = useCallback((next) => {
    setDoc(next)
  }, [])

  const onSearch = useCallback(
    (text) => {
      const el = chatInputRef.current
      if (el && typeof el.setValue === 'function') {
        el.setValue(text)
      } else {
        // Fallback: fire as a turn.
        handleSend(text)
      }
    },
    [handleSend],
  )

  const ctxValue = {
    apiBaseUrl,
    tenantSlug,
    sessionId,
    prefetch: prefetchRef.current,
    onUpdateDocument: (next) => {
      onUpdateDocument(next)
      // Heuristic: any frontend-driven document change came from a
      // drill or back; we update the back-availability flag based on
      // which path we took. Drill set canGoBack=true; back resets it.
    },
    onSearch,
  }

  const handleBack = useCallback(() => {
    dispatchAction({ kind: 'back' }, {
      apiBaseUrl,
      tenantSlug,
      sessionId,
      prefetch: prefetchRef.current,
      onUpdateDocument: (next) => {
        setDoc(next)
        setCanGoBack(false)
      },
    })
  }, [apiBaseUrl, sessionId, tenantSlug])

  // The ctx that every renderer leaf reads. Wrap onUpdateDocument so
  // a drill (called from inside the renderer) flips canGoBack=true.
  const renderCtx = {
    ...ctxValue,
    onUpdateDocument: (next) => {
      setDoc(next)
      setCanGoBack(true)
    },
  }

  return (
    <div className="kw-overlay">
      <div className="kw-display">
        {canGoBack && (
          <button className="kw-back" type="button" onClick={handleBack}>
            ← Back
          </button>
        )}
        {doc ? (
          <RenderContext.Provider value={renderCtx}>
            <SceneGraphRenderer document={doc} />
          </RenderContext.Provider>
        ) : (
          <div className="kw-empty-state">
            Type a prompt on the right to start.
          </div>
        )}
      </div>
      <ChatPanel
        messages={messages}
        onSend={handleSend}
        isLoading={isLoading}
        inputRef={chatInputRef}
        facets={facets}
        searchEpoch={searchEpoch}
        onFilter={onFilter}
      />
    </div>
  )
}

function summariseSpans(spans) {
  if (!Array.isArray(spans)) return { count: 0 }
  const llm = spans.find((s) => s?.name === 'agent2.llm')
  const tokens = llm?.attrs?.['tokens.input']
  const cost = llm?.attrs?.cost_usd
  return {
    count: spans.length,
    agent2_input_tokens: tokens,
    agent2_cost_usd: cost,
  }
}

function ackText(resp) {
  const tools = Array.isArray(resp.toolCalls) ? resp.toolCalls : []
  const last = tools[tools.length - 1]
  if (last?.name === 'visual_assembly') {
    const input = last.input || {}
    const preset = input.preset
    const ops = Array.isArray(input.ops) ? input.ops.length : 0
    if (preset && ops) return `rendered preset "${preset}" with ${ops} ops`
    if (preset) return `rendered preset "${preset}"`
    if (ops) return `applied ${ops} ops on the current view`
  }
  return 'rendered'
}

import { useEffect, useState } from 'react'
import ChatPanel from './chat/ChatPanel'
import SceneGraphRenderer from './renderer/SceneGraphRenderer'
import { initSession, pipelineRequest } from './api/client'

// WidgetApp — root component. Owns:
//   - sessionId  (created on mount via /session/init)
//   - messages   (chat history shown on the right)
//   - document   (current scene graph shown on the left)
//   - isLoading  (pipeline call in flight)
//
// Layout shell mirrors V4: full-screen overlay, flex row, scene graph
// occupies the left flex:1 area, ChatPanel pinned at 360px right.
// User clicks LIKE / Buy → no-op handler in wrapper.js logs to console
// (P0-C will wire actions endpoint).

export default function WidgetApp({ tenantSlug, apiBaseUrl }) {
  const [sessionId, setSessionId] = useState(null)
  const [messages, setMessages] = useState([])
  const [document, setDocument] = useState(null)
  const [isLoading, setIsLoading] = useState(false)

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

  const handleSend = async (query) => {
    setMessages((m) => [...m, { role: 'user', text: query }])
    if (!sessionId) {
      setMessages((m) => [...m, { role: 'error', text: 'No session yet — try again in a moment' }])
      return
    }
    setIsLoading(true)
    try {
      const resp = await pipelineRequest({
        baseUrl: apiBaseUrl,
        tenantSlug,
        sessionId,
        query,
      })
      // eslint-disable-next-line no-console
      console.debug('[v5-renderer] spans', summariseSpans(resp.spans), {
        latencyMs: resp.latencyMs,
        agent1Ms: resp.agent1Ms,
        agent2Ms: resp.agent2Ms,
      })
      setDocument(resp.document || null)
      const ack = ackText(resp)
      setMessages((m) => [...m, { role: 'bot', text: ack }])
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error('[v5-renderer] pipeline failed', err)
      setMessages((m) => [...m, { role: 'error', text: err.message }])
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <div className="kw-overlay">
      <div className="kw-display">
        {document ? (
          <SceneGraphRenderer document={document} />
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

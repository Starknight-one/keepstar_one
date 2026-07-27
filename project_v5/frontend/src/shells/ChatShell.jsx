import { useCallback, useEffect, useRef, useState } from 'react'
import BlocksMessageList from '../chat/BlocksMessageList'
import RenderContext from '../renderer/RenderContext'
import { pipelineSmartRequest } from '../api/client'
import { appendStreamBlock, finalizeTurn, replaceBlockInMessages } from '../chat/turnBlocks'
import { applyTheme, tokensFromDoc } from '../theme-style'

// ChatShell — the shared chat-first surface (RUNTIME_SPEC §5.1): one
// centered chat column IS the page. Used by the onboarding shell
// (~720px) and the CRM shell (wide, R13). Blocks stream into the
// current turn's bot message as `event: block` frames arrive (real
// production order, final owner decision 3); the terminal result frame
// settles the canonical list. Legacy single-document turns (e.g. a CRM
// visual_assembly answer) render as one inline document block —
// back-compat, no server change required.
//
// The storefront keeps WidgetApp (display area + 360px chat column) —
// this shell never renders there.
export default function ChatShell({
  apiBaseUrl,
  tenantSlug,
  sessionId,
  shadowRoot,
  variant, // 'onboarding' | 'crm' — column width + accents (CSS)
  headerTitle,
  headerBadge,
  initialMessages,
  placeholder,
  credentials, // 'include' for onboarding (ks_onboard cookie in dev cross-origin setups)
}) {
  const [messages, setMessages] = useState(() => initialMessages || [])
  const [isLoading, setIsLoading] = useState(false)
  const [draft, setDraft] = useState('')

  // Monotonic turn counter — ties streamed block frames to the bot
  // message they belong to (a ref: the callbacks close over one turn).
  const turnSeq = useRef(0)
  const prefetchRef = useRef({ adjacentTemplate: {}, entities: {} })
  const [actionState, setActionState] = useState({ likedIds: [], cartItems: [] })

  // Per-tenant design-system theme (same delivery path as WidgetApp):
  // the result document carries doc.theme.tokens; block documents may
  // carry it too when the turn is blocks-only.
  const applyThemeFromTurn = useCallback(
    (resp, blocks) => {
      if (!shadowRoot) return
      const themed =
        (resp?.document && tokensFromDoc(resp.document) && resp.document) ||
        (blocks || []).map((b) => b.document).find((d) => d && tokensFromDoc(d))
      if (themed) applyTheme(shadowRoot, tokensFromDoc(themed))
    },
    [shadowRoot],
  )

  const replaceBlock = useCallback((blockId, document) => {
    setMessages((m) => replaceBlockInMessages(m, blockId, document))
  }, [])

  const handleSend = useCallback(
    async (query) => {
      if (!sessionId) {
        setMessages((m) => [...m, { role: 'error', text: 'No session yet — try again in a moment' }])
        return
      }
      const turnId = ++turnSeq.current
      setMessages((m) => [...m, { role: 'user', text: query }, { role: 'status', text: 'Thinking…' }])
      setIsLoading(true)
      try {
        const resp = await pipelineSmartRequest({
          baseUrl: apiBaseUrl,
          tenantSlug,
          sessionId,
          query,
          credentials,
          onStage: (ev) => {
            if (ev.phase !== 'data_done') return
            setMessages((m) => replaceStatus(m, ev.empty ? 'Composing an answer…' : 'Composing a response…'))
          },
          onBlock: (b) => {
            setMessages((m) => appendStreamBlock(m, turnId, b))
          },
        })
        prefetchRef.current = resp.prefetch || { adjacentTemplate: {}, entities: {} }
        setMessages((m) => finalizeTurn(m, turnId, resp))
        applyThemeFromTurn(resp, Array.isArray(resp.blocks) ? resp.blocks : [])
      } catch (err) {
        // eslint-disable-next-line no-console
        console.error('[v5-chat] pipeline failed', err)
        setMessages((m) => [...m.filter((x) => x.role !== 'status'), { role: 'error', text: err.message }])
      } finally {
        setIsLoading(false)
      }
    },
    [apiBaseUrl, tenantSlug, sessionId, credentials, applyThemeFromTurn],
  )

  const submit = (e) => {
    e?.preventDefault()
    const value = draft.trim()
    if (!value || isLoading) return
    setDraft('')
    handleSend(value)
  }

  // The SAME RenderContext shape as the main surface (§5.1) so inline
  // documents behave identically; onReplaceBlock is the block-swap
  // applier every form/operation response routes through (§4.8).
  const renderCtx = {
    apiBaseUrl,
    tenantSlug,
    sessionId,
    prefetch: prefetchRef.current,
    actionState,
    onActionState: setActionState,
    onUpdateDocument: () => {}, // per-block scope set by InlineBlock
    onSearch: (text) => setDraft(text),
    blockId: null,
    onReplaceBlock: replaceBlock,
  }

  return (
    <div className={'kw-chatpage' + (variant ? ` kw-chatpage--${variant}` : '')}>
      <div className="kw-chatpage-col">
        {headerTitle ? (
          <div className="kw-chatpage-header">
            <span className="kw-chatpage-title">{headerTitle}</span>
            {headerBadge ? <span className="kw-chatpage-badge">{headerBadge}</span> : null}
          </div>
        ) : null}
        <RenderContext.Provider value={renderCtx}>
          <BlocksMessageList messages={messages} />
        </RenderContext.Provider>
        <form className="kw-chatpage-input" onSubmit={submit}>
          <input
            type="text"
            placeholder={isLoading ? 'Loading…' : placeholder || 'Ask anything…'}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            disabled={isLoading}
          />
          <button type="submit" disabled={isLoading || !draft.trim()}>
            Send
          </button>
        </form>
      </div>
    </div>
  )
}

function replaceStatus(messages, text) {
  if (!messages.some((m) => m.role === 'status')) return messages
  const out = messages.filter((m) => m.role !== 'status')
  out.push({ role: 'status', text })
  return out
}

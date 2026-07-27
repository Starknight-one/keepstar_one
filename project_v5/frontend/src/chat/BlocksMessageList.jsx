import { useEffect, useRef } from 'react'
import InlineBlock from './InlineBlock'

// BlocksMessageList — chat history for the chat-first shells
// (onboarding / CRM, RUNTIME_SPEC §5.1). Unlike the storefront's
// MessageList (plain text acks), one bot message here is an ordered
// list of TurnBlocks: text paragraphs interleaved with inline rendered
// documents, appearing one by one as `event: block` frames arrive.
// Plain-text messages (user / error / status / legacy bot text) render
// exactly like the storefront bubbles.
export default function BlocksMessageList({ messages }) {
  const scrollRef = useRef(null)

  // Streaming blocks grow the history downward — keep the newest content
  // in view (the column is the whole page in chat-first shells).
  useEffect(() => {
    const el = scrollRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [messages])

  return (
    <div className="kw-chatpage-history" ref={scrollRef}>
      {messages.map((m, i) =>
        m.role === 'bot' && Array.isArray(m.blocks) ? (
          <div className="kw-turn" key={m.turnId != null ? `turn-${m.turnId}` : i}>
            {m.blocks.map((b, j) =>
              b.kind === 'text' ? (
                <div className="kw-msg kw-msg-bot kw-block-text" key={b.blockId || j}>
                  {b.text}
                </div>
              ) : (
                <InlineBlock block={b} key={b.blockId || j} />
              ),
            )}
          </div>
        ) : (
          <div
            key={i}
            className={
              'kw-msg ' +
              (m.role === 'user'
                ? 'kw-msg-user'
                : m.role === 'error'
                ? 'kw-msg-error'
                : m.role === 'status'
                ? 'kw-msg-status'
                : 'kw-msg-bot')
            }
          >
            {m.text}
          </div>
        ),
      )}
    </div>
  )
}

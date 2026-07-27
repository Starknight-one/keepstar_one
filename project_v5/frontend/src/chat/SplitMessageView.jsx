import { useEffect, useRef } from 'react'
import InlineBlock from './InlineBlock'

// SplitMessageView — the "canvas left / chat right" layout the product
// is known for (owner: onboarding must look like the original widget).
// One message stream, two panes: document blocks materialize on the
// left canvas in arrival order; text blocks and user/status/error
// bubbles stay in the right chat column. Both panes follow the newest
// content.
export default function SplitMessageView({ messages, header, inputForm }) {
  const canvasRef = useRef(null)
  const chatRef = useRef(null)

  const canvasBlocks = []
  const chatItems = []
  messages.forEach((m, i) => {
    if (m.role === 'bot' && Array.isArray(m.blocks)) {
      m.blocks.forEach((b, j) => {
        if (b.kind === 'text') {
          chatItems.push({ kind: 'bubble', role: 'bot', text: b.text, key: b.blockId || `t${i}-${j}` })
        } else {
          canvasBlocks.push({ block: b, key: b.blockId || `d${i}-${j}` })
        }
      })
    } else {
      chatItems.push({ kind: 'bubble', role: m.role, text: m.text, key: `m${i}` })
    }
  })

  useEffect(() => {
    if (canvasRef.current) canvasRef.current.scrollTop = canvasRef.current.scrollHeight
    if (chatRef.current) chatRef.current.scrollTop = chatRef.current.scrollHeight
  }, [messages])

  return (
    <div className="kw-split">
      <div className="kw-split-canvas" ref={canvasRef}>
        {canvasBlocks.length === 0 ? (
          <div className="kw-empty-state">Your workspace preview will appear here.</div>
        ) : (
          canvasBlocks.map(({ block, key }) => <InlineBlock block={block} key={key} />)
        )}
      </div>
      <div className="kw-split-chat">
        {header}
        <div className="kw-split-history" ref={chatRef}>
          {chatItems.map((it) => (
            <div
              key={it.key}
              className={
                'kw-msg ' +
                (it.role === 'user'
                  ? 'kw-msg-user'
                  : it.role === 'error'
                  ? 'kw-msg-error'
                  : it.role === 'status'
                  ? 'kw-msg-status'
                  : 'kw-msg-bot')
              }
            >
              {it.text}
            </div>
          ))}
        </div>
        {inputForm}
      </div>
    </div>
  )
}

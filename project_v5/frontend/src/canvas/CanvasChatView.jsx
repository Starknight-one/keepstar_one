import { useMemo } from 'react'
import CanvasStage from './CanvasStage'
import ChatDock from './ChatDock'
import { collectSurfaceLinks } from './surfaceLinks'

// CanvasChatView — the builder layout (V2_SPEC §2 step 3): the canvas IS
// the stage, the chat floats over it and docks at the bottom (mock-demo
// birthDesktop pattern). Replaces the v1 split layout (canvas left /
// chat right).
//
// One message stream, two destinations — unchanged from the split
// layout so every existing behavior (registration form, uploader card,
// operation invokes, resume) keeps working: document blocks materialize
// on the canvas, text blocks and user/error lines live in the dock.
// Status lines are NOT dock bubbles here — they are the canvas thinking
// indicator, so the in-flight state sits where the content will land.

export const DEFAULT_THINKING_LABEL = 'Designing…'

// splitTurnStream — pure message-stream → {canvas documents, dock lines}
// projection. Kept pure and exported so the routing is unit-testable
// without a DOM.
export function splitTurnStream(messages) {
  const canvasBlocks = []
  const chatItems = []
  ;(messages || []).forEach((m, i) => {
    if (m.role === 'bot' && Array.isArray(m.blocks)) {
      m.blocks.forEach((b, j) => {
        if (b.kind === 'text') {
          chatItems.push({ role: 'bot', text: b.text, key: b.blockId || `t${i}-${j}` })
        } else {
          canvasBlocks.push({ block: b, key: b.blockId || `d${i}-${j}` })
        }
      })
    } else {
      chatItems.push({ role: m.role, text: m.text, key: `m${i}` })
    }
  })
  return { canvasBlocks, chatItems }
}

// thinkingLabel — what the canvas indicator says right now, or null when
// it must be off. WHY the block check: the indicator is a placeholder for
// content that has not arrived; the moment the turn streams its first
// document the canvas shows the real thing and the indicator hands off
// (L1 — narration never stands in for something renderable). The label
// itself is the live stage phase when the pipeline reported one, so the
// text is honest rather than decorative.
export function thinkingLabel(messages, isLoading) {
  if (!isLoading) return null
  const list = Array.isArray(messages) ? messages : []
  let status = null
  for (let i = list.length - 1; i >= 0; i--) {
    const m = list[i]
    if (!m) continue
    if (m.role === 'status' && status === null && m.text) status = m.text
    if (m.role === 'user') break
    if (m.role === 'bot' && Array.isArray(m.blocks) && m.blocks.some((b) => b.kind !== 'text')) {
      return null
    }
  }
  return status || DEFAULT_THINKING_LABEL
}

export default function CanvasChatView({ messages, header, inputForm, isLoading, manifest }) {
  const { canvasBlocks, chatItems } = splitTurnStream(messages)
  const label = thinkingLabel(messages, isLoading)
  // Walking every rendered document is not free — only redo it when the
  // stream or the manifest actually moved (a keystroke in the composer
  // re-renders this whole view).
  const surfaces = useMemo(() => collectSurfaceLinks({ manifest, messages }), [manifest, messages])

  return (
    <div className="kw-canvas">
      <CanvasStage
        blocks={canvasBlocks}
        header={header}
        thinkingLabel={label}
        surfaces={surfaces}
      />
      <ChatDock items={chatItems.filter((it) => it.role !== 'status')}>{inputForm}</ChatDock>
    </div>
  )
}

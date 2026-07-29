import CanvasStage from './CanvasStage'
import ChatDock from './ChatDock'

// CanvasChatView — the builder layout (V2_SPEC §2 step 3): the canvas IS
// the stage, the chat floats over it and docks at the bottom (mock-demo
// birthDesktop pattern). Replaces the v1 split layout (canvas left /
// chat right).
//
// One message stream, two destinations — unchanged from the split
// layout so every existing behavior (registration form, uploader card,
// operation invokes, resume) keeps working: document blocks materialize
// on the canvas, text blocks and user/status/error lines live in the
// dock.

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

export default function CanvasChatView({ messages, header, inputForm }) {
  const { canvasBlocks, chatItems } = splitTurnStream(messages)

  return (
    <div className="kw-canvas">
      <CanvasStage blocks={canvasBlocks} header={header} />
      <ChatDock items={chatItems}>{inputForm}</ChatDock>
    </div>
  )
}

import SceneGraphRenderer from '../renderer/SceneGraphRenderer'
import RenderContext, { useRenderContext } from '../renderer/RenderContext'

// InlineBlock — renders one document TurnBlock inside the chat column
// (RUNTIME_SPEC §5.1: the SAME RenderContext as the main surface, so
// forms, operation_invoke buttons, drill actions and block swaps all
// work identically). The only per-block additions to the context:
//   blockId          — so /operations/invoke requests reference the
//                      originating block (§4.8)
//   onUpdateDocument — scoped to THIS block: a drill inside an inline
//                      card replaces the block's document, not some
//                      other surface
//
// display:"screen" blocks also render inline here — the chat-first
// shells ARE the surface (R13); the server has already stamped
// Current.Template for them, so navigation state stays consistent.
export default function InlineBlock({ block }) {
  const baseCtx = useRenderContext()
  if (!block || !block.document) return null

  const blockCtx = {
    ...baseCtx,
    blockId: block.blockId || null,
    onUpdateDocument: (next) => {
      if (typeof baseCtx?.onReplaceBlock === 'function' && block.blockId) {
        baseCtx.onReplaceBlock(block.blockId, next)
      } else if (typeof baseCtx?.onUpdateDocument === 'function') {
        baseCtx.onUpdateDocument(next)
      }
    },
  }

  return (
    <div
      className="kw-inline-block"
      data-block-id={block.blockId || ''}
      data-display={block.display || 'inline'}
    >
      <RenderContext.Provider value={blockCtx}>
        <SceneGraphRenderer document={block.document} />
      </RenderContext.Provider>
    </div>
  )
}

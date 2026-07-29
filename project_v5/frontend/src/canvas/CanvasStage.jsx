import { useEffect, useRef } from 'react'
import InlineBlock from '../chat/InlineBlock'

// CanvasStage — the full-page stage the builder assembles onto
// (V2_SPEC §2 step 3 + L1 "render, don't narrate"). Document blocks
// land here in arrival order; the chat dock floats over it.
export default function CanvasStage({ blocks, header }) {
  const paneRef = useRef(null)

  // Follow the newest block — assembly grows downward.
  useEffect(() => {
    const el = paneRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [blocks.length])

  return (
    <div className="kw-canvas-stage">
      <div className="kw-canvas-topbar">{header}</div>
      <div className="kw-canvas-pane" ref={paneRef}>
        {blocks.length === 0 ? (
          <div className="kw-empty-state">Your workspace will be assembled here.</div>
        ) : (
          <div className="kw-canvas-blocks">
            {blocks.map(({ block, key }) => (
              <InlineBlock block={block} key={key} />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

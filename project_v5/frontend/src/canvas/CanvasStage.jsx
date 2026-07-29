import { useEffect, useRef } from 'react'
import InlineBlock from '../chat/InlineBlock'
import ThinkingIndicator from './ThinkingIndicator'
import { useStaggerDelays } from './stagger'

// CanvasStage — the full-page stage the builder assembles onto
// (V2_SPEC §2 step 3 + L1 "render, don't narrate"). Document blocks
// land here as their `event: block` frames arrive, each staggering in
// (y + scale + opacity); the chat dock floats over it.
export default function CanvasStage({ blocks, header, thinkingLabel }) {
  const paneRef = useRef(null)
  const delays = useStaggerDelays(blocks.map((b) => b.key))

  // Follow the newest block — assembly grows downward.
  useEffect(() => {
    const el = paneRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [blocks.length, thinkingLabel])

  const empty = blocks.length === 0 && !thinkingLabel

  return (
    <div className="kw-canvas-stage">
      <div className="kw-canvas-topbar">{header}</div>
      <div className="kw-canvas-pane" ref={paneRef}>
        {empty ? (
          <div className="kw-empty-state">Your workspace will be assembled here.</div>
        ) : (
          <div className="kw-canvas-blocks">
            {blocks.map(({ block, key }) => (
              <div
                className="kw-canvas-block"
                key={key}
                style={{ animationDelay: `${delays.get(key) || 0}ms` }}
              >
                <InlineBlock block={block} />
              </div>
            ))}
            {thinkingLabel ? <ThinkingIndicator label={thinkingLabel} /> : null}
          </div>
        )}
      </div>
    </div>
  )
}

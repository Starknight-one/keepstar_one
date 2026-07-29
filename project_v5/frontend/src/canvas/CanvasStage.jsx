import { useEffect, useRef, useState } from 'react'
import InlineBlock from '../chat/InlineBlock'
import ThinkingIndicator from './ThinkingIndicator'
import { useStaggerDelays } from './stagger'
import { VERSION } from './version'

// CanvasStage — the full-page stage the builder assembles onto
// (V2_SPEC §2 step 3 + L1 "render, don't narrate"). Document blocks
// land on the Builder tab as their `event: block` frames arrive, each
// staggering in (y + scale + opacity); the chat dock floats over it.
//
// Tabs (V2_SPEC §2 step 5 + L2 — the user looks at their app, never at a
// schema): Builder is always there; a Storefront / CRM tab appears the
// moment that surface's URL has been issued, and shows the LIVE page in
// an iframe — the real thing, not a picture of it. Before a URL exists
// its tab is simply absent. Selection stays with the user here; the
// builder-driven demonstration that switches tabs is B5.
const BUILDER_TAB = 'builder'

export default function CanvasStage({ blocks, header, thinkingLabel, surfaces = [] }) {
  const paneRef = useRef(null)
  const delays = useStaggerDelays(blocks.map((b) => b.key))
  const [tab, setTab] = useState(BUILDER_TAB)
  // Surfaces keep their iframe mounted once opened, so switching tabs
  // never throws away the user's state on that page (a storefront
  // search, a CRM chat).
  const [opened, setOpened] = useState(() => [])

  // A tab that is no longer issued must never stay selected.
  const live = surfaces.some((s) => s.surface === tab)
  useEffect(() => {
    if (tab !== BUILDER_TAB && !live) setTab(BUILDER_TAB)
  }, [tab, live])

  const select = (next) => {
    setTab(next)
    if (next !== BUILDER_TAB) setOpened((o) => (o.includes(next) ? o : [...o, next]))
  }

  // Follow the newest block — assembly grows downward.
  useEffect(() => {
    const el = paneRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [blocks.length, thinkingLabel])

  const onBuilder = tab === BUILDER_TAB
  const empty = blocks.length === 0 && !thinkingLabel

  return (
    <div className="kw-canvas-stage">
      <div className="kw-canvas-topbar">
        {header}
        <div className="kw-canvas-tabs" role="tablist">
          <Tab id={BUILDER_TAB} active={onBuilder} onSelect={select}>
            Builder
          </Tab>
          {surfaces.map((s) => (
            <Tab key={s.surface} id={s.surface} active={tab === s.surface} onSelect={select} entering>
              {s.label}
            </Tab>
          ))}
        </div>
      </div>

      <div className="kw-canvas-pane" ref={paneRef} hidden={!onBuilder}>
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

      {surfaces
        .filter((s) => opened.includes(s.surface))
        .map((s) => (
          <div
            className="kw-canvas-pane kw-canvas-pane--surface"
            key={s.surface}
            hidden={tab !== s.surface}
          >
            <div className="kw-canvas-surfacebar">
              <span className="kw-canvas-surfaceurl">{s.url}</span>
              <a
                className="kw-canvas-surfaceopen"
                href={s.url}
                target="_blank"
                rel="noreferrer"
              >
                Open in a new tab
              </a>
            </div>
            <iframe className="kw-canvas-frame" src={s.url} title={s.label} />
          </div>
        ))}

      {/* What is live right now (V2_SPEC §1) — history in VERSIONS.md. */}
      <div className="kw-canvas-version">{VERSION}</div>
    </div>
  )
}

function Tab({ id, active, entering, onSelect, children }) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
      className={
        'kw-canvas-tab' +
        (active ? ' kw-canvas-tab--active' : '') +
        (entering ? ' kw-canvas-tab--entering' : '')
      }
      onClick={() => onSelect(id)}
    >
      {children}
    </button>
  )
}

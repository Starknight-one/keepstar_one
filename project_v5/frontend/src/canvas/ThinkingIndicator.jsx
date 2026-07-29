// ThinkingIndicator — the in-flight state on the canvas (mock-demo
// birthDesktop): brand mark + label + three dots, floating where the
// content is about to land. It hands off the moment the first document
// block of the turn arrives — the indicator is never the answer (L1).
export default function ThinkingIndicator({ label }) {
  return (
    <div className="kw-think" role="status">
      <span className="kw-think-mark" aria-hidden="true" />
      <span className="kw-think-label">{label}</span>
      <span className="kw-think-dots" aria-hidden="true">
        <span className="kw-think-dot" />
        <span className="kw-think-dot" />
        <span className="kw-think-dot" />
      </span>
    </div>
  )
}

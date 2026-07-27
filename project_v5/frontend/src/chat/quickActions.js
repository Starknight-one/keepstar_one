// Contextual quick-action chips (owner feedback 2026-07-28): controls
// must not sit on screen permanently — they appear when relevant.
// Visibility is declarative: a chip may carry `when`, evaluated here
// against the conversation state, so the shell stays universal (no
// vertical or flow hardcode — any surface tags its own chips). Pure
// functions, unit-testable without React or a fetch mock.
//
//   when: 'plan-unapplied'   — the latest known manifest has staged
//                              steps not yet applied ("Accept the plan")
//   when: 'after-first-turn' — at least one bot turn happened: a bot
//                              message AFTER the first user message (the
//                              seeded greeting alone does not count)
//   when absent/''           — always visible (legacy behavior)
//   unknown `when`           — hidden: an irrelevant control must never
//                              render because of a tag typo/drift

// manifestCounts — {staged, applied} out of either manifest shape:
//   full manifest {steps:[{status}]}  (GET /onboard/session resume)
//   summary counts {staged, applied}  (pipeline response manifest field)
// `skipped` steps are terminally resolved and count as applied; proposed,
// accepted and failed steps are all still waiting on an apply.
export function manifestCounts(manifest) {
  if (!manifest || typeof manifest !== 'object') return { staged: 0, applied: 0 }
  if (Array.isArray(manifest.steps)) {
    let staged = 0
    let applied = 0
    for (const s of manifest.steps) {
      if (!s || typeof s !== 'object') continue
      staged++
      if (s.status === 'applied' || s.status === 'skipped') applied++
    }
    return { staged, applied }
  }
  return {
    staged: typeof manifest.staged === 'number' ? manifest.staged : 0,
    applied: typeof manifest.applied === 'number' ? manifest.applied : 0,
  }
}

// hasPlanUnapplied — a plan is staged and not (fully) applied. This is
// the "Accept the plan" moment; once staged === applied the chip goes.
export function hasPlanUnapplied(manifest) {
  const { staged, applied } = manifestCounts(manifest)
  return staged > applied
}

// hasBotTurn — a real turn happened: a bot message after the first user
// message. Covers live turns AND resumed transcripts (both carry user
// messages); excludes the fresh page where the only bot message is the
// seeded greeting.
export function hasBotTurn(messages) {
  if (!Array.isArray(messages)) return false
  const firstUser = messages.findIndex((m) => m && m.role === 'user')
  if (firstUser === -1) return false
  return messages.some((m, i) => i > firstUser && m && m.role === 'bot')
}

// visibleQuickActions — the chips the shell actually renders right now.
export function visibleQuickActions(quickActions, { manifest, messages } = {}) {
  if (!Array.isArray(quickActions)) return []
  return quickActions.filter((qa) => {
    if (!qa || typeof qa !== 'object') return false
    if (qa.when === undefined || qa.when === '') return true
    if (qa.when === 'plan-unapplied') return hasPlanUnapplied(manifest)
    if (qa.when === 'after-first-turn') return hasBotTurn(messages)
    return false
  })
}

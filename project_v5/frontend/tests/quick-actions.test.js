import { describe, expect, it } from 'vitest'
import {
  manifestCounts,
  hasPlanUnapplied,
  hasBotTurn,
  visibleQuickActions,
} from '../src/chat/quickActions'

// Contextual chip visibility (owner feedback 2026-07-28): chips must not
// sit on screen permanently — "Accept the plan" only while a staged plan
// awaits an apply, "Show the plan" only after a real bot turn, nothing on
// a fresh page. The logic is pure so these tests encode the exact rules
// the shell renders by.

const ACCEPT = { label: 'Accept the plan', send: 'apply it', when: 'plan-unapplied' }
const SHOW = { label: 'Show the plan', send: 'show it', when: 'after-first-turn' }

describe('manifestCounts', () => {
  it('handles null/undefined/garbage as an empty plan', () => {
    expect(manifestCounts(null)).toEqual({ staged: 0, applied: 0 })
    expect(manifestCounts(undefined)).toEqual({ staged: 0, applied: 0 })
    expect(manifestCounts('nope')).toEqual({ staged: 0, applied: 0 })
  })

  it('counts a full manifest: proposed/accepted/failed are unapplied, skipped counts as applied', () => {
    const m = {
      steps: [
        { id: '1', status: 'applied' },
        { id: '2', status: 'proposed' },
        { id: '3', status: 'accepted' },
        { id: '4', status: 'failed' },
        { id: '5', status: 'skipped' },
      ],
    }
    expect(manifestCounts(m)).toEqual({ staged: 5, applied: 2 })
  })

  it('accepts a summary shape {staged, applied} from the pipeline response', () => {
    expect(manifestCounts({ staged: 6, applied: 2 })).toEqual({ staged: 6, applied: 2 })
  })
})

describe('hasPlanUnapplied', () => {
  it('true only while staged exceeds applied', () => {
    expect(hasPlanUnapplied({ steps: [{ status: 'proposed' }] })).toBe(true)
    expect(hasPlanUnapplied({ staged: 3, applied: 1 })).toBe(true)
    expect(hasPlanUnapplied({ steps: [{ status: 'applied' }] })).toBe(false)
    expect(hasPlanUnapplied({ staged: 3, applied: 3 })).toBe(false)
    expect(hasPlanUnapplied(null)).toBe(false)
    expect(hasPlanUnapplied({ steps: [] })).toBe(false)
  })
})

describe('hasBotTurn', () => {
  it('false on a fresh page — the seeded greeting alone is not a turn', () => {
    expect(hasBotTurn([])).toBe(false)
    expect(hasBotTurn([{ role: 'bot', text: 'Tell me about your business.' }])).toBe(false)
  })

  it('false while the first turn is still unanswered', () => {
    expect(
      hasBotTurn([
        { role: 'bot', text: 'greeting' },
        { role: 'user', text: 'I run a realtor agency' },
        { role: 'status', text: 'Thinking…' },
      ]),
    ).toBe(false)
  })

  it('true once a bot message follows the first user message (live turn or resumed transcript)', () => {
    expect(
      hasBotTurn([
        { role: 'bot', text: 'greeting' },
        { role: 'user', text: 'hi' },
        { role: 'bot', turnId: 1, blocks: [] },
      ]),
    ).toBe(true)
    // Resumed transcript: no turnIds, still a real turn.
    expect(
      hasBotTurn([
        { role: 'user', text: 'I run a realtor agency' },
        { role: 'bot', text: 'Here is the plan so far.' },
      ]),
    ).toBe(true)
  })
})

describe('visibleQuickActions', () => {
  const TURN_MESSAGES = [
    { role: 'user', text: 'hi' },
    { role: 'bot', turnId: 1, blocks: [] },
  ]

  it('renders NO chips on a fresh page', () => {
    const fresh = [{ role: 'bot', text: 'greeting' }]
    expect(visibleQuickActions([ACCEPT, SHOW], { manifest: null, messages: fresh })).toEqual([])
  })

  it('shows Accept only while the plan is staged and unapplied', () => {
    const staged = { steps: [{ status: 'proposed' }, { status: 'proposed' }] }
    expect(visibleQuickActions([ACCEPT, SHOW], { manifest: staged, messages: TURN_MESSAGES })).toEqual([
      ACCEPT,
      SHOW,
    ])
    const done = { steps: [{ status: 'applied' }, { status: 'applied' }] }
    expect(visibleQuickActions([ACCEPT, SHOW], { manifest: done, messages: TURN_MESSAGES })).toEqual([SHOW])
  })

  it('shows Show-the-plan after the first bot turn even before anything is staged', () => {
    expect(visibleQuickActions([ACCEPT, SHOW], { manifest: null, messages: TURN_MESSAGES })).toEqual([SHOW])
  })

  it('untagged chips keep legacy always-on behavior; unknown tags hide', () => {
    const plain = { label: 'Help', send: 'help' }
    const bogus = { label: 'X', send: 'x', when: 'no-such-condition' }
    expect(visibleQuickActions([plain, bogus], { manifest: null, messages: [] })).toEqual([plain])
  })

  it('tolerates a missing/invalid quickActions prop', () => {
    expect(visibleQuickActions(undefined, {})).toEqual([])
    expect(visibleQuickActions([null, 'x'], {})).toEqual([])
  })
})

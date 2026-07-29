import { describe, expect, it } from 'vitest'
import { assignDelays, STAGGER_STEP_MS } from '../src/canvas/stagger'

// Staged reveal (V2_SPEC B3, owner law "never fake streaming"). The
// delay table exists ONLY to keep a batch that mounts in one frame from
// flashing in together. A block that arrived alone on the wire must get
// delay 0 — the SSE stream is the choreography. If this ever starts
// handing out non-zero delays to single arrivals, the reveal has become
// theatre and the law is broken.

describe('assignDelays', () => {
  it('gives a block that arrives alone no delay at all', () => {
    const first = assignDelays(new Map(), ['b1'])
    expect(first.get('b1')).toBe(0)
    const second = assignDelays(first, ['b1', 'b2'])
    expect(second.get('b2')).toBe(0)
  })

  it('staggers a batch that mounts together (resume / terminal settle)', () => {
    const d = assignDelays(new Map(), ['b1', 'b2', 'b3'])
    expect([d.get('b1'), d.get('b2'), d.get('b3')]).toEqual([
      0,
      STAGGER_STEP_MS,
      2 * STAGGER_STEP_MS,
    ])
  })

  it('keeps a known block on its first delay so re-renders never replay it', () => {
    const first = assignDelays(new Map(), ['b1', 'b2'])
    const again = assignDelays(first, ['b1', 'b2'])
    expect(again).toBe(first) // untouched map — nothing new to assign
    expect(again.get('b2')).toBe(STAGGER_STEP_MS)

    const withNew = assignDelays(again, ['b1', 'b2', 'b3'])
    expect(withNew.get('b2')).toBe(STAGGER_STEP_MS)
    expect(withNew.get('b3')).toBe(0)
  })
})

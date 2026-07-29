import { useRef } from 'react'

// Staged reveal (V2_SPEC B3 + L1): blocks arrive as real `event: block`
// SSE frames, so the honest stream IS the choreography — a block that
// arrives alone animates in immediately (delay 0). Only blocks that
// MOUNT TOGETHER (a resumed transcript, or the terminal result frame
// settling several blocks at once) get the realEstateBirth
// staggerChildren cadence, so a batch still reads as a wave instead of
// a flash. No block is ever held back to fake a stream.

export const STAGGER_STEP_MS = 90

// assignDelays — pure: every key not already known gets a delay equal to
// its position WITHIN THIS BATCH of new keys. Known keys keep the delay
// they were first assigned (a re-render must not replay the animation).
export function assignDelays(known, keys, stepMs = STAGGER_STEP_MS) {
  let batch = 0
  let out = known
  for (const k of keys) {
    if (out.has(k)) continue
    if (out === known) out = new Map(known)
    out.set(k, batch * stepMs)
    batch++
  }
  return out
}

// useStaggerDelays — the hook form. Assignment happens during render so
// the delay is on the element the moment it first paints; it is
// idempotent for keys already seen (safe under StrictMode double-render).
export function useStaggerDelays(keys) {
  const ref = useRef(new Map())
  ref.current = assignDelays(ref.current, keys)
  return ref.current
}

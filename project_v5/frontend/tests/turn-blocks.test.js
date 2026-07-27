import { describe, expect, it } from 'vitest'
import {
  appendStreamBlock,
  finalizeTurn,
  replaceBlockInMessages,
  transcriptToMessages,
} from '../src/chat/turnBlocks'

// The blocks chat message transforms (§4.7 + §5.1): streamed blocks
// build the current turn's bot message one by one IN ARRIVAL ORDER, the
// terminal result settles it, and apply {target:"block"} swaps a single
// block's document in place. Pure functions — the streaming feel of the
// chat shells hangs on exactly these invariants.

const T1 = { blockId: 'b1', kind: 'text', text: 'first' }
const D1 = { blockId: 'b2', kind: 'document', document: { version: '2.10', children: [] }, display: 'inline' }
const T2 = { blockId: 'b3', kind: 'text', text: 'second' }

describe('appendStreamBlock', () => {
  it('creates the turn message before the trailing status line, then appends in order', () => {
    let m = [
      { role: 'user', text: 'hi' },
      { role: 'status', text: 'Thinking…' },
    ]
    m = appendStreamBlock(m, 1, T1)
    expect(m.map((x) => x.role)).toEqual(['user', 'bot', 'status'])
    m = appendStreamBlock(m, 1, D1)
    m = appendStreamBlock(m, 1, T2)
    expect(m[1].blocks).toEqual([T1, D1, T2]) // real arrival order, text/document interleaved
  })

  it('a re-emitted blockId replaces in place instead of duplicating', () => {
    let m = appendStreamBlock([], 1, T1)
    m = appendStreamBlock(m, 1, { ...T1, text: 'first (updated)' })
    expect(m[0].blocks).toHaveLength(1)
    expect(m[0].blocks[0].text).toBe('first (updated)')
  })
})

describe('finalizeTurn', () => {
  it('settles on the terminal blocks and drops the status line', () => {
    let m = [
      { role: 'user', text: 'hi' },
      { role: 'status', text: 'Thinking…' },
    ]
    m = appendStreamBlock(m, 1, T1)
    m = finalizeTurn(m, 1, { blocks: [T1, D1, T2] })
    expect(m.map((x) => x.role)).toEqual(['user', 'bot'])
    expect(m[1].blocks).toEqual([T1, D1, T2])
  })

  it('keeps the streamed blocks when the result carries none', () => {
    let m = appendStreamBlock([{ role: 'status', text: '…' }], 1, T1)
    m = finalizeTurn(m, 1, {})
    expect(m).toEqual([{ role: 'bot', turnId: 1, blocks: [T1] }])
  })

  it('legacy single-document turn synthesizes one inline document block (back-compat)', () => {
    const doc = { version: '2.10', children: [{ type: 'text', content: 'x' }] }
    const m = finalizeTurn([{ role: 'user', text: 'q' }, { role: 'status', text: '…' }], 7, { document: doc })
    expect(m).toHaveLength(2)
    expect(m[1].blocks).toHaveLength(1)
    expect(m[1].blocks[0].kind).toBe('document')
    expect(m[1].blocks[0].document).toBe(doc)
  })

  it('a turn with nothing visible leaves no empty bot message', () => {
    let m = [{ role: 'user', text: 'q' }, { role: 'status', text: '…' }]
    m = finalizeTurn(m, 3, {})
    expect(m).toEqual([{ role: 'user', text: 'q' }])
  })
})

describe('replaceBlockInMessages', () => {
  it('swaps exactly the matching block document (e.g. success_plaque apply)', () => {
    const plaque = { version: '2.10', children: [{ type: 'text', content: 'Booked!' }] }
    const messages = [
      { role: 'user', text: 'book it' },
      { role: 'bot', turnId: 1, blocks: [T1, D1] },
    ]
    const out = replaceBlockInMessages(messages, 'b2', plaque)
    expect(out[1].blocks[0]).toEqual(T1) // untouched
    expect(out[1].blocks[1].document).toBe(plaque)
    expect(out[1].blocks[1].kind).toBe('document')
    // unknown blockId → identical list back (no re-render churn)
    expect(replaceBlockInMessages(messages, 'nope', plaque)).toBe(messages)
  })
})

describe('transcriptToMessages', () => {
  it('maps resume transcript entries defensively', () => {
    const out = transcriptToMessages([
      { role: 'user', text: 'I run a realtor agency' },
      { role: 'bot', blocks: [T1, D1] },
      { role: 'assistant', text: 'plain text entry' },
      null,
      { role: 'user' }, // no text → skipped
    ])
    expect(out).toEqual([
      { role: 'user', text: 'I run a realtor agency' },
      { role: 'bot', blocks: [T1, D1] },
      { role: 'bot', text: 'plain text entry' },
    ])
    expect(transcriptToMessages(undefined)).toEqual([])
  })
})

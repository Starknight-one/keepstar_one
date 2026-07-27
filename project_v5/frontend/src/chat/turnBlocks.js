// Pure message-list transforms for the blocks chat (RUNTIME_SPEC §4.7 +
// §5.1 + final owner decision 3). One bot message = {role:'bot', turnId,
// blocks:[TurnBlock]}; blocks stream in one by one (`event: block`
// frames, real arrival order) and the terminal result frame settles the
// canonical list. Kept pure so the streaming behavior is unit-testable
// without React or a fetch mock.
//
// TurnBlock (domain/turn.go): {blockId, kind:"text"|"document", text?,
// document?, display:"inline"|"screen"}.

// appendStreamBlock — one `event: block` frame landed for turnId.
// Appends to that turn's bot message, creating it BEFORE any trailing
// status line (status stays visually last) on the first block. A repeat
// blockId replaces in place (server re-emission is not a duplicate row).
export function appendStreamBlock(messages, turnId, block) {
  const idx = messages.findIndex((m) => m.role === 'bot' && m.turnId === turnId)
  if (idx === -1) {
    const out = [...messages]
    const insertAt = out.length > 0 && out[out.length - 1].role === 'status' ? out.length - 1 : out.length
    out.splice(insertAt, 0, { role: 'bot', turnId, blocks: [block], streaming: true })
    return out
  }
  const msg = messages[idx]
  const blocks = [...(msg.blocks || [])]
  const existing = block.blockId ? blocks.findIndex((b) => b.blockId === block.blockId) : -1
  if (existing === -1) blocks.push(block)
  else blocks[existing] = block
  const out = [...messages]
  out[idx] = { ...msg, blocks }
  return out
}

// finalizeTurn — the terminal result landed for turnId. Drops status
// lines, settles the turn's bot message on the canonical block list:
//   resp.blocks (terminal frame is the authority — same content as the
//   streamed frames, R9) → else the blocks streamed so far → else a
//   legacy single-document turn synthesizes ONE document block
//   (back-compat) → else the turn produced nothing visible and the
//   pending message is removed.
export function finalizeTurn(messages, turnId, resp) {
  let out = messages.filter((m) => m.role !== 'status')
  const idx = out.findIndex((m) => m.role === 'bot' && m.turnId === turnId)
  const streamed = idx === -1 ? [] : out[idx].blocks || []
  let blocks = Array.isArray(resp?.blocks) && resp.blocks.length > 0 ? resp.blocks : streamed
  if (blocks.length === 0 && resp?.document) {
    blocks = [
      {
        blockId: `turn-${turnId}-document`,
        kind: 'document',
        document: resp.document,
        display: 'screen',
      },
    ]
  }
  if (blocks.length === 0) {
    if (idx !== -1) out = out.filter((_, i) => i !== idx)
    return out
  }
  const settled = { role: 'bot', turnId, blocks }
  if (idx === -1) return [...out, settled]
  out = [...out]
  out[idx] = settled
  return out
}

// replaceBlockInMessages — a server-declared apply {target:"block",
// blockId, document} (§4.8, e.g. success_plaque) swaps ONE block's
// document in place, wherever it sits in the history.
export function replaceBlockInMessages(messages, blockId, document) {
  if (!blockId) return messages
  let touched = false
  const out = messages.map((m) => {
    if (m.role !== 'bot' || !Array.isArray(m.blocks)) return m
    const bIdx = m.blocks.findIndex((b) => b.blockId === blockId)
    if (bIdx === -1) return m
    touched = true
    const blocks = [...m.blocks]
    blocks[bIdx] = { ...blocks[bIdx], kind: 'document', text: undefined, document }
    return { ...m, blocks }
  })
  return touched ? out : messages
}

// transcriptToMessages — maps the GET /onboard/session resume payload's
// transcript entries onto chat messages, defensively: unknown roles
// render as bot text, entries may carry text and/or blocks.
export function transcriptToMessages(transcript) {
  if (!Array.isArray(transcript)) return []
  const out = []
  for (const t of transcript) {
    if (!t || typeof t !== 'object') continue
    if (t.role === 'user') {
      if (typeof t.text === 'string' && t.text !== '') out.push({ role: 'user', text: t.text })
      continue
    }
    if (Array.isArray(t.blocks) && t.blocks.length > 0) {
      out.push({ role: 'bot', blocks: t.blocks })
    } else if (typeof t.text === 'string' && t.text !== '') {
      out.push({ role: 'bot', text: t.text })
    }
  }
  return out
}

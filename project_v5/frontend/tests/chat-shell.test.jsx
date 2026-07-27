import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, fireEvent, screen, waitFor } from '@testing-library/react'
import ChatShell from '../src/shells/ChatShell'

// The chat-first shell (§5.1) over the streamed turn protocol (§4.7 +
// final owner decision 3): interleaved text and inline document blocks
// arrive one by one via `event: block` frames and render in REAL
// arrival order inside the chat column; a legacy single-document turn
// (no blocks) still renders as one inline document (back-compat).

function sseResponse(wire) {
  const encoder = new TextEncoder()
  let sent = false
  return {
    ok: true,
    status: 200,
    body: {
      getReader: () => ({
        read: async () => {
          if (sent) return { done: true, value: undefined }
          sent = true
          return { done: false, value: encoder.encode(wire) }
        },
        cancel: async () => {},
      }),
    },
  }
}

const TEXT_BLOCK = { blockId: 'b1', kind: 'text', text: 'Here is your uploader:' }
const DOC_BLOCK = {
  blockId: 'b2',
  kind: 'document',
  display: 'inline',
  document: {
    version: '2.10',
    children: [{ type: 'text', id: 't1', content: 'Universal uploader', format: {} }],
  },
}
const TAIL_BLOCK = { blockId: 'b3', kind: 'text', text: 'Say ok to apply the plan.' }

function turnWire(blocks, result) {
  return (
    'event: stage\ndata: {"phase":"data_start"}\n\n' +
    blocks.map((b) => `event: block\ndata: ${JSON.stringify(b)}\n\n`).join('') +
    `event: result\ndata: ${JSON.stringify(result)}\n\n`
  )
}

beforeEach(() => {
  globalThis.fetch = vi.fn()
})

afterEach(() => {
  vi.restoreAllMocks()
})

function renderShell(extraProps = {}) {
  return render(
    <ChatShell
      apiBaseUrl="http://api/v1"
      tenantSlug="keepstar-onboarding"
      sessionId="s-1"
      variant="onboarding"
      placeholder="Describe your business…"
      {...extraProps}
    />,
  )
}

async function send(text) {
  fireEvent.change(screen.getByPlaceholderText('Describe your business…'), { target: { value: text } })
  fireEvent.click(screen.getByRole('button', { name: 'Send' }))
}

describe('ChatShell blocks turn', () => {
  it('renders interleaved text + inline document blocks in arrival order', async () => {
    const blocks = [TEXT_BLOCK, DOC_BLOCK, TAIL_BLOCK]
    fetch.mockResolvedValue(sseResponse(turnWire(blocks, { blocks, latencyMs: 10 })))

    renderShell()
    await send('I run a realtor agency')

    await screen.findByText('Say ok to apply the plan.')
    expect(screen.getByText('Here is your uploader:')).toBeTruthy()
    // The document block rendered INSIDE the chat column via InlineBlock.
    const inline = document.querySelector('.kw-inline-block')
    expect(inline).not.toBeNull()
    expect(inline.getAttribute('data-block-id')).toBe('b2')
    expect(inline.textContent).toContain('Universal uploader')

    // Real arrival order: text bubble → inline document → text bubble.
    const turn = document.querySelector('.kw-turn')
    const kinds = Array.from(turn.children).map((el) =>
      el.classList.contains('kw-inline-block') ? 'document' : 'text',
    )
    expect(kinds).toEqual(['text', 'document', 'text'])
  })

  it('back-compat: a single-document turn without blocks renders one inline document', async () => {
    fetch.mockResolvedValue(
      sseResponse(
        turnWire([], {
          document: {
            version: '2.10',
            children: [{ type: 'text', id: 't9', content: 'Lead table view', format: {} }],
          },
          latencyMs: 10,
        }),
      ),
    )

    renderShell()
    await send('any new leads?')

    await waitFor(() => {
      expect(document.querySelector('.kw-inline-block')).not.toBeNull()
    })
    expect(document.querySelector('.kw-inline-block').textContent).toContain('Lead table view')
  })

  it('surfaces a pipeline failure as an error bubble and re-enables the input', async () => {
    fetch.mockResolvedValue(
      sseResponse(
        'event: stage\ndata: {"phase":"data_start"}\n\n' +
          'event: error\ndata: {"status":500,"message":"pipeline failed"}\n\n',
      ),
    )

    renderShell()
    await send('boom')

    await screen.findByText('pipeline 500: pipeline failed')
    expect(screen.getByPlaceholderText('Describe your business…').disabled).toBe(false)
  })
})

// Contextual chips (owner feedback 2026-07-28): chips render only when
// relevant — nothing on a fresh page, "Accept the plan" only while the
// manifest field on the pipeline response (or session resume) shows
// staged steps not yet applied.
describe('ChatShell contextual chips', () => {
  const CHIPS = [
    { label: 'Accept the plan', send: 'apply it', when: 'plan-unapplied' },
    { label: 'Show the plan', send: 'show it', when: 'after-first-turn' },
  ]

  it('renders no chips on a fresh page (seeded greeting only)', () => {
    renderShell({
      quickActions: CHIPS,
      initialMessages: [{ role: 'bot', text: 'Tell me about your business.' }],
    })
    expect(screen.queryByRole('button', { name: 'Accept the plan' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Show the plan' })).toBeNull()
  })

  it('shows Accept after a turn whose response manifest has staged > applied; hides it once applied', async () => {
    const blocks = [TEXT_BLOCK]
    fetch.mockResolvedValueOnce(
      sseResponse(turnWire(blocks, { blocks, manifest: { staged: 3, applied: 0 }, latencyMs: 5 })),
    )

    renderShell({ quickActions: CHIPS })
    await send('I run a realtor agency')

    await screen.findByRole('button', { name: 'Accept the plan' })
    expect(screen.getByRole('button', { name: 'Show the plan' })).toBeTruthy()

    // The apply turn settles the manifest: staged === applied → the
    // Accept chip must leave the screen, Show-the-plan stays.
    const doneBlocks = [{ blockId: 'b9', kind: 'text', text: 'Applied — everything is assembled.' }]
    fetch.mockResolvedValueOnce(
      sseResponse(
        turnWire(doneBlocks, { blocks: doneBlocks, manifest: { staged: 3, applied: 3 }, latencyMs: 5 }),
      ),
    )
    await send('apply it')
    await screen.findByText('Applied — everything is assembled.')

    expect(screen.queryByRole('button', { name: 'Accept the plan' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Show the plan' })).toBeTruthy()
  })

  it('keeps the staged plan visible across a turn whose response carries no manifest field', async () => {
    const blocks = [TEXT_BLOCK]
    fetch.mockResolvedValueOnce(
      sseResponse(turnWire(blocks, { blocks, manifest: { staged: 2, applied: 0 }, latencyMs: 5 })),
    )
    renderShell({ quickActions: CHIPS })
    await send('I run a realtor agency')
    await screen.findByRole('button', { name: 'Accept the plan' })

    const chatBlocks = [{ blockId: 'b8', kind: 'text', text: 'It works for any business.' }]
    fetch.mockResolvedValueOnce(sseResponse(turnWire(chatBlocks, { blocks: chatBlocks, latencyMs: 5 })))
    await send('what is this runtime?')
    await screen.findByText('It works for any business.')

    expect(screen.getByRole('button', { name: 'Accept the plan' })).toBeTruthy()
  })

  it('seeds chip visibility from the resumed manifest (full steps shape) before any turn', () => {
    renderShell({
      quickActions: CHIPS,
      initialMessages: [
        { role: 'user', text: 'I run a realtor agency' },
        { role: 'bot', text: 'Here is the plan so far.' },
      ],
      initialManifest: { steps: [{ id: '1', op: 'create_tenant', status: 'proposed' }] },
    })
    expect(screen.getByRole('button', { name: 'Accept the plan' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Show the plan' })).toBeTruthy()
  })
})

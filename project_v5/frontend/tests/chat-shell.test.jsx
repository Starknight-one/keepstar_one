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

function renderShell() {
  return render(
    <ChatShell
      apiBaseUrl="http://api/v1"
      tenantSlug="keepstar-onboarding"
      sessionId="s-1"
      variant="onboarding"
      placeholder="Describe your business…"
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

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, fireEvent, screen } from '@testing-library/react'
import ChatShell from '../src/shells/ChatShell'
import { splitTurnStream } from '../src/canvas/CanvasChatView'

// The V2 builder layout (V2_SPEC §2 step 3): the canvas is the full
// stage and the chat floats over it as a bottom dock. WHY these
// assertions matter: the whole flow rests on "render, don't narrate"
// (L1) — anything renderable must land on the canvas, never be
// summarized in a chat bubble; and the dock must not become a second
// canvas (documents in the dock would shrink the stage to nothing).

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

const TEXT_BLOCK = { blockId: 'b1', kind: 'text', text: 'Here is your workspace:' }
const DOC_BLOCK = {
  blockId: 'b2',
  kind: 'document',
  display: 'inline',
  document: {
    version: '2.10',
    children: [{ type: 'text', id: 't1', content: 'Registration form', format: {} }],
  },
}

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

function renderCanvasShell(extraProps = {}) {
  return render(
    <ChatShell
      apiBaseUrl="http://api/v1"
      tenantSlug="keepstar-onboarding"
      sessionId="s-1"
      variant="onboarding"
      layout="canvas"
      placeholder="Describe your business…"
      {...extraProps}
    />,
  )
}

describe('splitTurnStream', () => {
  it('routes document blocks to the canvas and text blocks to the dock', () => {
    const { canvasBlocks, chatItems } = splitTurnStream([
      { role: 'user', text: 'I run a realtor agency' },
      { role: 'bot', turnId: 1, blocks: [TEXT_BLOCK, DOC_BLOCK] },
    ])
    expect(canvasBlocks.map((b) => b.block.blockId)).toEqual(['b2'])
    expect(chatItems.map((i) => i.text)).toEqual([
      'I run a realtor agency',
      'Here is your workspace:',
    ])
  })

  it('keeps plain user/status/error lines in the dock', () => {
    const { canvasBlocks, chatItems } = splitTurnStream([
      { role: 'status', text: 'Designing…' },
      { role: 'error', text: 'pipeline 500' },
    ])
    expect(canvasBlocks).toEqual([])
    expect(chatItems.map((i) => i.role)).toEqual(['status', 'error'])
  })
})

describe('ChatShell canvas layout', () => {
  it('renders documents on the stage and chat lines in the floating dock', async () => {
    const blocks = [TEXT_BLOCK, DOC_BLOCK]
    fetch.mockResolvedValue(sseResponse(turnWire(blocks, { blocks, latencyMs: 10 })))

    renderCanvasShell()
    fireEvent.change(screen.getByPlaceholderText('Describe your business…'), {
      target: { value: 'I run a realtor agency' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Send' }))

    await screen.findByText('Here is your workspace:')

    // The rendered document is ON the canvas — not inside the dock.
    const onStage = document.querySelector('.kw-canvas-pane .kw-inline-block')
    expect(onStage).not.toBeNull()
    expect(onStage.getAttribute('data-block-id')).toBe('b2')
    expect(document.querySelector('.kw-dock .kw-inline-block')).toBeNull()

    // The text block is a dock bubble, and the composer docks with it.
    const dock = document.querySelector('.kw-dock')
    expect(dock.textContent).toContain('Here is your workspace:')
    expect(dock.querySelector('.kw-chatpage-input')).not.toBeNull()

    // The dock floats OVER the stage — both are children of one canvas.
    const canvas = document.querySelector('.kw-canvas')
    expect(canvas.querySelector('.kw-canvas-stage')).not.toBeNull()
    expect(canvas.querySelector('.kw-dock')).not.toBeNull()
  })

  it('shows the empty stage message before anything has been assembled', () => {
    renderCanvasShell({ initialMessages: [{ role: 'bot', text: 'Tell me about your business.' }] })
    expect(screen.getByText('Your workspace will be assembled here.')).toBeTruthy()
    // The greeting still lives in the dock.
    expect(document.querySelector('.kw-dock-history').textContent).toContain(
      'Tell me about your business.',
    )
  })
})

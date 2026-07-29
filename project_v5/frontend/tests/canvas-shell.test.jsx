import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, fireEvent, screen } from '@testing-library/react'
import ChatShell from '../src/shells/ChatShell'
import { splitTurnStream, thinkingLabel } from '../src/canvas/CanvasChatView'
import { VERSION } from '../src/canvas/version'

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

// The indicator is a PLACEHOLDER, never an answer (L1): it must vanish
// the moment the turn can show something real on the canvas.
describe('thinkingLabel', () => {
  const IN_FLIGHT = [
    { role: 'user', text: 'I run a realtor agency' },
    { role: 'status', text: 'Designing…' },
  ]

  it('is off when nothing is in flight', () => {
    expect(thinkingLabel(IN_FLIGHT, false)).toBeNull()
  })

  it('reports the live pipeline phase while the turn runs', () => {
    expect(thinkingLabel(IN_FLIGHT, true)).toBe('Designing…')
    const composing = [
      { role: 'user', text: 'I run a realtor agency' },
      { role: 'status', text: 'Composing a response…' },
    ]
    expect(thinkingLabel(composing, true)).toBe('Composing a response…')
  })

  it('hands off as soon as the turn streams its first document block', () => {
    const withDoc = [
      { role: 'user', text: 'I run a realtor agency' },
      { role: 'bot', turnId: 1, blocks: [TEXT_BLOCK, DOC_BLOCK] },
      { role: 'status', text: 'Composing a response…' },
    ]
    expect(thinkingLabel(withDoc, true)).toBeNull()
  })

  it('stays up while the turn has only produced chat text', () => {
    const textOnly = [
      { role: 'user', text: 'I run a realtor agency' },
      { role: 'bot', turnId: 1, blocks: [TEXT_BLOCK] },
      { role: 'status', text: 'Composing a response…' },
    ]
    expect(thinkingLabel(textOnly, true)).toBe('Composing a response…')
  })

  it('ignores documents from earlier turns', () => {
    const secondTurn = [
      { role: 'bot', turnId: 1, blocks: [DOC_BLOCK] },
      { role: 'user', text: 'now add a CRM' },
      { role: 'status', text: 'Designing…' },
    ]
    expect(thinkingLabel(secondTurn, true)).toBe('Designing…')
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

  it('holds the thinking indicator on the canvas while a turn is in flight', async () => {
    let release
    fetch.mockReturnValue(new Promise((resolve) => (release = resolve)))

    renderCanvasShell()
    fireEvent.change(screen.getByPlaceholderText('Describe your business…'), {
      target: { value: 'I run a realtor agency' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Send' }))

    // The in-flight state lives where the content will land, not as a
    // second chat bubble.
    await screen.findByText('Designing…')
    expect(document.querySelector('.kw-canvas-pane .kw-think')).not.toBeNull()
    expect(document.querySelector('.kw-dock-history').textContent).not.toContain('Designing…')

    release(sseResponse(turnWire([], { blocks: [], latencyMs: 1 })))
  })

  it('shows the empty stage message before anything has been assembled', () => {
    renderCanvasShell({ initialMessages: [{ role: 'bot', text: 'Tell me about your business.' }] })
    expect(screen.getByText('Your workspace will be assembled here.')).toBeTruthy()
    // The greeting still lives in the dock.
    expect(document.querySelector('.kw-dock-history').textContent).toContain(
      'Tell me about your business.',
    )
  })

  // We test on prod: the corner stamp is how anyone can tell WHICH build
  // they are looking at, so it must come from the one constant that
  // VERSIONS.md documents — never a second hardcoded string.
  it('stamps the live version in the canvas corner from the single source', () => {
    renderCanvasShell()
    const stamp = document.querySelector('.kw-canvas .kw-canvas-version')
    expect(stamp).not.toBeNull()
    expect(stamp.textContent).toBe(VERSION)
  })
})

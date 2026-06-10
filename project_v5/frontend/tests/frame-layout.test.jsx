import { describe, expect, it } from 'vitest'
import { render } from '@testing-library/react'
import SceneGraphRenderer from '../src/renderer/SceneGraphRenderer'

// Chunk 12 — Frame.jsx supports `layout.wrap` and node-level
// `width / minWidth / maxWidth`. The grid card-presets emit these to
// produce a sensible row-of-cards instead of stacking vertically.

describe('Frame layout — wrap', () => {
  it('layout.wrap=true → data-wrap="true" on the rendered frame', () => {
    const doc = {
      version: '2.10',
      children: [
        {
          type: 'frame',
          id: 'grid',
          layout: { direction: 'row', wrap: true, gap: 'md' },
          children: [],
        },
      ],
    }
    const { container } = render(<SceneGraphRenderer document={doc} />)
    const frame = container.querySelector('[data-id="grid"]')
    expect(frame).not.toBeNull()
    expect(frame.getAttribute('data-wrap')).toBe('true')
    expect(frame.getAttribute('data-direction')).toBe('row')
  })

  it('layout.wrap missing → no data-wrap attribute', () => {
    const doc = {
      version: '2.10',
      children: [
        {
          type: 'frame',
          id: 'plain',
          layout: { direction: 'column' },
          children: [],
        },
      ],
    }
    const { container } = render(<SceneGraphRenderer document={doc} />)
    const frame = container.querySelector('[data-id="plain"]')
    // Empty / null when missing — Frame.jsx emits "" when wrap is falsy.
    expect(frame.getAttribute('data-wrap')).toBe('')
  })
})

describe('Frame layout — width / maxWidth / minWidth', () => {
  it('numeric width → "<n>px" inline style', () => {
    const doc = {
      version: '2.10',
      children: [
        {
          type: 'frame',
          id: 'card',
          width: 280,
          children: [],
        },
      ],
    }
    const { container } = render(<SceneGraphRenderer document={doc} />)
    const frame = container.querySelector('[data-id="card"]')
    expect(frame.style.width).toBe('280px')
  })

  it('string width passes through (units preserved)', () => {
    const doc = {
      version: '2.10',
      children: [
        {
          type: 'frame',
          id: 'fluid',
          width: '50%',
          children: [],
        },
      ],
    }
    const { container } = render(<SceneGraphRenderer document={doc} />)
    const frame = container.querySelector('[data-id="fluid"]')
    expect(frame.style.width).toBe('50%')
  })

  it('maxWidth + minWidth applied alongside width', () => {
    const doc = {
      version: '2.10',
      children: [
        {
          type: 'frame',
          id: 'detail',
          maxWidth: 720,
          minWidth: 240,
          children: [],
        },
      ],
    }
    const { container } = render(<SceneGraphRenderer document={doc} />)
    const frame = container.querySelector('[data-id="detail"]')
    expect(frame.style.maxWidth).toBe('720px')
    expect(frame.style.minWidth).toBe('240px')
  })

  it('no sizing → no inline width / maxWidth / minWidth', () => {
    const doc = {
      version: '2.10',
      children: [
        {
          type: 'frame',
          id: 'auto',
          children: [],
        },
      ],
    }
    const { container } = render(<SceneGraphRenderer document={doc} />)
    const frame = container.querySelector('[data-id="auto"]')
    expect(frame.style.width).toBe('')
    expect(frame.style.maxWidth).toBe('')
    expect(frame.style.minWidth).toBe('')
  })
})

// Component-vocabulary PR1 — scroll strip + collapsible substrates.

describe('Frame layout — horizontal scroll strip', () => {
  it("layout.scroll='x' → data-scroll=\"x\" on the rendered frame", () => {
    const doc = {
      version: '2.10',
      children: [
        {
          type: 'frame',
          id: 'strip',
          layout: { direction: 'row', scroll: 'x', gap: 'md' },
          children: [],
        },
      ],
    }
    const { container } = render(<SceneGraphRenderer document={doc} />)
    const frame = container.querySelector('[data-id="strip"]')
    expect(frame.getAttribute('data-scroll')).toBe('x')
  })

  it('layout.scroll missing → empty data-scroll', () => {
    const doc = {
      version: '2.10',
      children: [
        {
          type: 'frame',
          id: 'static',
          layout: { direction: 'row' },
          children: [],
        },
      ],
    }
    const { container } = render(<SceneGraphRenderer document={doc} />)
    const frame = container.querySelector('[data-id="static"]')
    expect(frame.getAttribute('data-scroll')).toBe('')
  })
})

describe('Frame — collapsible', () => {
  const collapsibleDoc = (extra = {}) => ({
    version: '2.10',
    children: [
      {
        type: 'frame',
        id: 'section',
        collapsible: true,
        layout: { direction: 'column', gap: 'sm' },
        children: [
          { type: 'text', id: 'head', content: 'Description' },
          { type: 'text', id: 'body-1', content: 'First paragraph' },
          { type: 'text', id: 'body-2', content: 'Second paragraph' },
        ],
        ...extra,
      },
    ],
  })

  it('renders <details>/<summary>: first child as header, rest in the body', () => {
    const { container } = render(
      <SceneGraphRenderer document={collapsibleDoc()} />,
    )
    const details = container.querySelector('details.kw-collapsible')
    expect(details).not.toBeNull()
    expect(details.getAttribute('data-id')).toBe('section')

    const summary = details.querySelector('summary.kw-summary')
    expect(summary).not.toBeNull()
    expect(summary.querySelector('[data-id="head"]')?.textContent).toBe(
      'Description',
    )

    const body = details.querySelector('.kw-collapsible-body')
    expect(body).not.toBeNull()
    expect(body.getAttribute('data-gap')).toBe('sm')
    expect(body.querySelector('[data-id="head"]')).toBeNull()
    expect(body.querySelector('[data-id="body-1"]')?.textContent).toBe(
      'First paragraph',
    )
    expect(body.querySelector('[data-id="body-2"]')?.textContent).toBe(
      'Second paragraph',
    )
  })

  it('defaultOpen=true sets the open attribute', () => {
    const { container } = render(
      <SceneGraphRenderer document={collapsibleDoc({ defaultOpen: true })} />,
    )
    const details = container.querySelector('details.kw-collapsible')
    expect(details.hasAttribute('open')).toBe(true)
  })

  it('no defaultOpen → closed by default', () => {
    const { container } = render(
      <SceneGraphRenderer document={collapsibleDoc()} />,
    )
    const details = container.querySelector('details.kw-collapsible')
    expect(details.hasAttribute('open')).toBe(false)
  })
})

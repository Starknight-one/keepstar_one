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

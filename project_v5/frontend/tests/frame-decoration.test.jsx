import { describe, expect, it } from 'vitest'
import { render } from '@testing-library/react'
import SceneGraphRenderer from '../src/renderer/SceneGraphRenderer'

// Product-card redesign — capability layer. Frames gained visual
// decoration (fill / cornerRadius / stroke / effect shadow / clip /
// layout.padding) and absolute-overlay support; leaves (rectangle / text /
// image / icon_font) can be positioned overlays; images can round corners.
//
// The load-bearing contract these tests pin:
//   - an absolutely-positioned child gets position:absolute and its
//     parent frame becomes position:relative (overlay anchoring), and
//   - a frame carrying NONE of the new props renders with no inline style
//     (byte-identical to before — backward compatibility).

function renderDoc(node) {
  return render(
    <SceneGraphRenderer document={{ version: '2.10', children: [node] }} />,
  )
}

describe('Frame decoration', () => {
  it('fill / cornerRadius / stroke / shadow / clip / padding → inline style', () => {
    const { container } = renderDoc({
      type: 'frame',
      id: 'card',
      fill: '#FFFFFF',
      cornerRadius: 16,
      stroke: { thickness: 1, fill: '#ECECEC' },
      effect: [{ type: 'shadow', offset: { x: 0, y: 2 }, blur: 12, color: '#00000010' }],
      clip: true,
      layout: { direction: 'column', padding: [10, 12, 14, 16] },
      children: [],
    })
    const frame = container.querySelector('[data-id="card"]')
    expect(frame.style.background).toContain('rgb(255, 255, 255)')
    expect(frame.style.borderRadius).toBe('16px')
    expect(frame.style.border).toContain('1px solid')
    expect(frame.style.boxShadow).toContain('0px 2px 12px')
    expect(frame.style.overflow).toBe('hidden')
    // top/right/bottom/left all differ so jsdom keeps the 4-value shorthand.
    expect(frame.style.padding).toBe('10px 12px 14px 16px')
    // Flex behavior intact.
    expect(frame.getAttribute('data-direction')).toBe('column')
  })

  it('cornerRadius "full" → pill radius; [v,h] padding shorthand', () => {
    const { container } = renderDoc({
      type: 'frame',
      id: 'badge',
      fill: '#FF9800',
      cornerRadius: 'full',
      layout: { padding: [2, 10] },
      children: [],
    })
    const frame = container.querySelector('[data-id="badge"]')
    expect(frame.style.borderRadius).toBe('9999px')
    expect(frame.style.padding).toBe('2px 10px')
  })

  it('a plain frame (no new props) carries NO inline style', () => {
    const { container } = renderDoc({
      type: 'frame',
      id: 'plain',
      layout: { direction: 'column', gap: 'sm' },
      children: [],
    })
    const frame = container.querySelector('[data-id="plain"]')
    expect(frame.getAttribute('style')).toBeNull()
  })
})

describe('Absolute overlays', () => {
  it('absolute child → position:absolute + offsets; parent → relative', () => {
    const { container } = renderDoc({
      type: 'frame',
      id: 'hero',
      layout: { direction: 'column' },
      children: [
        { type: 'image', id: 'img', fills: [{ url: 'https://img.test/x.jpg' }] },
        {
          type: 'frame',
          id: 'like-slot',
          position: 'absolute',
          top: 8,
          right: 8,
          zIndex: 2,
          children: [],
        },
      ],
    })
    const parent = container.querySelector('[data-id="hero"]')
    expect(parent.style.position).toBe('relative')

    const overlay = container.querySelector('[data-id="like-slot"]')
    expect(overlay.style.position).toBe('absolute')
    expect(overlay.style.top).toBe('8px')
    expect(overlay.style.right).toBe('8px')
    expect(overlay.style.zIndex).toBe('2')
  })

  it('parent WITHOUT an absolute child is not forced relative', () => {
    const { container } = renderDoc({
      type: 'frame',
      id: 'flow',
      layout: { direction: 'column' },
      children: [{ type: 'text', id: 't', content: 'hi' }],
    })
    const parent = container.querySelector('[data-id="flow"]')
    expect(parent.style.position).toBe('')
  })

  it('string offsets pass through (e.g. percentages)', () => {
    const { container } = renderDoc({
      type: 'frame',
      id: 'p',
      children: [
        {
          type: 'rectangle',
          id: 'overlay-rect',
          position: 'absolute',
          bottom: '-16px',
          right: '12px',
          fills: '#000000',
          width: 32,
          height: 32,
        },
      ],
    })
    const rect = container.querySelector('[data-id="overlay-rect"]')
    expect(rect.style.position).toBe('absolute')
    expect(rect.style.bottom).toBe('-16px')
    expect(rect.style.right).toBe('12px')
  })

  it('absolute Text and IconFont overlays get position:absolute', () => {
    const { container } = renderDoc({
      type: 'frame',
      id: 'p',
      children: [
        {
          type: 'text',
          id: 'badge-text',
          content: 'Sale',
          position: 'absolute',
          top: 10,
          left: 10,
        },
        {
          type: 'icon_font',
          id: 'star-overlay',
          iconFontName: 'star',
          position: 'absolute',
          top: 4,
          right: 4,
        },
      ],
    })
    expect(container.querySelector('[data-id="badge-text"]').style.position).toBe(
      'absolute',
    )
    expect(container.querySelector('[data-id="star-overlay"]').style.position).toBe(
      'absolute',
    )
  })
})

describe('Image cornerRadius', () => {
  it('number cornerRadius → borderRadius', () => {
    const { container } = renderDoc({
      type: 'image',
      id: 'hero-img',
      fills: [{ url: 'https://img.test/x.jpg' }],
      cornerRadius: 12,
    })
    const img = container.querySelector('[data-id="hero-img"]')
    expect(img.style.borderRadius).toBe('12px')
  })

  it('4-corner radius array → top-corners-only shorthand', () => {
    const { container } = renderDoc({
      type: 'image',
      id: 'hero-img',
      fills: [{ url: 'https://img.test/x.jpg' }],
      cornerRadius: [16, 16, 0, 0],
    })
    const img = container.querySelector('[data-id="hero-img"]')
    expect(img.style.borderRadius).toBe('16px 16px 0px 0px')
  })

  it('no cornerRadius → no inline style on the image', () => {
    const { container } = renderDoc({
      type: 'image',
      id: 'plain-img',
      fills: [{ url: 'https://img.test/x.jpg' }],
    })
    const img = container.querySelector('[data-id="plain-img"]')
    expect(img.getAttribute('style')).toBeNull()
  })
})

describe('Graceful degradation — empty optional fields', () => {
  it('a bound text that resolved to empty renders nothing (no empty <p>)', () => {
    const { container } = renderDoc({
      type: 'frame',
      id: 'wrap',
      children: [
        // badge field absent → content undefined → renders null
        { type: 'text', id: 'badge-text', fieldBinding: 'badge' },
        { type: 'text', id: 'title', content: 'Real Title' },
      ],
    })
    expect(container.querySelector('[data-id="badge-text"]')).toBeNull()
    expect(container.querySelector('[data-id="title"]')?.textContent).toBe(
      'Real Title',
    )
  })

  it('hideWhenEmpty frame disappears when its only bound child is empty', () => {
    const { container } = renderDoc({
      type: 'frame',
      id: 'hero',
      children: [
        {
          type: 'frame',
          id: 'badge',
          hideWhenEmpty: true,
          fill: '#FF9800',
          children: [{ type: 'text', id: 'badge-text', fieldBinding: 'badge' }],
        },
      ],
    })
    // The orange pill must not render at all (no stray empty badge).
    expect(container.querySelector('[data-id="badge"]')).toBeNull()
  })

  it('hideWhenEmpty frame stays when its bound child has content', () => {
    const { container } = renderDoc({
      type: 'frame',
      id: 'hero',
      children: [
        {
          type: 'frame',
          id: 'badge',
          hideWhenEmpty: true,
          fill: '#FF9800',
          children: [
            { type: 'text', id: 'badge-text', fieldBinding: 'badge', content: 'Sale' },
          ],
        },
      ],
    })
    const badge = container.querySelector('[data-id="badge"]')
    expect(badge).not.toBeNull()
    expect(badge.style.background).toContain('rgb(255, 152, 0)')
    expect(container.querySelector('[data-id="badge-text"]')?.textContent).toBe(
      'Sale',
    )
  })
})

describe('Action-kind button styling hook', () => {
  it('actionKind → data-action-kind on the rendered button', () => {
    const { container } = renderDoc({
      type: 'frame',
      id: 'p',
      children: [
        {
          type: 'text',
          id: 'cart-btn',
          content: '+',
          wrapper: 'button',
          actionKind: 'cart_add',
          action: { kind: 'cart_add', entity: { type: 'product', id: 'p-1' } },
        },
      ],
    })
    const btn = container.querySelector('button.kw-button')
    expect(btn.getAttribute('data-action-kind')).toBe('cart_add')
  })

  it('button without actionKind has no data-action-kind attribute', () => {
    const { container } = renderDoc({
      type: 'text',
      id: 'plain-btn',
      content: 'Buy',
      wrapper: 'button',
    })
    const btn = container.querySelector('button.kw-button')
    expect(btn.getAttribute('data-action-kind')).toBeNull()
  })
})

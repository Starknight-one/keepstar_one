import { describe, expect, it, vi } from 'vitest'
import { render } from '@testing-library/react'
import SceneGraphRenderer from '../src/renderer/SceneGraphRenderer'

// Component-vocabulary PR1 — rectangle / line / icon_font leaf nodes.
// All three are decorative leaves passed through by the backend; the
// widget turns fills/cornerRadius/stroke into inline styles.

function renderDoc(node) {
  return render(
    <SceneGraphRenderer document={{ version: '2.10', children: [node] }} />,
  )
}

describe('Rectangle node', () => {
  it('renders background / border-radius / border from fills, cornerRadius, stroke', () => {
    const { container } = renderDoc({
      type: 'rectangle',
      id: 'accent',
      fills: [{ type: 'color', color: '#5BA4D9' }],
      cornerRadius: 8,
      stroke: { thickness: 2, fill: '#F0924A' },
      width: 120,
      height: 40,
      opacity: 0.5,
    })
    const rect = container.querySelector('.kw-rect')
    expect(rect).not.toBeNull()
    expect(rect.style.background).toContain('rgb(91, 164, 217)')
    expect(rect.style.borderRadius).toBe('8px')
    expect(rect.style.border).toContain('2px solid')
    expect(rect.style.width).toBe('120px')
    expect(rect.style.height).toBe('40px')
    expect(rect.style.opacity).toBe('0.5')
  })

  it('accepts a bare hex-string fill and a 4-corner radius array', () => {
    const { container } = renderDoc({
      type: 'rectangle',
      id: 'hex',
      fills: '#ff0000',
      cornerRadius: [4, 8, 12, 16],
    })
    const rect = container.querySelector('.kw-rect')
    expect(rect.style.background).toContain('rgb(255, 0, 0)')
    expect(rect.style.borderRadius).toBe('4px 8px 12px 16px')
  })

  it('ignores gradient fills and renders no children', () => {
    const { container } = renderDoc({
      type: 'rectangle',
      id: 'grad',
      fills: [{ type: 'gradient', stops: [] }],
      children: [{ type: 'text', id: 'never', content: 'never' }],
    })
    const rect = container.querySelector('.kw-rect')
    expect(rect).not.toBeNull()
    expect(rect.style.background).toBe('')
    expect(rect.children.length).toBe(0)
  })
})

describe('Line node', () => {
  it('default horizontal divider: thickness → height, color, full width', () => {
    const { container } = renderDoc({
      type: 'line',
      id: 'div1',
      stroke: { thickness: 2, fill: '#ff0000' },
    })
    const line = container.querySelector('.kw-line')
    expect(line).not.toBeNull()
    expect(line.getAttribute('role')).toBe('separator')
    expect(line.style.height).toBe('2px')
    expect(line.style.width).toBe('100%')
    expect(line.style.background).toContain('rgb(255, 0, 0)')
  })

  it('falls back to 1px thickness and the --kw-line token', () => {
    const { container } = renderDoc({ type: 'line', id: 'bare' })
    const line = container.querySelector('.kw-line')
    expect(line.style.height).toBe('1px')
    expect(line.style.background).toContain('var(--kw-line, #e5e7eb)')
  })

  it('height > width → vertical divider (dimensions swap)', () => {
    const { container } = renderDoc({
      type: 'line',
      id: 'vert',
      width: 1,
      height: 40,
      stroke: { thickness: 3 },
    })
    const line = container.querySelector('.kw-line')
    expect(line.style.width).toBe('3px')
    expect(line.style.height).toBe('40px')
    expect(line.getAttribute('aria-orientation')).toBe('vertical')
  })

  it('numeric width caps a horizontal divider', () => {
    const { container } = renderDoc({ type: 'line', id: 'short', width: 80 })
    const line = container.querySelector('.kw-line')
    expect(line.style.width).toBe('80px')
    expect(line.style.height).toBe('1px')
  })
})

describe('IconFont node', () => {
  it('known name renders an inline <svg> with real path data', () => {
    const { container } = renderDoc({
      type: 'icon_font',
      id: 'fav',
      iconFontName: 'star',
    })
    const svg = container.querySelector('svg.kw-icon')
    expect(svg).not.toBeNull()
    expect(svg.getAttribute('viewBox')).toBe('0 0 24 24')
    const path = svg.querySelector('path')
    expect(path).not.toBeNull()
    expect(path.getAttribute('d')).toBeTruthy()
  })

  it('applies size and fill color, defaulting to 16 / currentColor', () => {
    const { container } = renderDoc({
      type: 'icon_font',
      id: 'cart',
      iconFontName: 'shopping-cart',
      width: 20,
      height: 20,
      fill: '#F0924A',
    })
    const svg = container.querySelector('svg.kw-icon')
    expect(svg.getAttribute('width')).toBe('20')
    expect(svg.getAttribute('height')).toBe('20')
    expect(svg.style.color).toBe('rgb(240, 146, 74)')

    const { container: c2 } = renderDoc({
      type: 'icon_font',
      id: 'check',
      iconFontName: 'check',
    })
    const svg2 = c2.querySelector('svg.kw-icon')
    expect(svg2.getAttribute('width')).toBe('16')
    expect(svg2.getAttribute('height')).toBe('16')
    expect(svg2.getAttribute('stroke')).toBe('currentColor')
    expect(svg2.style.color).toBe('')
  })

  it('unknown name logs a warning and renders nothing', () => {
    const spy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const { container } = renderDoc({
      type: 'icon_font',
      id: 'mystery',
      iconFontName: 'no-such-icon',
    })
    expect(container.querySelector('svg.kw-icon')).toBeNull()
    expect(spy).toHaveBeenCalledWith(
      '[v5-renderer] unknown icon_font name',
      'no-such-icon',
      'mystery',
    )
    spy.mockRestore()
  })
})

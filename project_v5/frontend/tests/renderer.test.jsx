import { describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import SceneGraphRenderer from '../src/renderer/SceneGraphRenderer'

import productGrid from './fixtures/product-card-grid.json'
import productDetail from './fixtures/product-detail.json'
import emptyNotFound from './fixtures/empty-not-found.json'

describe('SceneGraphRenderer — product card grid', () => {
  it('renders three top-level cards bound to product names', () => {
    render(<SceneGraphRenderer document={productGrid} />)
    // Three product names appear as text content.
    expect(screen.getByText('Glow Serum 30ml')).toBeInTheDocument()
    expect(screen.getByText('Hydration Mist')).toBeInTheDocument()
    expect(screen.getByText('Lavender Hand Cream')).toBeInTheDocument()
  })

  it('formats prices via the currency formatter', () => {
    render(<SceneGraphRenderer document={productGrid} />)
    // priceFormatted is "currency"-formatted; backend may have stored
    // either a number (24.5) or a pre-formatted string. Either way the
    // renderer should produce a leading $.
    const matches = screen.getAllByText(/\$24/)
    expect(matches.length).toBeGreaterThan(0)
  })

  it('formats compact star rating', () => {
    render(<SceneGraphRenderer document={productGrid} />)
    expect(screen.getByText('★ 4.5')).toBeInTheDocument()
  })

  it('renders brand wrapper as a badge span', () => {
    const { container } = render(<SceneGraphRenderer document={productGrid} />)
    const badge = container.querySelector('.kw-badge')
    expect(badge).not.toBeNull()
    expect(badge?.textContent).toBe('Hey Babes')
  })

  it('renders three product images with correct aspect-ratio attribute', () => {
    const { container } = render(<SceneGraphRenderer document={productGrid} />)
    const imgs = container.querySelectorAll('img.kw-image')
    expect(imgs.length).toBe(2) // 2 cards have images; card-3 has none
    expect(imgs[0].getAttribute('data-aspect')).toBe('4:3')
    expect(imgs[0].getAttribute('src')).toContain('picsum.photos')
  })
})

describe('SceneGraphRenderer — product detail', () => {
  it('renders the hero image, title, price, rating, description, CTA', () => {
    const { container } = render(<SceneGraphRenderer document={productDetail} />)
    expect(screen.getByText('Glow Serum 30ml')).toBeInTheDocument()
    expect(screen.getByText(/Lightweight serum/)).toBeInTheDocument()
    // Currency-formatted price.
    expect(screen.getByText(/\$24/)).toBeInTheDocument()
    // Full stars vocabulary on rating.
    expect(screen.getByText(/★/)).toBeInTheDocument()
    // CTA renders as a real <button>.
    const btn = container.querySelector('button.kw-button')
    expect(btn).not.toBeNull()
    expect(btn?.textContent).toBe('Add to cart')
  })

  it('button click logs a no-op TODO marker (P0-C will wire actions)', () => {
    const { container } = render(<SceneGraphRenderer document={productDetail} />)
    const btn = container.querySelector('button.kw-button')
    const spy = vi.spyOn(console, 'log').mockImplementation(() => {})
    fireEvent.click(btn)
    expect(spy).toHaveBeenCalledWith(
      '[v5-action]',
      expect.objectContaining({ wrapper: 'button', hint: expect.stringContaining('TODO') }),
    )
    spy.mockRestore()
  })

  it('hero image carries 16:9 aspect-ratio', () => {
    const { container } = render(<SceneGraphRenderer document={productDetail} />)
    const img = container.querySelector('img.kw-image')
    expect(img?.getAttribute('data-aspect')).toBe('16:9')
  })
})

describe('SceneGraphRenderer — empty state', () => {
  it('renders headline + subtext without any data binding', () => {
    render(<SceneGraphRenderer document={emptyNotFound} />)
    expect(screen.getByText('Nothing matched')).toBeInTheDocument()
    expect(screen.getByText(/Try removing some filters/)).toBeInTheDocument()
  })
})

describe('SceneGraphRenderer — edge cases', () => {
  it('null document renders nothing', () => {
    const { container } = render(<SceneGraphRenderer document={null} />)
    expect(container.firstChild).toBeNull()
  })

  it('empty children renders the inner shell only (no children)', () => {
    const { container } = render(
      <SceneGraphRenderer document={{ version: '2.10', children: [] }} />,
    )
    expect(container.querySelector('.kw-display-inner')?.children.length).toBe(0)
  })

  it('reusable component definitions are filtered out', () => {
    const doc = {
      version: '2.10',
      children: [
        { type: 'frame', id: 'reusable-def', reusable: true, children: [{ type: 'text', content: 'hidden' }] },
        { type: 'text', id: 'visible', content: 'visible' },
      ],
    }
    render(<SceneGraphRenderer document={doc} />)
    expect(screen.queryByText('hidden')).toBeNull()
    expect(screen.getByText('visible')).toBeInTheDocument()
  })

  it('unknown node types log a warning but do not crash', () => {
    const doc = {
      version: '2.10',
      children: [{ type: 'galaxy', id: 'x' }],
    }
    const spy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    render(<SceneGraphRenderer document={doc} />)
    expect(spy).toHaveBeenCalledWith(
      '[v5-renderer] unknown node type',
      'galaxy',
      'x',
    )
    spy.mockRestore()
  })
})

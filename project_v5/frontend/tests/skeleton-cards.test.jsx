import { describe, expect, it } from 'vitest'
import { render } from '@testing-library/react'
import SkeletonCards from '../src/chat/SkeletonCards'

describe('SkeletonCards', () => {
  it('renders exactly N cards, each with an image block and two bars', () => {
    const { container } = render(<SkeletonCards count={4} />)
    const cards = container.querySelectorAll('.kw-skeleton-card')
    expect(cards.length).toBe(4)
    for (const card of cards) {
      expect(card.querySelectorAll('.kw-skeleton-image').length).toBe(1)
      expect(card.querySelectorAll('.kw-skeleton-bar').length).toBe(2)
    }
    expect(container.querySelector('.kw-skeleton-row')).not.toBeNull()
  })

  it('count 0 → empty row', () => {
    const { container } = render(<SkeletonCards count={0} />)
    expect(container.querySelectorAll('.kw-skeleton-card').length).toBe(0)
  })
})

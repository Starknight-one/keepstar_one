import { describe, expect, it } from 'vitest'
import { fillTemplate } from '../src/renderer/fillTemplate'

describe('fillTemplate', () => {
  it('binds text content + image url + value via fieldBinding', () => {
    const tpl = {
      version: '2.10',
      children: [
        {
          type: 'frame',
          id: 'detail',
          children: [
            { type: 'image', id: 'hero', fieldBinding: 'images' },
            { type: 'text', id: 'title', fieldBinding: 'name' },
            { type: 'text', id: 'price', fieldBinding: 'priceFormatted' },
            { type: 'rectangle', id: 'rect', fieldBinding: 'rating' },
          ],
        },
      ],
    }
    const entity = {
      id: 'p-1',
      name: 'Glow Serum',
      priceFormatted: '$24.50',
      images: ['https://cdn/img.png'],
      rating: 4.5,
    }
    const filled = fillTemplate(tpl, entity)
    const detail = filled.children[0]
    const hero = detail.children[0]
    const title = detail.children[1]
    const price = detail.children[2]
    const rect = detail.children[3]
    expect(title.content).toBe('Glow Serum')
    expect(price.content).toBe('$24.50')
    expect(hero.fills[0].url).toBe('https://cdn/img.png')
    expect(hero.fills[0].type).toBe('image')
    expect(hero.fills[0].mode).toBe('fill')
    expect(rect.value).toBe(4.5)
  })

  it('returns a fresh deep clone — does not mutate the source template', () => {
    const tpl = {
      version: '2.10',
      children: [{ type: 'text', id: 't', fieldBinding: 'name' }],
    }
    const entity = { id: 'p-1', name: 'Cream' }
    const filled = fillTemplate(tpl, entity)
    expect(filled).not.toBe(tpl)
    expect(filled.children[0].content).toBe('Cream')
    expect(tpl.children[0].content).toBeUndefined()
  })

  it('skips nodes with __resolvedFrom (already inlined components)', () => {
    const tpl = {
      version: '2.10',
      children: [
        {
          type: 'ref',
          __resolvedFrom: 'price-rating-root',
          children: [{ type: 'text', id: 'inner', fieldBinding: 'price', content: 'inherited' }],
        },
      ],
    }
    const filled = fillTemplate(tpl, { id: 'p-1', price: 999 })
    // The __resolvedFrom guard short-circuits before recursing, so
    // inner.content stays untouched.
    expect(filled.children[0].children[0].content).toBe('inherited')
  })

  it('image fieldBinding can resolve string (not array)', () => {
    const tpl = {
      version: '2.10',
      children: [{ type: 'image', id: 'h', fieldBinding: 'heroImage' }],
    }
    const filled = fillTemplate(tpl, { id: 'p-1', heroImage: 'https://x/y.jpg' })
    expect(filled.children[0].fills[0].url).toBe('https://x/y.jpg')
  })

  it('absent / null entity values are skipped (no __bound stamp)', () => {
    const tpl = {
      version: '2.10',
      children: [{ type: 'text', id: 't', fieldBinding: 'description' }],
    }
    const filled = fillTemplate(tpl, { id: 'p-1' })
    expect(filled.children[0].content).toBeUndefined()
    expect(filled.children[0].__bound).toBeUndefined()
  })

  it('non-object inputs return the template unchanged', () => {
    expect(fillTemplate(null, {})).toBeNull()
    expect(fillTemplate({ version: '2.10' }, null)).toEqual({ version: '2.10' })
  })
})

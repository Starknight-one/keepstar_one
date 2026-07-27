import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, fireEvent, waitFor } from '@testing-library/react'
import SceneGraphRenderer from '../src/renderer/SceneGraphRenderer'
import RenderContext from '../src/renderer/RenderContext'

// Form primitives (RUNTIME_SPEC §5.2 + §4.8 + R6) — input / select /
// datetime / textarea / submit nodes, the FormProvider state module,
// and the two submit paths:
//   operation_invoke → POST {base}/operations/invoke, values as params;
//   form_submit      → POST document-specified same-origin endpoint
//                      (registration — NEVER through operations, R6).

beforeEach(() => {
  globalThis.fetch = vi.fn()
})

afterEach(() => {
  vi.restoreAllMocks()
})

function mkCtx(overrides = {}) {
  return {
    apiBaseUrl: 'http://api/v1',
    tenantSlug: 'demo',
    sessionId: 's-1',
    prefetch: { adjacentTemplate: {}, entities: {} },
    onUpdateDocument: vi.fn(),
    onSearch: vi.fn(),
    ...overrides,
  }
}

function renderDoc(children, ctx = mkCtx()) {
  const utils = render(
    <RenderContext.Provider value={ctx}>
      <SceneGraphRenderer document={{ version: '2.10', children }} />
    </RenderContext.Provider>,
  )
  return { ...utils, ctx }
}

function okResponse(payload) {
  return { ok: true, status: 200, text: async () => JSON.stringify(payload) }
}

// booking_form-shaped fixture: hidden bound id + text + tel + datetime
// + submit via operation_invoke (spec §5.2 "Two v1 forms").
function bookingForm(overrides = {}) {
  return {
    type: 'frame',
    id: 'bf',
    formId: 'booking',
    layout: { direction: 'column', gap: 'md' },
    children: [
      { type: 'input', id: 'lid', name: 'listingId', inputType: 'hidden', fieldBinding: 'id', content: 'prod-1' },
      { type: 'input', id: 'nm', name: 'name', label: 'Your name', required: true },
      { type: 'input', id: 'ph', name: 'phone', inputType: 'tel', label: 'Phone', required: true },
      {
        type: 'datetime',
        id: 'dt',
        name: 'preferredTime',
        mode: 'datetime',
        label: 'Preferred time',
        required: true,
        hours: { from: 9, to: 18 },
      },
      {
        type: 'submit',
        id: 'sb',
        label: 'Book showing',
        loadingLabel: 'Booking…',
        action: { kind: 'operation_invoke', operation: 'book_showing' },
      },
    ],
    ...overrides,
  }
}

describe('form primitive rendering', () => {
  it('renders input/select/datetime/textarea/submit with labels and token classes', () => {
    const { container } = renderDoc([
      {
        type: 'frame',
        id: 'f',
        formId: 'demo-form',
        children: [
          { type: 'input', id: 'i1', name: 'email', inputType: 'email', label: 'Email', required: true },
          {
            type: 'select',
            id: 's1',
            name: 'rooms',
            label: 'Rooms',
            options: [{ value: '2', label: 'Two' }, '3'],
          },
          { type: 'datetime', id: 'd1', name: 'when', mode: 'date', label: 'Date' },
          { type: 'textarea', id: 't1', name: 'notes', label: 'Notes', rows: 3 },
          { type: 'submit', id: 'b1', label: 'Send', action: { kind: 'operation_invoke', operation: 'x' } },
        ],
      },
    ])
    const form = container.querySelector('form.kw-form')
    expect(form).not.toBeNull()
    expect(form.getAttribute('data-form-id')).toBe('demo-form')

    const input = container.querySelector('input.kw-input[type="email"]')
    expect(input).not.toBeNull()
    expect(input.getAttribute('name')).toBe('email')

    const select = container.querySelector('select.kw-select')
    const optionValues = [...select.querySelectorAll('option')].map((o) => o.value)
    expect(optionValues).toEqual(['', '2', '3'])

    expect(container.querySelector('input.kw-input[type="date"]')).not.toBeNull()
    const ta = container.querySelector('textarea.kw-textarea')
    expect(ta.getAttribute('rows')).toBe('3')

    const btn = container.querySelector('button.kw-submit')
    expect(btn).toHaveTextContent('Send')

    // Required label marker on the email field.
    const labels = [...container.querySelectorAll('.kw-field-label')].map((l) => l.textContent)
    expect(labels[0]).toContain('Email')
    expect(labels[0]).toContain('*')
  })

  it('hidden input renders nothing visible', () => {
    const { container } = renderDoc([bookingForm()])
    const inputs = [...container.querySelectorAll('input')]
    expect(inputs.some((i) => i.type === 'hidden')).toBe(false)
    expect(inputs.length).toBe(3) // name, phone, datetime — no hidden control
  })

  it('datetime maps mode → native input type and datetime falls back from unknown modes', () => {
    const { container } = renderDoc([
      {
        type: 'frame',
        id: 'f',
        formId: 'f1',
        children: [
          { type: 'datetime', id: 'd1', name: 'a', mode: 'time' },
          { type: 'datetime', id: 'd2', name: 'b', mode: 'bogus' },
        ],
      },
    ])
    expect(container.querySelector('input[type="time"]')).not.toBeNull()
    expect(container.querySelector('input[type="datetime-local"]')).not.toBeNull()
  })
})

describe('client validation', () => {
  it('required fields block submit — no network call, field errors + form message', async () => {
    const { container } = renderDoc([bookingForm()])
    fireEvent.click(container.querySelector('button.kw-submit'))
    await waitFor(() => {
      expect(container.querySelectorAll('.kw-field-error').length).toBeGreaterThanOrEqual(2)
    })
    expect(fetch).not.toHaveBeenCalled()
    expect(container.querySelector('.kw-form-message[data-status="error"]')).toHaveTextContent(
      'Please fix the highlighted fields.',
    )
    // Field-level messages name their field.
    const errs = [...container.querySelectorAll('.kw-field-error')].map((e) => e.textContent)
    expect(errs).toContain('Your name is required.')
  })

  it('validates email shape on blur', async () => {
    const { container } = renderDoc([
      {
        type: 'frame',
        id: 'f',
        formId: 'reg',
        children: [{ type: 'input', id: 'e', name: 'email', inputType: 'email', label: 'Email' }],
      },
    ])
    const input = container.querySelector('input.kw-input')
    fireEvent.change(input, { target: { value: 'not-an-email' } })
    fireEvent.blur(input)
    await waitFor(() => {
      expect(container.querySelector('.kw-field-error')).toHaveTextContent('Enter a valid email address.')
    })
    expect(input.getAttribute('data-invalid')).toBe('true')
    // Typing clears the error.
    fireEvent.change(input, { target: { value: 'a@b.co' } })
    await waitFor(() => {
      expect(container.querySelector('.kw-field-error')).toBeNull()
    })
  })

  it('enforces the datetime business-hours window from node props', async () => {
    const { container } = renderDoc([bookingForm()])
    fireEvent.change(container.querySelector('input[type="text"]'), { target: { value: 'Ana' } })
    fireEvent.change(container.querySelector('input[type="tel"]'), { target: { value: '+55 11 91234-5678' } })
    const dt = container.querySelector('input[type="datetime-local"]')
    fireEvent.change(dt, { target: { value: '2030-01-01T20:00' } })
    fireEvent.click(container.querySelector('button.kw-submit'))
    await waitFor(() => {
      expect(container.querySelector('.kw-field-error')).toHaveTextContent(
        'Choose a time between 09:00 and 18:00.',
      )
    })
    expect(fetch).not.toHaveBeenCalled()
  })

  it('number inputs validate min/max and coerce to numbers in the payload', async () => {
    fetch.mockResolvedValue(okResponse({ status: 'ok' }))
    const { container } = renderDoc([
      {
        type: 'frame',
        id: 'f',
        formId: 'nf',
        children: [
          { type: 'input', id: 'n', name: 'guests', inputType: 'number', label: 'Guests', validation: { min: 1, max: 10 } },
          { type: 'submit', id: 's', label: 'Go', action: { kind: 'operation_invoke', operation: 'op' } },
        ],
      },
    ])
    const input = container.querySelector('input[type="number"]')
    fireEvent.change(input, { target: { value: '99' } })
    fireEvent.click(container.querySelector('button.kw-submit'))
    await waitFor(() => {
      expect(container.querySelector('.kw-field-error')).toHaveTextContent('Must be at most 10.')
    })
    expect(fetch).not.toHaveBeenCalled()

    fireEvent.change(input, { target: { value: '4' } })
    fireEvent.click(container.querySelector('button.kw-submit'))
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1))
    const body = JSON.parse(fetch.mock.calls[0][1].body)
    expect(body.params.guests).toBe(4) // number, not "4"
  })
})

describe('operation_invoke submit flow', () => {
  it('POSTs collected values (incl. hidden bound id) to /operations/invoke and applies form success', async () => {
    fetch.mockResolvedValue(
      okResponse({
        status: 'ok',
        apply: [{ target: 'form', formId: 'booking', status: 'ok', message: 'Showing booked!' }],
      }),
    )
    const { container } = renderDoc([bookingForm()])
    fireEvent.change(container.querySelector('input[type="text"]'), { target: { value: 'Ana' } })
    fireEvent.change(container.querySelector('input[type="tel"]'), { target: { value: '+5511912345678' } })
    fireEvent.change(container.querySelector('input[type="datetime-local"]'), {
      target: { value: '2030-01-01T14:00' },
    })
    fireEvent.click(container.querySelector('button.kw-submit'))

    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1))
    const [url, init] = fetch.mock.calls[0]
    expect(url).toBe('http://api/v1/operations/invoke')
    expect(init.method).toBe('POST')
    expect(init.headers['X-Tenant-Slug']).toBe('demo')
    const body = JSON.parse(init.body)
    expect(body.sessionId).toBe('s-1')
    expect(body.operation).toBe('book_showing')
    expect(body.formId).toBe('booking')
    expect(body.params).toEqual({
      listingId: 'prod-1',
      name: 'Ana',
      phone: '+5511912345678',
      preferredTime: '2030-01-01T14:00:00', // seconds appended
    })

    await waitFor(() => {
      expect(container.querySelector('.kw-form-message[data-status="success"]')).toHaveTextContent(
        'Showing booked!',
      )
    })
  })

  it('renders the server error message into the form block (apply target form, status error)', async () => {
    fetch.mockResolvedValue(
      okResponse({
        status: 'error',
        apply: [{ target: 'form', formId: 'booking', status: 'error', message: 'That time is in the past.' }],
      }),
    )
    const { container } = renderDoc([bookingForm()])
    fireEvent.change(container.querySelector('input[type="text"]'), { target: { value: 'Ana' } })
    fireEvent.change(container.querySelector('input[type="tel"]'), { target: { value: '+5511912345678' } })
    fireEvent.change(container.querySelector('input[type="datetime-local"]'), {
      target: { value: '2030-01-01T14:00' },
    })
    fireEvent.click(container.querySelector('button.kw-submit'))
    await waitFor(() => {
      expect(container.querySelector('.kw-form-message[data-status="error"]')).toHaveTextContent(
        'That time is in the past.',
      )
    })
  })

  it('apply target block falls back to onUpdateDocument and prefers onReplaceBlock when provided', async () => {
    const plaque = { version: '2.10', children: [{ type: 'text', id: 'p', content: 'Done' }] }
    fetch.mockResolvedValue(
      okResponse({ status: 'ok', apply: [{ target: 'block', blockId: 'b-1', document: plaque }] }),
    )

    // Storefront fallback: no onReplaceBlock → whole-view swap.
    const first = renderDoc([bookingForm()])
    fillBooking(first.container)
    fireEvent.click(first.container.querySelector('button.kw-submit'))
    await waitFor(() => expect(first.ctx.onUpdateDocument).toHaveBeenCalledWith(plaque))

    // Blocks shell: onReplaceBlock wins.
    const onReplaceBlock = vi.fn()
    const second = renderDoc([bookingForm()], mkCtx({ onReplaceBlock }))
    fillBooking(second.container)
    fireEvent.click(second.container.querySelector('button.kw-submit'))
    await waitFor(() => expect(onReplaceBlock).toHaveBeenCalledWith('b-1', plaque))
    expect(second.ctx.onUpdateDocument).not.toHaveBeenCalled()
  })

  it('non-2xx JSON error payloads surface honestly, never as success', async () => {
    fetch.mockResolvedValue({
      ok: false,
      status: 403,
      text: async () => JSON.stringify({ message: 'operation denied' }),
    })
    const { container } = renderDoc([bookingForm()])
    fillBooking(container)
    fireEvent.click(container.querySelector('button.kw-submit'))
    await waitFor(() => {
      expect(container.querySelector('.kw-form-message[data-status="error"]')).toHaveTextContent(
        'operation denied',
      )
    })
  })
})

describe('form_submit flow (R6 — registration path)', () => {
  const registrationForm = {
    type: 'frame',
    id: 'rf',
    formId: 'registration',
    children: [
      { type: 'input', id: 'n', name: 'name', label: 'Name', required: true },
      { type: 'input', id: 'e', name: 'email', inputType: 'email', label: 'Email', required: true },
      { type: 'input', id: 'p', name: 'password', inputType: 'password', label: 'Password', required: true },
      {
        type: 'submit',
        id: 's',
        label: 'Create account',
        action: { kind: 'form_submit', endpoint: '/api/v1/onboard/step/step-7/submit' },
      },
    ],
  }

  it('POSTs values to the step-submit endpoint on the API origin — never /operations/invoke', async () => {
    fetch.mockResolvedValue(
      okResponse({ status: 'ok', apply: [{ target: 'form', formId: 'registration', status: 'ok', message: 'Account created.' }] }),
    )
    const { container } = renderDoc([registrationForm])
    const inputs = container.querySelectorAll('input.kw-input')
    fireEvent.change(inputs[0], { target: { value: 'Vlad' } })
    fireEvent.change(inputs[1], { target: { value: 'v@k.one' } })
    fireEvent.change(inputs[2], { target: { value: 'hunter22' } })
    fireEvent.click(container.querySelector('button.kw-submit'))

    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1))
    const [url, init] = fetch.mock.calls[0]
    expect(url).toBe('http://api/api/v1/onboard/step/step-7/submit')
    expect(url).not.toContain('/operations/invoke')
    expect(init.credentials).toBe('include')
    const body = JSON.parse(init.body)
    expect(body).toEqual({
      sessionId: 's-1',
      formId: 'registration',
      values: { name: 'Vlad', email: 'v@k.one', password: 'hunter22' },
    })
    await waitFor(() => {
      expect(container.querySelector('.kw-form-message[data-status="success"]')).toHaveTextContent(
        'Account created.',
      )
    })
  })

  it('refuses absolute/external form_submit endpoints before any network activity', async () => {
    const doc = {
      ...registrationForm,
      children: registrationForm.children.map((c) =>
        c.type === 'submit' ? { ...c, action: { kind: 'form_submit', endpoint: 'https://evil.example/steal' } } : c,
      ),
    }
    const { container } = renderDoc([doc])
    const inputs = container.querySelectorAll('input.kw-input')
    fireEvent.change(inputs[0], { target: { value: 'Vlad' } })
    fireEvent.change(inputs[1], { target: { value: 'v@k.one' } })
    fireEvent.change(inputs[2], { target: { value: 'hunter22' } })
    const errSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    fireEvent.click(container.querySelector('button.kw-submit'))
    await waitFor(() => {
      expect(container.querySelector('.kw-form-message[data-status="error"]')).not.toBeNull()
    })
    expect(fetch).not.toHaveBeenCalled()
    errSpy.mockRestore()
  })
})

describe('degradation', () => {
  it('a submit node outside a formId frame warns and does not crash or POST', async () => {
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const { container } = renderDoc([
      { type: 'submit', id: 'lone', label: 'Send', action: { kind: 'operation_invoke', operation: 'x' } },
    ])
    fireEvent.click(container.querySelector('button.kw-submit'))
    await Promise.resolve()
    expect(fetch).not.toHaveBeenCalled()
    expect(warnSpy).toHaveBeenCalledWith('[v5-form] submit node outside a formId frame', 'lone')
    warnSpy.mockRestore()
  })

  it('fields without a provider keep local state and still validate on blur', async () => {
    const { container } = renderDoc([
      { type: 'input', id: 'solo', name: 'email', inputType: 'email', label: 'Email' },
    ])
    const input = container.querySelector('input.kw-input')
    fireEvent.change(input, { target: { value: 'nope' } })
    fireEvent.blur(input)
    await waitFor(() => {
      expect(container.querySelector('.kw-field-error')).toHaveTextContent('Enter a valid email address.')
    })
  })
})

function fillBooking(container) {
  fireEvent.change(container.querySelector('input[type="text"]'), { target: { value: 'Ana' } })
  fireEvent.change(container.querySelector('input[type="tel"]'), { target: { value: '+5511912345678' } })
  fireEvent.change(container.querySelector('input[type="datetime-local"]'), {
    target: { value: '2030-01-01T14:00' },
  })
}

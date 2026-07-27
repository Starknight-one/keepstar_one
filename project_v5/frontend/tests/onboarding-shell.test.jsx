import { afterEach, describe, expect, it, vi } from 'vitest'
import { render, fireEvent, screen } from '@testing-library/react'
import OnboardingShell from '../src/shells/OnboardingShell'

// The /onboard page flow (§5.1 + R5): the stored session id resumes via
// GET /onboard/session?sessionId= (refresh-safe, all truth in DB); a 403
// lands on the password gate which posts {password} to /onboard/gate
// (wrong → inline error, right → session create/resume → chat); 503
// renders the disabled page; a stale stored id (404) is discarded and a
// fresh session is created. All onboarding calls carry
// credentials:'include' so the HttpOnly ks_onboard cookie rides along.

const BASE = 'http://api/v1'
const STORE_KEY = 'keepstar-onboard-session'

function routeFetch(routes) {
  return vi.fn(async (url, init = {}) => {
    const method = (init.method || 'GET').toUpperCase()
    const key = `${method} ${url.replace(BASE, '')}`
    const handler = routes[key]
    if (!handler) throw new Error(`unexpected fetch: ${key}`)
    return handler(init)
  })
}

const resp = (status, payload) => ({
  ok: status >= 200 && status < 300,
  status,
  text: async () => (payload === undefined ? '' : JSON.stringify(payload)),
  json: async () => payload,
})

const SESSION = {
  sessionId: 'onb-1',
  mode: 'onboarding',
  tenant: { slug: 'keepstar-onboarding', name: 'Keepstar Onboarding' },
}

afterEach(() => {
  vi.restoreAllMocks()
  window.localStorage.clear()
})

describe('OnboardingShell', () => {
  it('gate → wrong password error → right password → chat (session id persisted)', async () => {
    let gated = false
    globalThis.fetch = routeFetch({
      'POST /onboard/session': () => (gated ? resp(200, SESSION) : resp(403, undefined)),
      'POST /onboard/gate': (init) => {
        const body = JSON.parse(init.body)
        expect(init.credentials).toBe('include')
        if (body.password === 'Bb8tj13k') {
          gated = true
          return resp(200, { ok: true })
        }
        return resp(403, undefined)
      },
    })

    render(<OnboardingShell apiBaseUrl={BASE} />)

    // No stored session + no cookie: create 403s → password gate.
    const input = await screen.findByLabelText('Access password')

    // Wrong password → inline error, still on the gate.
    fireEvent.change(input, { target: { value: 'nope' } })
    fireEvent.click(screen.getByRole('button', { name: 'Enter' }))
    await screen.findByText('Wrong password. Try again.')

    // Right password → gate 200 → session create → chat column.
    fireEvent.change(input, { target: { value: 'Bb8tj13k' } })
    fireEvent.click(screen.getByRole('button', { name: 'Enter' }))
    await screen.findByPlaceholderText('Describe your business…')
    expect(window.localStorage.getItem(STORE_KEY)).toBe('onb-1')
  })

  it('resumes the stored session with its transcript (refresh-safe)', async () => {
    window.localStorage.setItem(STORE_KEY, 'onb-9')
    globalThis.fetch = routeFetch({
      'GET /onboard/session?sessionId=onb-9': () =>
        resp(200, {
          sessionId: 'onb-9',
          mode: 'onboarding',
          transcript: [
            { role: 'user', text: 'I run a realtor agency' },
            { role: 'assistant', text: 'Here is the plan so far.' },
          ],
        }),
    })

    render(<OnboardingShell apiBaseUrl={BASE} />)

    await screen.findByPlaceholderText('Describe your business…')
    expect(screen.getByText('I run a realtor agency')).toBeTruthy()
    expect(screen.getByText('Here is the plan so far.')).toBeTruthy()
  })

  it('renders the disabled page on 503 (ONBOARDING_PASSWORD unset)', async () => {
    globalThis.fetch = routeFetch({
      'POST /onboard/session': () => resp(503, undefined),
    })

    render(<OnboardingShell apiBaseUrl={BASE} />)
    await screen.findByText('Onboarding is currently disabled. Please come back later.')
  })

  it('discards a stale stored session (404) and starts a fresh one', async () => {
    window.localStorage.setItem(STORE_KEY, 'onb-old')
    globalThis.fetch = routeFetch({
      'GET /onboard/session?sessionId=onb-old': () => resp(404, undefined),
      'POST /onboard/session': () => resp(200, SESSION),
    })

    render(<OnboardingShell apiBaseUrl={BASE} />)
    await screen.findByPlaceholderText('Describe your business…')
    expect(window.localStorage.getItem(STORE_KEY)).toBe('onb-1')
  })
})

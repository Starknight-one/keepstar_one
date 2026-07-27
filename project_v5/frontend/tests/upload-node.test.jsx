import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, fireEvent, waitFor } from '@testing-library/react'
import SceneGraphRenderer from '../src/renderer/SceneGraphRenderer'
import RenderContext from '../src/renderer/RenderContext'

// Upload node (RUNTIME_SPEC §5.2 upload, R25 async two-phase):
//   disarmed (beat 2) → picker off + explanatory note;
//   armed (beat 3, token bound via render-time ops) → multipart POST with
//   the token BEFORE the file → poll → completed summary / failed + retry.

beforeEach(() => {
  globalThis.fetch = vi.fn()
  vi.useFakeTimers({ shouldAdvanceTime: true })
})

afterEach(() => {
  vi.useRealTimers()
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
  return render(
    <RenderContext.Provider value={ctx}>
      <SceneGraphRenderer document={{ version: '2.10', children }} />
    </RenderContext.Provider>,
  )
}

function uploadNode(overrides = {}) {
  return {
    type: 'upload',
    id: 'up-input',
    name: 'file',
    accept: ['.csv', '.json'],
    maxSizeMb: 20,
    ...overrides,
  }
}

function jsonResponse(status, payload) {
  return { ok: status >= 200 && status < 300, status, text: async () => JSON.stringify(payload) }
}

describe('upload node — disarmed (beat 2, R25)', () => {
  it('renders the picker disabled with the seeded note; nothing fetches', () => {
    const { container } = renderDoc([
      uploadNode({ disarmed: true, note: 'The uploader activates once you confirm the plan.' }),
    ])
    const root = container.querySelector('.kw-upload')
    expect(root.getAttribute('data-armed')).toBe('false')
    expect(container.querySelector('.kw-upload-input')).toBeDisabled()
    expect(container.querySelector('.kw-upload-button')).toBeDisabled()
    expect(container.querySelector('.kw-upload-note')).toHaveTextContent('activates once you confirm')
    expect(fetch).not.toHaveBeenCalled()
  })

  it('armed-without-token renders disarmed with a loud diagnostic note', () => {
    const { container } = renderDoc([uploadNode({ disarmed: false })])
    expect(container.querySelector('.kw-upload').getAttribute('data-armed')).toBe('false')
    expect(container.querySelector('.kw-upload-note')).toHaveTextContent('not armed')
  })
})

describe('upload node — armed two-phase flow (beat 3)', () => {
  it('POSTs multipart with token before file, polls, and shows the honest summary', async () => {
    fetch
      .mockResolvedValueOnce(jsonResponse(202, { jobId: 'job-1', status: 'accepted', sessionId: 's-onb' }))
      .mockResolvedValueOnce(
        jsonResponse(200, {
          jobId: 'job-1',
          status: 'completed',
          processed: 20,
          projectionRows: 20,
          invalidated: true,
          errors: [],
        }),
      )

    const { container } = renderDoc([uploadNode({ disarmed: false, token: 'tok-123' })])
    expect(container.querySelector('.kw-upload').getAttribute('data-armed')).toBe('true')

    const file = new File(['id,name\n1,Flat'], 'realtor_20.csv', { type: 'text/csv' })
    const input = container.querySelector('.kw-upload-input')
    await fireEvent.change(input, { target: { files: [file] } })

    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1))
    const [uploadUrl, uploadInit] = fetch.mock.calls[0]
    expect(uploadUrl).toBe('http://api/v1/onboard/upload')
    expect(uploadInit.credentials).toBe('include')
    const entries = [...uploadInit.body.entries()]
    expect(entries[0][0]).toBe('token')
    expect(entries[0][1]).toBe('tok-123')
    expect(entries[1][0]).toBe('file')

    await vi.advanceTimersByTimeAsync(2000)
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2))
    const [statusUrl, statusInit] = fetch.mock.calls[1]
    expect(statusUrl).toBe('http://api/v1/onboard/upload/job-1?sessionId=s-onb')
    expect(statusInit.credentials).toBe('include')

    await waitFor(() => {
      expect(container.querySelector('.kw-upload-status[data-status="done"]')).toHaveTextContent(
        'Imported 20 items — 20 searchable listings.',
      )
    })
  })

  it('failed import relays the errors and re-enables the picker (re-upload, R25)', async () => {
    fetch
      .mockResolvedValueOnce(jsonResponse(202, { jobId: 'job-2', status: 'accepted', sessionId: 's-onb' }))
      .mockResolvedValueOnce(
        jsonResponse(200, { jobId: 'job-2', status: 'failed', errors: ['row 3: bad price'] }),
      )

    const { container } = renderDoc([uploadNode({ disarmed: false, token: 'tok-123' })])
    const file = new File(['broken'], 'bad.csv', { type: 'text/csv' })
    await fireEvent.change(container.querySelector('.kw-upload-input'), { target: { files: [file] } })

    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1))
    await vi.advanceTimersByTimeAsync(2000)
    await waitFor(() => {
      expect(container.querySelector('.kw-upload-status[data-status="failed"]')).toHaveTextContent(
        'row 3: bad price',
      )
    })
    expect(container.querySelector('.kw-upload-input')).not.toBeDisabled()
    expect(container.querySelector('.kw-upload-button')).not.toBeDisabled()
  })

  it('rejects a wrong extension client-side without any network activity', async () => {
    const { container } = renderDoc([uploadNode({ disarmed: false, token: 'tok-123' })])
    const file = new File(['x'], 'listing.xlsx')
    await fireEvent.change(container.querySelector('.kw-upload-input'), { target: { files: [file] } })
    await waitFor(() => {
      expect(container.querySelector('.kw-upload-status[data-status="failed"]')).toHaveTextContent(
        'Unsupported file type',
      )
    })
    expect(fetch).not.toHaveBeenCalled()
  })
})

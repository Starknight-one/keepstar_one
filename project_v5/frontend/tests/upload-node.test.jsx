import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, fireEvent, waitFor } from '@testing-library/react'
import SceneGraphRenderer from '../src/renderer/SceneGraphRenderer'
import RenderContext from '../src/renderer/RenderContext'

// Upload node (RUNTIME_SPEC §5.2 upload, R25 async two-phase + owner
// decision 2026-07-28): the control is ALWAYS usable — the user's upload
// IS the approval, so neither the legacy `disarmed` flag nor a missing
// token disables the picker. The request carries the sessionId (before
// the file part, streamed server-side); a bound token still rides along.
// Server rejections show their reason inline.

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

function textResponse(status, body) {
  return { ok: status >= 200 && status < 300, status, text: async () => body }
}

describe('upload node — never locked (owner decision 2026-07-28)', () => {
  it('renders usable even when the document says disarmed and has no token', () => {
    const { container } = renderDoc([
      uploadNode({ disarmed: true, note: 'Upload your catalog any time.' }),
    ])
    expect(container.querySelector('.kw-upload-input')).not.toBeDisabled()
    expect(container.querySelector('.kw-upload-button')).not.toBeDisabled()
    expect(container.querySelector('.kw-upload-note')).toHaveTextContent('Upload your catalog any time.')
    expect(fetch).not.toHaveBeenCalled()
  })

  it('uploads token-less: multipart carries sessionId before the file', async () => {
    fetch
      .mockResolvedValueOnce(jsonResponse(202, { jobId: 'job-3', status: 'accepted', sessionId: 's-1' }))
      .mockResolvedValueOnce(
        jsonResponse(200, {
          jobId: 'job-3',
          status: 'completed',
          processed: 5,
          projectionRows: 5,
          invalidated: true,
          errors: [],
        }),
      )

    const { container } = renderDoc([uploadNode({ disarmed: true })])
    const file = new File(['id,name\n1,Flat'], 'listings.csv', { type: 'text/csv' })
    await fireEvent.change(container.querySelector('.kw-upload-input'), { target: { files: [file] } })

    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1))
    const [uploadUrl, uploadInit] = fetch.mock.calls[0]
    expect(uploadUrl).toBe('http://api/v1/onboard/upload')
    expect(uploadInit.credentials).toBe('include')
    const entries = [...uploadInit.body.entries()]
    expect(entries.map(([k]) => k)).toEqual(['sessionId', 'file'])
    expect(entries[0][1]).toBe('s-1')

    await vi.advanceTimersByTimeAsync(2000)
    await waitFor(() => {
      expect(container.querySelector('.kw-upload-status[data-status="done"]')).toHaveTextContent(
        'Imported 5 items — 5 searchable listings.',
      )
    })
  })
})

describe('upload node — two-phase flow (R25)', () => {
  it('POSTs multipart with token and sessionId before the file, polls, shows the honest summary', async () => {
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

    const { container } = renderDoc([uploadNode({ token: 'tok-123' })])
    const file = new File(['id,name\n1,Flat'], 'realtor_20.csv', { type: 'text/csv' })
    const input = container.querySelector('.kw-upload-input')
    await fireEvent.change(input, { target: { files: [file] } })

    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1))
    const [uploadUrl, uploadInit] = fetch.mock.calls[0]
    expect(uploadUrl).toBe('http://api/v1/onboard/upload')
    expect(uploadInit.credentials).toBe('include')
    const entries = [...uploadInit.body.entries()]
    expect(entries.map(([k]) => k)).toEqual(['token', 'sessionId', 'file'])
    expect(entries[0][1]).toBe('tok-123')
    expect(entries[1][1]).toBe('s-1')

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

    const { container } = renderDoc([uploadNode({ token: 'tok-123' })])
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
    const { container } = renderDoc([uploadNode({ token: 'tok-123' })])
    const file = new File(['x'], 'listing.xlsx')
    await fireEvent.change(container.querySelector('.kw-upload-input'), { target: { files: [file] } })
    await waitFor(() => {
      expect(container.querySelector('.kw-upload-status[data-status="failed"]')).toHaveTextContent(
        'Unsupported file type',
      )
    })
    expect(fetch).not.toHaveBeenCalled()
  })

  it('rejects an oversized file client-side without any network activity', async () => {
    const { container } = renderDoc([uploadNode({ maxSizeMb: 1 })])
    const big = new File([new Uint8Array(2 * 1024 * 1024)], 'huge.csv', { type: 'text/csv' })
    await fireEvent.change(container.querySelector('.kw-upload-input'), { target: { files: [big] } })
    await waitFor(() => {
      expect(container.querySelector('.kw-upload-status[data-status="failed"]')).toHaveTextContent(
        'File is too large — up to 1 MB.',
      )
    })
    expect(fetch).not.toHaveBeenCalled()
  })

  it('shows the server rejection reason inline and re-enables the picker', async () => {
    fetch.mockResolvedValueOnce(textResponse(409, 'upload token already used\n'))

    const { container } = renderDoc([uploadNode({ token: 'tok-spent' })])
    const file = new File(['id,name'], 'again.csv', { type: 'text/csv' })
    await fireEvent.change(container.querySelector('.kw-upload-input'), { target: { files: [file] } })

    await waitFor(() => {
      expect(container.querySelector('.kw-upload-status[data-status="failed"]')).toHaveTextContent(
        'upload token already used',
      )
    })
    expect(container.querySelector('.kw-upload-input')).not.toBeDisabled()
    expect(container.querySelector('.kw-upload-button')).not.toBeDisabled()
  })
})

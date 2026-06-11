import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { pipelineSmartRequest, pipelineStreamRequest } from '../src/api/client'

beforeEach(() => {
  globalThis.fetch = vi.fn()
})

afterEach(() => {
  vi.restoreAllMocks()
})

const OPTS = {
  baseUrl: 'http://api/v1',
  tenantSlug: 'demo',
  sessionId: 's-1',
  query: 'sofas',
}

// Hand-rolled streaming response: body.getReader() yields each chunk in
// order, then done. Frame boundaries deliberately don't line up with
// chunk boundaries in the tests below.
function streamResponse(chunks) {
  const encoder = new TextEncoder()
  let i = 0
  return {
    ok: true,
    status: 200,
    body: {
      getReader: () => ({
        read: async () =>
          i < chunks.length
            ? { done: false, value: encoder.encode(chunks[i++]) }
            : { done: true, value: undefined },
      }),
    },
  }
}

const DATA_DONE = {
  phase: 'data_done',
  signal: 'new_search: 12 items found',
  toolName: 'catalog_search',
  count: 12,
  empty: false,
  bypassed: false,
  agent1Ms: 2031,
}

const RESULT = {
  document: { version: '2.10', children: [] },
  latencyMs: 4200,
  spans: [],
}

describe('pipelineStreamRequest', () => {
  it('parses frames split across chunk boundaries, emits stages, resolves with result', async () => {
    const wire =
      'event: stage\ndata: {"phase":"data_start"}\n\n' +
      `event: stage\ndata: ${JSON.stringify(DATA_DONE)}\n\n` +
      `event: result\ndata: ${JSON.stringify(RESULT)}\n\n`
    // Split mid-line and mid-JSON so the buffer has to reassemble.
    const chunks = [wire.slice(0, 17), wire.slice(17, 60), wire.slice(60, 61), wire.slice(61)]
    fetch.mockResolvedValue(streamResponse(chunks))

    const onStage = vi.fn()
    const resp = await pipelineStreamRequest({ ...OPTS, onStage })

    expect(fetch).toHaveBeenCalledTimes(1)
    const [url, init] = fetch.mock.calls[0]
    expect(url).toBe('http://api/v1/pipeline/stream')
    expect(init.headers['Accept']).toBe('text/event-stream')
    expect(JSON.parse(init.body)).toEqual({ sessionId: 's-1', query: 'sofas' })

    expect(onStage).toHaveBeenCalledTimes(2)
    expect(onStage.mock.calls[0][0]).toEqual({ phase: 'data_start' })
    expect(onStage.mock.calls[1][0]).toEqual(DATA_DONE)
    expect(resp).toEqual(RESULT)
  })

  it('error event → throws with status and message', async () => {
    fetch.mockResolvedValue(
      streamResponse([
        'event: stage\ndata: {"phase":"data_start"}\n\n' +
          'event: error\ndata: {"status":409,"message":"session closed"}\n\n',
      ]),
    )
    await expect(pipelineStreamRequest(OPTS)).rejects.toThrow('pipeline 409: session closed')
  })

  it('stream ending without a result frame → throws', async () => {
    fetch.mockResolvedValue(streamResponse(['event: stage\ndata: {"phase":"data_start"}\n\n']))
    await expect(pipelineStreamRequest(OPTS)).rejects.toThrow('pipeline stream ended early')
  })
})

describe('pipelineSmartRequest', () => {
  it('falls back to POST /pipeline when the route is missing (older backend)', async () => {
    fetch
      .mockResolvedValueOnce({ ok: false, status: 404, text: async () => 'not found' })
      .mockResolvedValueOnce({ ok: true, status: 200, json: async () => RESULT })

    const resp = await pipelineSmartRequest(OPTS)

    expect(fetch).toHaveBeenCalledTimes(2)
    expect(fetch.mock.calls[0][0]).toBe('http://api/v1/pipeline/stream')
    expect(fetch.mock.calls[1][0]).toBe('http://api/v1/pipeline')
    expect(resp).toEqual(RESULT)
  })

  it('falls back when the connection fails before any frame', async () => {
    fetch
      .mockRejectedValueOnce(new TypeError('network down'))
      .mockResolvedValueOnce({ ok: true, status: 200, json: async () => RESULT })

    const resp = await pipelineSmartRequest(OPTS)

    expect(fetch).toHaveBeenCalledTimes(2)
    expect(resp).toEqual(RESULT)
  })

  it('does NOT fall back once frames have arrived — the turn may already have run', async () => {
    fetch.mockResolvedValueOnce(
      streamResponse(['event: stage\ndata: {"phase":"data_start"}\n\n']),
    )
    await expect(pipelineSmartRequest(OPTS)).rejects.toThrow('pipeline stream ended early')
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it('does NOT fall back on a server error frame — the backend definitively answered', async () => {
    fetch.mockResolvedValueOnce(
      streamResponse([
        'event: stage\ndata: {"phase":"data_start"}\n\n',
        'event: error\ndata: {"status":409,"message":"session closed"}\n\n',
      ]),
    )
    await expect(pipelineSmartRequest(OPTS)).rejects.toThrow('pipeline 409: session closed')
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it('rethrows when the fallback also fails', async () => {
    fetch
      .mockResolvedValueOnce(streamResponse([]))
      .mockResolvedValueOnce({
        ok: false,
        status: 500,
        text: async () => 'pipeline failed',
      })
    await expect(pipelineSmartRequest(OPTS)).rejects.toThrow('pipeline 500: pipeline failed')
    expect(fetch).toHaveBeenCalledTimes(2)
  })
})

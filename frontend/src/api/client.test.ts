import { ApiError, request } from './client'

function mockFetch(res: Response) {
  const fn = vi.fn().mockResolvedValue(res)
  vi.stubGlobal('fetch', fn)
  return fn
}

afterEach(() => vi.unstubAllGlobals())

describe('request', () => {
  it('GETs under /api/v1 with credentials and parses JSON', async () => {
    const fn = mockFetch(Response.json({ ok: 1 }))
    await expect(request('/me')).resolves.toEqual({ ok: 1 })
    expect(fn).toHaveBeenCalledWith('/api/v1/me', expect.objectContaining({ credentials: 'include', method: 'GET' }))
  })

  it('sends a JSON body with a content type', async () => {
    const fn = mockFetch(Response.json({}, { status: 201 }))
    await request('/watchlists', { method: 'POST', body: { title: 'x' } })
    const init = fn.mock.calls[0][1]
    expect(init.body).toBe('{"title":"x"}')
    expect(init.headers['Content-Type']).toBe('application/json')
  })

  it('sends no content type without a body', async () => {
    const fn = mockFetch(Response.json({}))
    await request('/me')
    expect(fn.mock.calls[0][1].headers).toEqual({})
  })

  it('returns undefined for 204', async () => {
    mockFetch(new Response(null, { status: 204 }))
    await expect(request('/x', { method: 'DELETE' })).resolves.toBeUndefined()
  })

  it('throws ApiError from the error envelope', async () => {
    mockFetch(Response.json({ error: { code: 'not_found', message: 'nope' } }, { status: 404 }))
    const err = await request('/x').catch((e) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ status: 404, code: 'not_found', message: 'nope' })
  })

  it('falls back when the error body is not an envelope', async () => {
    mockFetch(new Response('<html>', { status: 502 }))
    const err = await request('/x').catch((e) => e)
    expect(err).toMatchObject({ status: 502, code: 'unknown', message: 'Request failed (502)' })
  })
})

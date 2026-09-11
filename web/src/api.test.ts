import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError, api } from './api'

function mockFetch(status: number, body: unknown, headers: Record<string, string> = { 'content-type': 'application/json' }) {
  const fn = vi.fn(async () => new Response(body === null ? null : JSON.stringify(body), { status, headers }))
  vi.stubGlobal('fetch', fn)
  return fn
}

describe('api client', () => {
  beforeEach(() => vi.restoreAllMocks())
  afterEach(() => vi.unstubAllGlobals())

  it('GETs JSON with credentials and same-origin headers', async () => {
    const fetchMock = mockFetch(200, [{ id: 1, name: 'johnnybt-demo' }])
    const pkgs = await api.packages.list()
    expect(pkgs[0].name).toBe('johnnybt-demo')
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toBe('/api/v1/packages')
    expect(init.credentials).toBe('same-origin')
    expect(init.method ?? 'GET').toBe('GET')
  })

  it('POSTs JSON bodies', async () => {
    const fetchMock = mockFetch(200, { id: 1, username: 'admin', is_admin: true })
    await api.auth.login('admin', 'pw')
    const [, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(init.method).toBe('POST')
    expect((init.headers as Record<string, string>)['Content-Type']).toBe('application/json')
    expect(JSON.parse(init.body as string)).toEqual({ username: 'admin', password: 'pw' })
  })

  it('turns error envelopes into ApiError with status and message', async () => {
    mockFetch(401, { error: 'invalid username or password' })
    await expect(api.auth.login('admin', 'nope')).rejects.toMatchObject({
      name: 'ApiError',
      status: 401,
      message: 'invalid username or password',
    })
  })

  it('falls back to a status message when the body is not JSON', async () => {
    mockFetch(502, null, { 'content-type': 'text/html' })
    const err = await api.packages.list().catch((e) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err.status).toBe(502)
    expect(err.message).toMatch(/502/)
  })

  it('handles 204 No Content', async () => {
    mockFetch(204, null, {})
    await expect(api.auth.logout()).resolves.toBeUndefined()
  })

  it('sends multipart uploads without a JSON content type', async () => {
    const fetchMock = mockFetch(200, [{ filename: 'x.whl', package: 'x', version: '1' }])
    const file = new File(['PK'], 'x-1-py3-none-any.whl')
    const res = await api.packages.upload([file])
    expect(res[0].package).toBe('x')
    const [, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(init.body).toBeInstanceOf(FormData)
    expect((init.headers as Record<string, string>)?.['Content-Type']).toBeUndefined()
    expect((init.body as FormData).getAll('files')).toHaveLength(1)
  })

  it('encodes path parameters', async () => {
    const fetchMock = mockFetch(204, null, {})
    await api.users.grant(7, 'JohnnyBT Demo', '2026-12-31')
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toBe('/api/v1/users/7/entitlements/JohnnyBT%20Demo')
    expect(JSON.parse(init.body as string)).toEqual({ expires_at: '2026-12-31' })
  })
})

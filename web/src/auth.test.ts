import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('./api', () => ({
  api: { auth: { me: vi.fn(), login: vi.fn(), logout: vi.fn() } },
  ApiError: class ApiError extends Error {
    constructor(public status: number, message: string) { super(message) }
  },
}))

import { api } from './api'
import { useAuth } from './auth'

const mocked = api.auth as unknown as { me: ReturnType<typeof vi.fn>; login: ReturnType<typeof vi.fn>; logout: ReturnType<typeof vi.fn> }

describe('auth state', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useAuth().reset()
  })

  it('starts unknown, then resolves the session once', async () => {
    mocked.me.mockResolvedValue({ id: 1, username: 'admin', is_admin: true })
    const auth = useAuth()
    expect(auth.user.value).toBeNull()
    expect(auth.ready.value).toBe(false)
    await auth.ensure()
    await auth.ensure()
    expect(mocked.me).toHaveBeenCalledTimes(1)
    expect(auth.user.value?.username).toBe('admin')
    expect(auth.ready.value).toBe(true)
  })

  it('treats a 401 as logged out rather than an error', async () => {
    mocked.me.mockRejectedValue(Object.assign(new Error('not logged in'), { status: 401 }))
    const auth = useAuth()
    await auth.ensure()
    expect(auth.user.value).toBeNull()
    expect(auth.ready.value).toBe(true)
  })

  it('login stores the user, logout clears it', async () => {
    mocked.login.mockResolvedValue({ id: 1, username: 'admin', is_admin: true })
    mocked.logout.mockResolvedValue(undefined)
    const auth = useAuth()
    await auth.login('admin', 'pw')
    expect(auth.user.value?.username).toBe('admin')
    await auth.logout()
    expect(auth.user.value).toBeNull()
  })
})

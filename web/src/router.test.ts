import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('./api', () => ({
  api: { auth: { me: vi.fn(), login: vi.fn(), logout: vi.fn() } },
  ApiError: class ApiError extends Error {},
}))

import { api } from './api'
import { useAuth } from './auth'
import { setRuntimeConfig } from './config'
import { createAppRouter } from './router'

const me = (api.auth as unknown as { me: ReturnType<typeof vi.fn> }).me
const loggedOut = () => me.mockRejectedValue(Object.assign(new Error(), { status: 401 }))
const loggedIn = () => me.mockResolvedValue({ id: 1, username: 'admin', is_admin: true })

describe('router', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useAuth().reset()
  })
  afterEach(() => setRuntimeConfig(undefined))

  it('serves the customer self-check page at the root without touching the session', async () => {
    loggedOut()
    const router = createAppRouter(true)
    await router.push('/')
    expect(router.currentRoute.value.name).toBe('me')
    await router.push('/me')
    expect(router.currentRoute.value.path).toBe('/')
    expect(me).not.toHaveBeenCalled()
  })

  it('sends anonymous admins to the login page under the admin prefix, remembering the target', async () => {
    loggedOut()
    const router = createAppRouter(true)
    await router.push('/admin/users/3')
    expect(router.currentRoute.value.path).toBe('/admin/login')
    expect(router.currentRoute.value.query.next).toBe('/admin/users/3')
    await router.push('/admin')
    expect(router.currentRoute.value.path).toBe('/admin/login')
  })

  it('lets a logged-in admin through and bounces them off the login page', async () => {
    loggedIn()
    const router = createAppRouter(true)
    await router.push('/admin')
    expect(router.currentRoute.value.path).toBe('/admin/packages')
    await router.push('/admin/login')
    expect(router.currentRoute.value.path).toBe('/admin/packages')
    await router.push('/admin/users/3')
    expect(router.currentRoute.value.path).toBe('/admin/users/3')
  })

  it('mounts the admin UI under the prefix the server injected', async () => {
    setRuntimeConfig({ adminPath: '/manage-7' })
    loggedIn()
    const router = createAppRouter(true)
    await router.push('/manage-7')
    expect(router.currentRoute.value.path).toBe('/manage-7/packages')
    // The old default prefix is now just an unknown path.
    await router.push('/admin/packages')
    expect(router.currentRoute.value.path).toBe('/')
  })

  it('redirects unknown paths to the root', async () => {
    loggedOut()
    const router = createAppRouter(true)
    await router.push('/nope/whatever')
    expect(router.currentRoute.value.path).toBe('/')
    expect(me).not.toHaveBeenCalled()
  })
})

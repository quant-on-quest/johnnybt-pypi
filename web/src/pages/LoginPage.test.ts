import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { apiMock, notifyMock } from '../test/mocks'

vi.mock('../api', async () => {
  const { apiMock } = await import('../test/mocks')
  return { api: apiMock(), ApiError: class extends Error {} }
})
vi.mock('../notify', async () => {
  const { notifyMock, errorMessage } = await import('../test/mocks')
  return { useNotify: () => notifyMock, errorMessage }
})

import { api } from '../api'
const mocks = { api: api as unknown as ReturnType<typeof apiMock> }

import { createAppRouter } from '../router'
import { useAuth } from '../auth'
import LoginPage from './LoginPage.vue'

describe('LoginPage', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    useAuth().reset()
  })

  it('logs in and goes to the requested page', async () => {
    mocks.api.auth.me.mockRejectedValue(Object.assign(new Error(), { status: 401 }))
    mocks.api.auth.login.mockResolvedValue({ id: 1, username: 'admin', is_admin: true })
    const router = createAppRouter(true)
    await router.push('/admin/login?next=/admin/users/3')
    const w = mount(LoginPage, { global: { plugins: [router] } })

    w.findComponent({ name: 'Form' }).vm.$emit('submit', { data: { username: 'admin', password: 'pw' } })
    await flushPromises()

    expect(mocks.api.auth.login).toHaveBeenCalledWith('admin', 'pw')
    // The target page is lazy-loaded, so wait for the navigation to settle.
    await vi.waitFor(() => expect(router.currentRoute.value.path).toBe('/admin/users/3'))
  })

  it('shows the server error and stays on the page', async () => {
    mocks.api.auth.me.mockRejectedValue(Object.assign(new Error(), { status: 401 }))
    mocks.api.auth.login.mockRejectedValue(new Error('invalid username or password'))
    const router = createAppRouter(true)
    await router.push('/admin/login')
    const w = mount(LoginPage, { global: { plugins: [router] } })

    w.findComponent({ name: 'Form' }).vm.$emit('submit', { data: { username: 'admin', password: 'bad' } })
    await flushPromises()

    expect(w.text()).toContain('invalid username or password')
    expect(router.currentRoute.value.path).toBe('/admin/login')
  })
})

import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'
import type { apiMock } from '../test/mocks'

vi.mock('../api', async () => {
  const { apiMock } = await import('../test/mocks')
  return { api: apiMock(), ApiError: class extends Error {} }
})
vi.mock('../notify', async () => {
  const { notifyMock, errorMessage } = await import('../test/mocks')
  return { useNotify: () => notifyMock, errorMessage }
})

import { api } from '../api'
import { createAppRouter } from '../router'
import UsersPage from './UsersPage.vue'
const mocks = { api: api as unknown as ReturnType<typeof apiMock> }

const users = [
  { id: 1, username: 'admin', is_admin: true, note: '', created_at: '2026-09-01T00:00:00Z', entitlement_count: 0, token_count: 1 },
  { id: 2, username: 'alice', is_admin: false, note: '微信 alice', created_at: '2026-09-11T00:00:00Z', entitlement_count: 2, token_count: 1 },
]

describe('UsersPage', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    mocks.api.users.list.mockResolvedValue(users)
  })

  it('lists users with counts and links to detail', async () => {
    const w = mount(UsersPage, { global: { stubs: { RouterLink: RouterLinkStub } } })
    await flushPromises()
    const text = w.text()
    expect(text).toContain('admin')
    expect(text).toContain('alice')
    expect(text).toContain('微信 alice')
    expect(w.findAllComponents(RouterLinkStub).map((l) => l.props('to'))).toEqual(['/admin/users/1', '/admin/users/2'])
  })

  it('creates a user and navigates to it', async () => {
    mocks.api.users.create.mockResolvedValue({ id: 3, username: 'bob', is_admin: false, note: '', created_at: '' })
    const router = createAppRouter(true)
    const push = vi.spyOn(router, 'push').mockResolvedValue(undefined)
    const w = mount(UsersPage, { global: { stubs: { RouterLink: RouterLinkStub }, plugins: [router] } })
    await flushPromises()
    w.findComponent({ name: 'Form' }).vm.$emit('submit', { data: { username: 'bob', note: 'new' } })
    await flushPromises()
    expect(mocks.api.users.create).toHaveBeenCalledWith('bob', 'new')
    expect(push).toHaveBeenCalledWith('/admin/users/3')
  })

  it('filters by search text', async () => {
    const w = mount(UsersPage, { global: { stubs: { RouterLink: RouterLinkStub } } })
    await flushPromises()
    w.findComponent({ name: 'Input' }).vm.$emit('update:modelValue', 'ali')
    await flushPromises()
    expect(w.text()).toContain('alice')
    expect(w.findAllComponents(RouterLinkStub).map((l) => l.props('to'))).toEqual(['/admin/users/2'])
  })
})

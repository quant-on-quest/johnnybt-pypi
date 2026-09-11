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

import MePage from './MePage.vue'

describe('MePage (customer self-check)', () => {
  beforeEach(() => {
    vi.resetAllMocks()
  })

  it('shows packages and install snippets for a valid token', async () => {
    const plain = 'jbt_' + 'y'.repeat(40)
    mocks.api.me.mockResolvedValue({
      username: 'alice',
      note: '',
      is_admin: false,
      token: { id: 1, name: 'laptop', prefix: plain.slice(0, 12), scope: 'read' },
      packages: [{ package_name: 'johnnybt-demo', normalized_name: 'johnnybt-demo', latest_version: '0.2.0', expires_at: '2099-01-01T00:00:00Z', active: true }],
      index_url: 'https://pypi.example.com/simple/',
    })
    const w = mount(MePage)
    w.findComponent({ name: 'Form' }).vm.$emit('submit', { data: { token: plain } })
    await flushPromises()
    expect(mocks.api.me).toHaveBeenCalledWith(plain)
    const text = w.text()
    expect(text).toContain('alice')
    expect(text).toContain('johnnybt-demo')
    expect(text).toContain('0.2.0')
    expect(text).toContain('2099-01-01')
    expect(text).toContain('uv auth login')
    expect(text).toContain(plain)
  })

  it('reports an invalid token', async () => {
    mocks.api.me.mockRejectedValue(new Error('invalid or revoked token'))
    const w = mount(MePage)
    w.findComponent({ name: 'Form' }).vm.$emit('submit', { data: { token: 'nope' } })
    await flushPromises()
    expect(w.text()).toContain('invalid or revoked token')
  })
})

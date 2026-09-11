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
import DownloadsPage from './DownloadsPage.vue'
const mocks = { api: api as unknown as ReturnType<typeof apiMock> }

describe('DownloadsPage', () => {
  beforeEach(() => vi.resetAllMocks())

  it('lists recent downloads', async () => {
    mocks.api.downloads.list.mockResolvedValue([
      { id: 1, filename: 'johnnybt_demo-0.1.0-py3-none-any.whl', ip: '1.2.3.4', user_agent: 'uv/0.9.3 {...}', at: '2026-09-11T10:00:00Z', username: 'alice', package_name: 'johnnybt-demo' },
    ])
    const w = mount(DownloadsPage, { global: { stubs: { RouterLink: RouterLinkStub } } })
    await flushPromises()
    const text = w.text()
    expect(text).toContain('alice')
    expect(text).toContain('johnnybt_demo-0.1.0-py3-none-any.whl')
    expect(text).toContain('1.2.3.4')
    expect(text).toContain('uv/0.9.3')
  })

  it('shows an empty state', async () => {
    mocks.api.downloads.list.mockResolvedValue([])
    const w = mount(DownloadsPage, { global: { stubs: { RouterLink: RouterLinkStub } } })
    await flushPromises()
    expect(w.text()).toContain('还没有下载记录')
  })
})

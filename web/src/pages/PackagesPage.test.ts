import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'
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

import PackagesPage from './PackagesPage.vue'

describe('PackagesPage', () => {
  beforeEach(() => {
    vi.resetAllMocks()
  })

  it('lists packages with version and counts', async () => {
    mocks.api.packages.list.mockResolvedValue([
      { id: 1, name: 'johnnybt-demo', normalized_name: 'johnnybt-demo', summary: 'Demo package', latest_version: '0.2.0', release_count: 2, file_count: 3, user_count: 4, download_count: 5 },
    ])
    const w = mount(PackagesPage, { global: { stubs: { RouterLink: RouterLinkStub } } })
    await flushPromises()
    const text = w.text()
    expect(text).toContain('johnnybt-demo')
    expect(text).toContain('Demo package')
    expect(text).toContain('0.2.0')
    expect(w.findComponent(RouterLinkStub).props('to')).toBe('/admin/packages/johnnybt-demo')
  })

  it('shows an empty state', async () => {
    mocks.api.packages.list.mockResolvedValue([])
    const w = mount(PackagesPage, { global: { stubs: { RouterLink: RouterLinkStub } } })
    await flushPromises()
    expect(w.text()).toContain('还没有包')
  })
})

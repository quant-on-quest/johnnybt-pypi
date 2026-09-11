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
import PackageDetailPage from './PackageDetailPage.vue'
const mocks = { api: api as unknown as ReturnType<typeof apiMock> }

const detail = {
  package: { id: 1, name: 'johnnybt-demo', normalized_name: 'johnnybt-demo', summary: 'Demo', description: '# Demo\n\nHello **world**', description_content_type: 'text/markdown', created_at: '', updated_at: '' },
  latest_version: '0.2.0',
  releases: [
    { id: 2, package_id: 1, version: '0.2.0', yanked: false, yanked_reason: '', created_at: '2026-09-11T00:00:00Z', files: [
      { id: 5, release_id: 2, filename: 'johnnybt_demo-0.2.0-py3-none-any.whl', sha256: 'abc', size: 1018, requires_python: '>=3.10', metadata_sha256: 'def', uploaded_at: '' },
    ] },
    { id: 1, package_id: 1, version: '0.1.0', yanked: true, yanked_reason: 'broken', created_at: '2026-09-01T00:00:00Z', files: [] },
  ],
  users: [{ user_id: 2, package_id: 1, username: 'alice', note: '', expires_at: null, granted_at: '', active: true }],
}

function mountPage() {
  const router = createAppRouter(true)
  const push = vi.spyOn(router, 'push').mockResolvedValue(undefined)
  const w = mount(PackageDetailPage, { props: { name: 'johnnybt-demo' }, global: { stubs: { RouterLink: RouterLinkStub }, plugins: [router] } })
  return { w, push }
}

describe('PackageDetailPage', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    mocks.api.packages.get.mockResolvedValue(detail)
    mocks.api.config.mockResolvedValue({ base_url: 'https://pypi.example.com', index_url: 'https://pypi.example.com/simple/' })
  })

  it('shows releases, files, yank state and entitled users', async () => {
    const { w } = mountPage()
    await flushPromises()
    const text = w.text()
    expect(text).toContain('johnnybt-demo')
    expect(text).toContain('0.2.0')
    expect(text).toContain('johnnybt_demo-0.2.0-py3-none-any.whl')
    expect(text).toContain('1018 B')
    expect(text).toContain('>=3.10')
    expect(text).toContain('broken') // yank reason
    expect(text).toContain('alice')
    expect(text).toContain('uv add johnnybt-demo')
  })

  it('renders the README as plain text (no HTML injection)', async () => {
    mocks.api.packages.get.mockResolvedValue({ ...detail, package: { ...detail.package, description: '<img src=x onerror=alert(1)>' } })
    const { w } = mountPage()
    await flushPromises()
    expect(w.find('img').exists()).toBe(false)
    expect(w.text()).toContain('<img src=x onerror=alert(1)>')
  })

  it('yanks and un-yanks a release', async () => {
    vi.stubGlobal('prompt', vi.fn(() => 'security issue'))
    mocks.api.packages.yank.mockResolvedValue(undefined)
    const { w } = mountPage()
    await flushPromises()
    await w.find('[data-test="yank-0.2.0"]').trigger('click')
    await flushPromises()
    expect(mocks.api.packages.yank).toHaveBeenCalledWith('johnnybt-demo', '0.2.0', true, 'security issue')
    await w.find('[data-test="unyank-0.1.0"]').trigger('click')
    await flushPromises()
    expect(mocks.api.packages.yank).toHaveBeenCalledWith('johnnybt-demo', '0.1.0', false, '')
    expect(mocks.api.packages.get).toHaveBeenCalledTimes(3)
    vi.unstubAllGlobals()
  })

  it('deletes the package after confirmation and goes back to the list', async () => {
    vi.stubGlobal('confirm', vi.fn(() => true))
    mocks.api.packages.remove.mockResolvedValue(undefined)
    const { w, push } = mountPage()
    await flushPromises()
    await w.find('[data-test="delete-package"]').trigger('click')
    await flushPromises()
    expect(mocks.api.packages.remove).toHaveBeenCalledWith('johnnybt-demo')
    expect(push).toHaveBeenCalledWith('/admin/packages')
    vi.unstubAllGlobals()
  })
})

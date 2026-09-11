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

import UserDetailPage from './UserDetailPage.vue'

const alice = { id: 2, username: 'alice', is_admin: false, note: '微信 alice', created_at: '2026-09-11T00:00:00Z' }
const detail = {
  user: alice,
  entitlements: [
    { user_id: 2, package_id: 1, package_name: 'johnnybt-demo', normalized_name: 'johnnybt-demo', latest_version: '0.2.0', expires_at: null, granted_at: '2026-09-11T00:00:00Z', active: true },
  ],
  tokens: [
    { id: 9, user_id: 2, name: 'laptop', prefix: 'jbt_abcdefgh', scope: 'read', created_at: '2026-09-11T00:00:00Z', last_used_at: null, revoked_at: null },
  ],
}

function mountPage() {
  return mount(UserDetailPage, { props: { id: 2 }, global: { stubs: { RouterLink: RouterLinkStub } } })
}

describe('UserDetailPage', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    mocks.api.users.get.mockResolvedValue(detail)
    mocks.api.packages.list.mockResolvedValue([
      { id: 1, name: 'johnnybt-demo', normalized_name: 'johnnybt-demo' },
      { id: 2, name: 'other-pkg', normalized_name: 'other-pkg' },
    ])
    mocks.api.config.mockResolvedValue({ base_url: 'https://pypi.example.com', index_url: 'https://pypi.example.com/simple/' })
  })

  it('renders the user, their entitlements and tokens', async () => {
    const w = mountPage()
    await flushPromises()
    const text = w.text()
    expect(text).toContain('alice')
    expect(text).toContain('微信 alice')
    expect(text).toContain('johnnybt-demo')
    expect(text).toContain('永久')
    expect(text).toContain('jbt_abcdefgh')
    expect(text).toContain('laptop')
  })

  it('grants a package with an expiry and reloads', async () => {
    mocks.api.users.grant.mockResolvedValue(undefined)
    const w = mountPage()
    await flushPromises()
    const forms = w.findAllComponents({ name: 'Form' })
    const grantForm = forms.find((f) => f.attributes('data-test') === 'grant-form')!
    grantForm.vm.$emit('submit', { data: { package: 'other-pkg', expires_at: '2026-12-31' } })
    await flushPromises()
    expect(mocks.api.users.grant).toHaveBeenCalledWith(2, 'other-pkg', '2026-12-31')
    expect(mocks.api.users.get).toHaveBeenCalledTimes(2)
    expect(notifyMock.success).toHaveBeenCalled()
  })

  it('creates a token and reveals it exactly once with install snippets', async () => {
    const plain = 'jbt_' + 'x'.repeat(40)
    mocks.api.users.createToken.mockResolvedValue({
      token: plain,
      info: { id: 10, user_id: 2, name: 'desktop', prefix: plain.slice(0, 12), scope: 'read', created_at: '', last_used_at: null, revoked_at: null },
      index_url: 'https://pypi.example.com/simple/',
    })
    const w = mountPage()
    await flushPromises()
    expect(w.text()).not.toContain(plain)
    const tokenForm = w.findAllComponents({ name: 'Form' }).find((f) => f.attributes('data-test') === 'token-form')!
    tokenForm.vm.$emit('submit', { data: { name: 'desktop', scope: 'read' } })
    await flushPromises()
    expect(mocks.api.users.createToken).toHaveBeenCalledWith(2, 'desktop', 'read')
    expect(w.text()).toContain(plain)
    expect(w.text()).toContain('uv auth login')
    expect(w.text()).toContain('johnnybt-demo') // snippets default to the user's first package
  })

  it('offers write scope only for admins', async () => {
    const w = mountPage()
    await flushPromises()
    expect(w.text()).not.toContain('write')
    mocks.api.users.get.mockResolvedValue({ ...detail, user: { ...alice, is_admin: true } })
    const w2 = mountPage()
    await flushPromises()
    expect(w2.text()).toContain('write')
  })

  it('revokes a token after confirmation', async () => {
    vi.stubGlobal('confirm', vi.fn(() => true))
    mocks.api.tokens.revoke.mockResolvedValue(undefined)
    const w = mountPage()
    await flushPromises()
    await w.find('[data-test="revoke-token-9"]').trigger('click')
    await flushPromises()
    expect(mocks.api.tokens.revoke).toHaveBeenCalledWith(9)
    vi.unstubAllGlobals()
  })
})

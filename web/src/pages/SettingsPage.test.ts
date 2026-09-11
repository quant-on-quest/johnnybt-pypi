import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
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
import { notifyMock } from '../test/mocks'
import SettingsPage from './SettingsPage.vue'
const mocks = { api: api as unknown as ReturnType<typeof apiMock> }

describe('SettingsPage', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    mocks.api.config.mockResolvedValue({ base_url: 'https://pypi.example.com', index_url: 'https://pypi.example.com/simple/', upload_url: 'https://pypi.example.com/legacy/' })
  })

  it('shows server URLs', async () => {
    const w = mount(SettingsPage)
    await flushPromises()
    expect(w.text()).toContain('https://pypi.example.com/simple/')
  })

  it('changes the password when the confirmation matches', async () => {
    mocks.api.auth.changePassword.mockResolvedValue(undefined)
    const w = mount(SettingsPage)
    await flushPromises()
    w.findComponent({ name: 'Form' }).vm.$emit('submit', { data: { current: 'old', next: 'new-password-1', confirm: 'new-password-1' } })
    await flushPromises()
    expect(mocks.api.auth.changePassword).toHaveBeenCalledWith('old', 'new-password-1')
    expect(notifyMock.success).toHaveBeenCalled()
  })

  it('refuses a mismatched confirmation without calling the API', async () => {
    const w = mount(SettingsPage)
    await flushPromises()
    w.findComponent({ name: 'Form' }).vm.$emit('submit', { data: { current: 'old', next: 'new-password-1', confirm: 'different' } })
    await flushPromises()
    expect(mocks.api.auth.changePassword).not.toHaveBeenCalled()
    expect(w.text()).toContain('两次输入的新密码不一致')
  })
})

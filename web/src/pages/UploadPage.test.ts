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

import UploadPage from './UploadPage.vue'

describe('UploadPage', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    mocks.api.config.mockResolvedValue({ base_url: 'https://pypi.example.com', index_url: 'https://pypi.example.com/simple/', upload_url: 'https://pypi.example.com/legacy/' })
  })

  it('uploads selected files and lists per-file results', async () => {
    mocks.api.packages.upload.mockResolvedValue([
      { filename: 'johnnybt_demo-0.1.0-py3-none-any.whl', package: 'johnnybt-demo', version: '0.1.0' },
      { filename: 'junk.txt', error: 'invalid distribution file' },
    ])
    const w = mount(UploadPage, { global: { stubs: { RouterLink: RouterLinkStub } } })
    await flushPromises()
    const files = [new File(['PK'], 'johnnybt_demo-0.1.0-py3-none-any.whl'), new File(['x'], 'junk.txt')]
    w.findComponent({ name: 'FileUpload' }).vm.$emit('update:modelValue', files)
    await w.find('[data-test="upload-button"]').trigger('click')
    await flushPromises()
    expect(mocks.api.packages.upload).toHaveBeenCalledWith(files)
    const text = w.text()
    expect(text).toContain('johnnybt-demo 0.1.0')
    expect(text).toContain('junk.txt')
    expect(text).toContain('invalid distribution file')
  })

  it('explains how to publish from the command line', async () => {
    const w = mount(UploadPage, { global: { stubs: { RouterLink: RouterLinkStub } } })
    await flushPromises()
    expect(w.text()).toContain('uv publish --publish-url https://pypi.example.com/legacy/')
  })
})

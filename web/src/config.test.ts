import { afterEach, describe, expect, it } from 'vitest'
import { admin, adminPath, setRuntimeConfig } from './config'

describe('runtime config', () => {
  afterEach(() => setRuntimeConfig(undefined))

  it('defaults the admin prefix to /admin', () => {
    expect(adminPath()).toBe('/admin')
    expect(admin('/users/3')).toBe('/admin/users/3')
    expect(admin('')).toBe('/admin')
  })

  it('reads the prefix injected by the Go server and normalises it', () => {
    setRuntimeConfig({ adminPath: '/manage-7/' })
    expect(adminPath()).toBe('/manage-7')
    expect(admin('/packages')).toBe('/manage-7/packages')
    setRuntimeConfig({ adminPath: 'nested/x' })
    expect(adminPath()).toBe('/nested/x')
  })

  it('ignores garbage and falls back to the default', () => {
    setRuntimeConfig({ adminPath: '' })
    expect(adminPath()).toBe('/admin')
    setRuntimeConfig({ adminPath: '/' })
    expect(adminPath()).toBe('/admin')
  })
})

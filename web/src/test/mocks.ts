import { vi } from 'vitest'

/** A fully mocked `api` module; every leaf is a vi.fn(). */
export function apiMock() {
  const fn = () => vi.fn()
  return {
    auth: { login: fn(), logout: fn(), me: fn(), changePassword: fn() },
    config: fn(),
    packages: { list: fn(), get: fn(), remove: fn(), upload: fn(), yank: fn(), removeRelease: fn() },
    users: { list: fn(), create: fn(), get: fn(), update: fn(), remove: fn(), grant: fn(), revoke: fn(), createToken: fn() },
    tokens: { revoke: fn() },
    downloads: { list: fn() },
    me: fn(),
  }
}

export const notifyMock = { success: vi.fn(), error: vi.fn() }

/** Pass-through for the real errorMessage helper when notify is mocked. */
export const errorMessage = (err: unknown) => (err instanceof Error ? err.message : String(err))

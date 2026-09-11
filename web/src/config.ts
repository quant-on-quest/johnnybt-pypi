// Runtime configuration injected by the Go server into index.html as
// window.__PYPI__. In `vite dev` nothing is injected and defaults apply.

export interface RuntimeConfig {
  adminPath?: string
}

declare global {
  interface Window {
    __PYPI__?: RuntimeConfig
  }
}

const DEFAULT_ADMIN_PATH = '/admin'

/** Test hook: replace the injected config. */
export function setRuntimeConfig(cfg: RuntimeConfig | undefined) {
  window.__PYPI__ = cfg
}

/** URL prefix of the admin UI, always "/x/y" with no trailing slash. */
export function adminPath(): string {
  const raw = window.__PYPI__?.adminPath?.trim() ?? ''
  const trimmed = raw.replace(/^\/+|\/+$/g, '')
  return trimmed ? `/${trimmed}` : DEFAULT_ADMIN_PATH
}

/** Prefix an admin-relative path: admin('/users/3') → '/admin/users/3'. */
export function admin(path: string): string {
  if (!path || path === '/') return adminPath()
  return adminPath() + (path.startsWith('/') ? path : `/${path}`)
}

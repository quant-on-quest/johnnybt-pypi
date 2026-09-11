// Typed client for the Go JSON API. Every call is same-origin; the admin
// session rides on an HttpOnly cookie set by /auth/login.

export class ApiError extends Error {
  override name = 'ApiError'
  constructor(
    public status: number,
    message: string,
  ) {
    super(message)
  }
}

export interface User {
  id: number
  username: string
  is_admin: boolean
  note: string
  created_at: string
}

export interface UserSummary extends User {
  entitlement_count: number
  token_count: number
}

export interface Token {
  id: number
  user_id: number
  name: string
  prefix: string
  scope: 'read' | 'write'
  created_at: string
  last_used_at: string | null
  revoked_at: string | null
}

export interface Entitlement {
  user_id: number
  package_id: number
  expires_at: string | null
  granted_at: string
  active: boolean
}

export interface UserEntitlement extends Entitlement {
  package_name: string
  normalized_name: string
  latest_version: string
}

export interface PackageEntitlement extends Entitlement {
  username: string
  note: string
}

export interface Package {
  id: number
  name: string
  normalized_name: string
  summary: string
  description: string
  description_content_type: string
  created_at: string
  updated_at: string
}

export interface PackageSummary extends Package {
  latest_version: string
  release_count: number
  file_count: number
  user_count: number
  download_count: number
}

export interface FileInfo {
  id: number
  release_id: number
  filename: string
  sha256: string
  size: number
  requires_python: string
  metadata_sha256: string
  uploaded_at: string
}

export interface Release {
  id: number
  package_id: number
  version: string
  yanked: boolean
  yanked_reason: string
  created_at: string
  files: FileInfo[]
}

export interface PackageDetail {
  package: Package
  latest_version: string
  releases: Release[]
  users: PackageEntitlement[]
}

export interface UserDetail {
  user: User
  entitlements: UserEntitlement[]
  tokens: Token[]
}

export interface TokenCreated {
  token: string
  info: Token
  index_url: string
}

export interface UploadResult {
  filename: string
  package?: string
  version?: string
  error?: string
}

export interface Download {
  id: number
  filename: string
  ip: string
  user_agent: string
  at: string
  username: string
  package_name: string
}

export interface ServerConfig {
  base_url: string
  index_url: string
  upload_url: string
}

export interface SelfCheck {
  username: string
  note: string
  is_admin: boolean
  token: Token
  packages: UserEntitlement[]
  index_url: string
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method, credentials: 'same-origin', headers: {} }
  if (body instanceof FormData) {
    init.body = body
  } else if (body !== undefined) {
    init.headers = { 'Content-Type': 'application/json' }
    init.body = JSON.stringify(body)
  }
  const res = await fetch(path, init)
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`.trim()
    try {
      const data = await res.json()
      if (data && typeof data.error === 'string') message = data.error
    } catch {
      // not JSON; keep the status text
    }
    throw new ApiError(res.status, message)
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

const get = <T>(path: string) => request<T>('GET', path)
const post = <T>(path: string, body?: unknown) => request<T>('POST', path, body)
const put = <T>(path: string, body?: unknown) => request<T>('PUT', path, body)
const del = <T>(path: string) => request<T>('DELETE', path)
const enc = encodeURIComponent

export const api = {
  auth: {
    login: (username: string, password: string) => post<User>('/api/v1/auth/login', { username, password }),
    logout: () => post<void>('/api/v1/auth/logout'),
    me: () => get<User>('/api/v1/auth/me'),
    changePassword: (current: string, next: string) => put<void>('/api/v1/auth/password', { current, new: next }),
  },
  config: () => get<ServerConfig>('/api/v1/config'),
  packages: {
    list: () => get<PackageSummary[]>('/api/v1/packages'),
    get: (name: string) => get<PackageDetail>(`/api/v1/packages/${enc(name)}`),
    remove: (name: string) => del<void>(`/api/v1/packages/${enc(name)}`),
    upload: (files: File[]) => {
      const form = new FormData()
      for (const f of files) form.append('files', f, f.name)
      return post<UploadResult[]>('/api/v1/packages/upload', form)
    },
    yank: (name: string, version: string, yanked: boolean, reason = '') =>
      post<void>(`/api/v1/packages/${enc(name)}/releases/${enc(version)}/yank`, { yanked, reason }),
    removeRelease: (name: string, version: string) => del<void>(`/api/v1/packages/${enc(name)}/releases/${enc(version)}`),
  },
  users: {
    list: () => get<UserSummary[]>('/api/v1/users'),
    create: (username: string, note: string) => post<User>('/api/v1/users', { username, note }),
    get: (id: number) => get<UserDetail>(`/api/v1/users/${id}`),
    update: (id: number, username: string, note: string) => put<User>(`/api/v1/users/${id}`, { username, note }),
    remove: (id: number) => del<void>(`/api/v1/users/${id}`),
    grant: (id: number, pkg: string, expiresAt: string) =>
      put<void>(`/api/v1/users/${id}/entitlements/${enc(pkg)}`, { expires_at: expiresAt }),
    revoke: (id: number, pkg: string) => del<void>(`/api/v1/users/${id}/entitlements/${enc(pkg)}`),
    createToken: (id: number, name: string, scope: 'read' | 'write') =>
      post<TokenCreated>(`/api/v1/users/${id}/tokens`, { name, scope }),
  },
  tokens: {
    revoke: (id: number) => del<void>(`/api/v1/tokens/${id}`),
  },
  downloads: {
    list: (limit = 200) => get<Download[]>(`/api/v1/downloads?limit=${limit}`),
  },
  me: (token: string) => post<SelfCheck>('/api/v1/me', { token }),
}

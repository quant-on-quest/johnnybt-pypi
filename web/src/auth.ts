import { computed, ref } from 'vue'
import { api, type User } from './api'

// Module-level state: one admin session per browser tab.
const user = ref<User | null>(null)
const ready = ref(false)
let pending: Promise<void> | null = null

function isUnauthorized(err: unknown): boolean {
  return typeof err === 'object' && err !== null && (err as { status?: number }).status === 401
}

export function useAuth() {
  /** Resolve the current session once; later calls reuse the answer. */
  async function ensure(): Promise<void> {
    if (ready.value) return
    if (!pending) {
      pending = api.auth
        .me()
        .then((u) => {
          user.value = u
        })
        .catch((err) => {
          if (!isUnauthorized(err)) throw err
          user.value = null
        })
        .finally(() => {
          ready.value = true
          pending = null
        })
    }
    return pending
  }

  async function login(username: string, password: string): Promise<void> {
    user.value = await api.auth.login(username, password)
    ready.value = true
  }

  async function logout(): Promise<void> {
    try {
      await api.auth.logout()
    } finally {
      user.value = null
    }
  }

  /** Test hook: forget everything. */
  function reset() {
    user.value = null
    ready.value = false
    pending = null
  }

  return { user, ready, isLoggedIn: computed(() => user.value !== null), ensure, login, logout, reset }
}

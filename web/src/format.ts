export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 ** 2) return `${(n / 1024).toFixed(1)} KB`
  if (n < 1024 ** 3) return `${(n / 1024 ** 2).toFixed(1)} MB`
  return `${(n / 1024 ** 3).toFixed(2)} GB`
}

function pad(n: number) {
  return String(n).padStart(2, '0')
}

export function formatDate(iso: string | null | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

export function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return `${formatDate(iso)} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export type BadgeColor = 'success' | 'error' | 'warning' | 'neutral'

/** Human label + badge colour for an entitlement's expiry. */
export function expiryLabel(expiresAt: string | null, active: boolean): { text: string; color: BadgeColor } {
  if (!expiresAt) return { text: '永久', color: 'success' }
  // The server stores "end of day" as the start of the next day; show the
  // calendar day the customer was told.
  const d = new Date(expiresAt)
  const shown = new Date(d.getTime() - 1)
  const day = formatDate(shown.toISOString())
  return active ? { text: `${day} 到期`, color: 'success' } : { text: `${day} 已过期`, color: 'error' }
}

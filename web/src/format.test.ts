import { describe, expect, it } from 'vitest'
import { formatBytes, formatDate, formatDateTime, expiryLabel } from './format'

describe('format helpers', () => {
  it('formats bytes with sensible units', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(1018)).toBe('1018 B')
    expect(formatBytes(1536)).toBe('1.5 KB')
    expect(formatBytes(5 * 1024 * 1024)).toBe('5.0 MB')
    expect(formatBytes(3 * 1024 ** 3)).toBe('3.00 GB')
  })

  it('formats dates in local time and tolerates null', () => {
    expect(formatDate(null)).toBe('—')
    expect(formatDate('')).toBe('—')
    expect(formatDate('2026-09-11T10:40:17.719758369Z')).toMatch(/^\d{4}-\d{2}-\d{2}$/)
    expect(formatDateTime('2026-09-11T10:40:17Z')).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}$/)
  })

  it('describes entitlement expiry', () => {
    expect(expiryLabel(null, true)).toEqual({ text: '永久', color: 'success' })
    expect(expiryLabel('2099-01-01T00:00:00Z', true)).toEqual({ text: '2099-01-01 到期', color: 'success' })
    expect(expiryLabel('2020-01-01T00:00:00Z', false)).toEqual({ text: '2020-01-01 已过期', color: 'error' })
  })
})

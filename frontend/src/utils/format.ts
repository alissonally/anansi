// Display helpers for sizes, counts and server timestamps.

const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB']

/** Human readable size (1024-based, like HeidiSQL). */
export function formatBytes(n: number | null | undefined): string {
  if (n === null || n === undefined || Number.isNaN(n)) return ''
  let v = n
  let u = 0
  while (Math.abs(v) >= 1024 && u < units.length - 1) {
    v /= 1024
    u++
  }
  const digits = u === 0 ? 0 : v < 10 ? 2 : v < 100 ? 1 : 0
  return `${v.toLocaleString('pt-BR', { minimumFractionDigits: digits, maximumFractionDigits: digits })} ${units[u]}`
}

/** Integer/float with thousand separators; '' for null. */
export function formatCount(n: number | string | null | undefined, maxFraction = 0): string {
  if (n === null || n === undefined || n === '') return ''
  const v = typeof n === 'string' ? Number(n) : n
  if (Number.isNaN(v)) return String(n)
  return v.toLocaleString('pt-BR', { maximumFractionDigits: maxFraction })
}

/** Server "YYYY-MM-DD HH:MM:SS" → local display (kept as is: server time). */
export function formatServerTime(s: string | null | undefined): string {
  if (!s) return ''
  return s.replace(/\.\d+$/, '')
}

/** Seconds → "3 d 04:05:06". */
export function formatUptime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return ''
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  const hms = [h, m, s].map((x) => String(x).padStart(2, '0')).join(':')
  return d ? `${d} d ${hms}` : hms
}

/**
 * Builds a filter predicate from user text. Like HeidiSQL's tree filters,
 * ";" separates alternatives; each alternative is a case-insensitive regular
 * expression, falling back to a plain substring when it is not a valid regex.
 */
export function textMatcher(text: string): (s: string) => boolean {
  const parts = text.split(';').map((p) => p.trim()).filter(Boolean)
  if (!parts.length) return () => true
  const tests = parts.map((p) => {
    try {
      const re = new RegExp(p, 'i')
      return (s: string) => re.test(s)
    } catch {
      const low = p.toLowerCase()
      return (s: string) => s.toLowerCase().includes(low)
    }
  })
  return (s: string) => tests.some((t) => t(s))
}

/** Profile.databaseFilter ("loja_*; relatorios") → matcher (case-insensitive; empty matches all). */
export function globMatcher(spec: string | undefined): (name: string) => boolean {
  const parts = (spec ?? '').split(';').map((x) => x.trim()).filter(Boolean)
  if (!parts.length) return () => true
  const res = parts.map((g) => new RegExp('^' + g.replace(/[.+^${}()|[\]\\]/g, '\\$&').replace(/\*/g, '.*').replace(/\?/g, '.') + '$', 'i'))
  return (name) => res.some((r) => r.test(name))
}

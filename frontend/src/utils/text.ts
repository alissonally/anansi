const encoder = new TextEncoder()

/** Converts a UTF-16 offset of s into the UTF-8 byte offset used by Go. */
export function utf8ByteOffset(s: string, utf16Offset: number): number {
  return encoder.encode(s.slice(0, Math.max(0, Math.min(utf16Offset, s.length)))).length
}

/** Formats milliseconds for display. */
export function formatDuration(ms: number | null | undefined): string {
  if (ms === null || ms === undefined) return ''
  if (ms < 1) return '< 1 ms'
  if (ms < 1000) return `${Math.round(ms)} ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(2)} s`
  const m = Math.floor(ms / 60_000)
  const s = Math.round((ms % 60_000) / 1000)
  return `${m} min ${s} s`
}

export function formatNumber(n: number): string {
  return n.toLocaleString('pt-BR')
}

export function plural(n: number, one: string, many: string): string {
  return `${formatNumber(n)} ${n === 1 ? one : many}`
}

/** Quotes a MySQL identifier when needed. */
export function quoteIdent(name: string): string {
  return /^[A-Za-z_][A-Za-z0-9_$]*$/.test(name) ? name : '`' + name.replace(/`/g, '``') + '`'
}

/** Always quotes a MySQL identifier. */
export function forceQuoteIdent(name: string): string {
  return '`' + name.replace(/`/g, '``') + '`'
}

/** Deep copy of plain JSON data (works on Vue reactive proxies). */
export function clone<T>(v: T): T {
  return JSON.parse(JSON.stringify(v)) as T
}

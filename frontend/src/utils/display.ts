// Display-only formatting of grid values (preferences): digit grouping and
// integer columns shown as UNIX timestamps. Values sent back to the server
// are never formatted.

const parts = new Intl.NumberFormat().formatToParts(1234567.5)
const GROUP = parts.find((p) => p.type === 'group')?.value ?? ','
const DECIMAL = parts.find((p) => p.type === 'decimal')?.value ?? '.'

/** "-1234567.891" → "-1.234.567,891" (locale separators, exact digits). */
export function groupDigits(s: string, group = GROUP, decimal = DECIMAL): string {
  const m = /^([+-]?)(\d+)(?:\.(\d+))?$/.exec(s.trim())
  if (!m) return s
  const int = m[2]!.replace(/\B(?=(\d{3})+(?!\d))/g, group)
  return m[1]! + int + (m[3] !== undefined ? decimal + m[3] : '')
}

const pad = (n: number) => String(n).padStart(2, '0')

/** Seconds since 1970 (UTC) as local "YYYY-MM-DD HH:MM:SS"; null if not an integer. */
export function unixToLocal(s: string): string | null {
  if (!/^-?\d{1,12}$/.test(s.trim())) return null
  const d = new Date(Number(s) * 1000)
  if (Number.isNaN(d.getTime())) return null
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

/** Key of a column in Preferences.timestampColumns. */
export function timestampKey(conn: string, db: string, table: string, col: string): string {
  return [conn, db, table, col].join('|')
}

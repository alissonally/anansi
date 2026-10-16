// Model of the editable data grid: rows with pending changes, conversion of
// the pending changes into RowEdit requests, and HeidiSQL-style quick
// filters. Pure functions, unit tested.
import type { Cell, CellValue, ColumnInfo, ColumnValue, ResultColumn, RowEdit, TableKey } from '../api/types'
import { forceQuoteIdent } from './text'

export type EditorKind = 'text' | 'longtext' | 'json' | 'number' | 'binary' | 'bit' | 'enum' | 'set' | 'bool' | 'date' | 'datetime' | 'time' | 'year' | 'spatial'

export interface GridColumn extends ResultColumn {
  /** Catalog information (absent for computed query columns). */
  info?: ColumnInfo
  editor: EditorKind
  /** ENUM/SET options. */
  options?: string[]
  /** Loaded truncated (LEFT(...)) — the full value is fetched on demand. */
  partial?: boolean
  /** Foreign key target used by the lookup dropdown and "follow". */
  fk?: { database: string; table: string; column: string }
  editable: boolean
}

export interface GridRow {
  /** Stable identity for rendering. */
  uid: number
  /** Values as loaded (null for new rows). */
  orig: Cell[] | null
  /** Pending changes by column index. */
  edits: Map<number, CellValue>
  deleted: boolean
  /** Column indexes holding a truncated value. */
  truncated: Set<number>
  error?: string
}

let uidSeq = 1

export function makeRow(values: Cell[] | null, truncated: Iterable<number> = []): GridRow {
  return { uid: uidSeq++, orig: values, edits: new Map(), deleted: false, truncated: new Set(truncated) }
}

/** ENUM('a','b') / SET(...) options, unescaping quotes. */
export function parseEnumValues(columnType: string): string[] {
  const m = /^(?:enum|set)\((.*)\)$/is.exec(columnType.trim())
  if (!m) return []
  const out: string[] = []
  const re = /'((?:[^'\\]|''|\\.)*)'/g
  let x: RegExpExecArray | null
  while ((x = re.exec(m[1]!))) out.push(x[1]!.replace(/''/g, "'").replace(/\\(.)/g, '$1'))
  return out
}

export function editorKindOf(info: ColumnInfo): EditorKind {
  const t = info.dataType.toLowerCase()
  const ct = info.columnType.toLowerCase()
  if (t === 'tinyint' && /^tinyint\(1\)/.test(ct)) return 'bool'
  if (/^(tinyint|smallint|mediumint|int|integer|bigint|decimal|numeric|float|double|real)$/.test(t)) return 'number'
  if (t === 'bit') return 'bit'
  if (t === 'enum') return 'enum'
  if (t === 'set') return 'set'
  if (t === 'json') return 'json'
  if (t === 'date') return 'date'
  if (t === 'datetime' || t === 'timestamp') return 'datetime'
  if (t === 'time') return 'time'
  if (t === 'year') return 'year'
  if (/^(tinytext|text|mediumtext|longtext)$/.test(t)) return 'longtext'
  if (/^(binary|varbinary|tinyblob|blob|mediumblob|longblob)$/.test(t)) return 'binary'
  if (/^(geometry|point|linestring|polygon|multipoint|multilinestring|multipolygon|geometrycollection|geomcollection)$/.test(t)) return 'spatial'
  return 'text'
}

/** Value currently shown for a cell (pending change or original). */
export function currentValue(row: GridRow, col: number): CellValue | Cell {
  const e = row.edits.get(col)
  if (e) return e
  return row.orig ? (row.orig[col] ?? null) : { kind: 'default', text: '' }
}

export function isCellValue(v: unknown): v is CellValue {
  return typeof v === 'object' && v !== null && 'kind' in v && 'text' in v
}

/** Text shown in the grid for a value. */
export function displayValue(v: CellValue | Cell): { text: string; cls: string } {
  if (isCellValue(v)) {
    switch (v.kind) {
      case 'null': return { text: 'NULL', cls: 'isnull' }
      case 'default': return { text: '(DEFAULT)', cls: 'isdefault' }
      case 'expression': return { text: `ƒ ${v.text}`, cls: 'isexpr' }
      case 'hex': return { text: '0x' + v.text.toUpperCase(), cls: '' }
      default: return { text: v.text, cls: '' }
    }
  }
  if (v === null) return { text: 'NULL', cls: 'isnull' }
  return { text: String(v), cls: '' }
}

/** Converts a loaded cell into the value used to identify the row. */
export function cellToValue(v: Cell, col: GridColumn): CellValue {
  if (v === null) return { kind: 'null', text: '' }
  const s = String(v)
  if (col.binary && /^0x[0-9a-f]*$/i.test(s)) return { kind: 'hex', text: s.slice(2) }
  return { kind: 'value', text: typeof v === 'boolean' ? (v ? '1' : '0') : s }
}

export function rowIsDirty(r: GridRow): boolean {
  return r.deleted || r.orig === null || r.edits.size > 0
}

/**
 * Builds the RowEdit for a pending row. key lists the identifying columns;
 * with kind "none" every loaded column identifies the row.
 */
export function rowEdit(row: GridRow, cols: GridColumn[], key: TableKey): RowEdit | null {
  const name = (i: number) => cols[i]!.name
  const values: ColumnValue[] = [...row.edits.entries()].sort((a, b) => a[0] - b[0]).map(([i, v]) => ({ column: name(i), value: v }))
  if (row.orig === null) {
    if (row.deleted) return null
    return { op: 'insert', where: [], values }
  }
  const keyCols = key.kind === 'none' ? cols.map((c) => c.name) : (key.columns ?? [])
  const where: ColumnValue[] = []
  for (const k of keyCols) {
    const i = cols.findIndex((c) => c.name === k)
    if (i < 0) continue
    // A truncated value cannot identify a row.
    if (row.truncated.has(i)) continue
    where.push({ column: k, value: cellToValue(row.orig[i] ?? null, cols[i]!) })
  }
  if (row.deleted) return { op: 'delete', where, values: [] }
  if (!values.length) return null
  return { op: 'update', where, values }
}

// ---- quick filters ---------------------------------------------------------

/** SQL string literal for filters typed into the WHERE box. */
export function sqlString(s: string): string {
  return "'" + s.replace(/\\/g, '\\\\').replace(/'/g, "''") + "'"
}

/** Literal for a loaded cell in a filter, by column type. */
export function filterLiteral(v: Cell, col: GridColumn): string {
  if (v === null) return 'NULL'
  const s = String(v)
  if (col.binary && /^0x[0-9a-f]*$/i.test(s)) return s.length > 2 ? s : "X''"
  if (col.numeric && /^-?\d+(\.\d+)?(e[+-]?\d+)?$/i.test(s)) return s
  return sqlString(s)
}

export type QuickOp = '=' | '!=' | '>' | '<' | '>=' | '<=' | 'like' | 'starts' | 'ends' | 'null' | 'notnull' | 'in' | 'notin'

/** WHERE clause for a quick filter on a column. */
export function quickFilter(op: QuickOp, col: GridColumn, values: Cell[]): string {
  const c = forceQuoteIdent(col.name)
  const nonNull = values.filter((v) => v !== null)
  const hasNull = values.some((v) => v === null)
  const like = (pattern: (s: string) => string) =>
    nonNull.map((v) => `${c} LIKE ${sqlString(pattern(String(v).replace(/[\\%_]/g, (m) => '\\' + m)))}`).join(' OR ')
  switch (op) {
    case 'null': return `${c} IS NULL`
    case 'notnull': return `${c} IS NOT NULL`
    case 'in':
    case 'notin': {
      const lits = [...new Set(nonNull.map((v) => filterLiteral(v, col)))]
      const neg = op === 'notin'
      const parts: string[] = []
      if (lits.length) parts.push(`${c} ${neg ? 'NOT IN' : 'IN'} (${lits.join(', ')})`)
      if (hasNull) parts.push(`${c} ${neg ? 'IS NOT NULL' : 'IS NULL'}`)
      return parts.join(neg ? ' AND ' : ' OR ') || `${c} IS ${neg ? 'NOT ' : ''}NULL`
    }
    case 'like': return like((s) => `%${s}%`)
    case 'starts': return like((s) => `${s}%`)
    case 'ends': return like((s) => `%${s}`)
    default: {
      if (hasNull && nonNull.length === 0) return op === '!=' ? `${c} IS NOT NULL` : `${c} IS NULL`
      return nonNull.map((v) => `${c} ${op} ${filterLiteral(v, col)}`).join(' OR ')
    }
  }
}

/** Combines an existing filter with a new clause (AND, or replacing). */
export function combineFilter(current: string, clause: string, and: boolean): string {
  const cur = current.trim()
  if (!and || !cur) return clause
  return `(${cur}) AND (${clause})`
}

/** HeidiSQL "search text": CAST(col AS CHAR) LIKE '%text%' OR ... */
export function searchFilter(text: string, cols: GridColumn[]): string {
  const t = text.trim()
  if (!t) return ''
  const pattern = sqlString(`%${t.replace(/[\\%_]/g, (m) => '\\' + m)}%`)
  return cols.filter((c) => c.editor !== 'binary' && c.editor !== 'spatial').map((c) => `CAST(${forceQuoteIdent(c.name)} AS CHAR) LIKE ${pattern}`).join(' OR ')
}

/** Values for the "insert value" submenu (all evaluated by the server). */
export const insertableExpressions: { label: string; expr: string; kinds: EditorKind[] }[] = [
  { label: 'Data e hora atual', expr: 'NOW()', kinds: ['datetime', 'text', 'date', 'time'] },
  { label: 'Data e hora atual (UTC)', expr: 'UTC_TIMESTAMP()', kinds: ['datetime', 'text'] },
  { label: 'Data atual', expr: 'CURDATE()', kinds: ['date', 'datetime', 'text'] },
  { label: 'Hora atual', expr: 'CURTIME()', kinds: ['time', 'text'] },
  { label: 'Ano atual', expr: 'YEAR(NOW())', kinds: ['year', 'number', 'text'] },
  { label: 'Timestamp UNIX atual', expr: 'UNIX_TIMESTAMP()', kinds: ['number', 'text'] },
  { label: 'UUID', expr: 'UUID()', kinds: ['text', 'longtext', 'binary'] },
  { label: 'UUID binário (16 bytes)', expr: "UNHEX(REPLACE(UUID(), '-', ''))", kinds: ['binary'] },
]

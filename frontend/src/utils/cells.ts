import type { Cell, ResultColumn } from '../api/types'
import { forceQuoteIdent } from './text'

/** Text copied for a single cell; SQL NULL becomes "NULL". */
export function cellToText(v: Cell): string {
  return v === null ? 'NULL' : String(v)
}

function tsvField(v: Cell): string {
  if (v === null) return '\\N'
  const s = String(v)
  return /[\t\n\r"]/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s
}

/** Tab-separated rows (NULL as \N, like mysql --batch). */
export function rowsToTsv(cols: ResultColumn[], rows: Cell[][], header: boolean): string {
  const lines: string[] = []
  if (header) lines.push(cols.map((c) => tsvField(c.name)).join('\t'))
  for (const r of rows) lines.push(cols.map((_, i) => tsvField(r[i] ?? null)).join('\t'))
  return lines.join('\n')
}

/** SQL literal for a cell value. */
export function sqlLiteral(v: Cell, col: ResultColumn): string {
  if (v === null) return 'NULL'
  if (typeof v === 'number') return String(v)
  if (typeof v === 'boolean') return v ? '1' : '0'
  if (col.binary && /^0x[0-9A-F]*$/i.test(v)) return v
  if (col.numeric && /^-?\d+(\.\d+)?(e[+-]?\d+)?$/i.test(v)) return v
  return "'" + v.replace(/\\/g, '\\\\').replace(/'/g, "''") + "'"
}

/** INSERT statement reproducing one row. */
export function rowToInsert(table: string, cols: ResultColumn[], row: Cell[]): string {
  const names = cols.map((c) => forceQuoteIdent(c.name)).join(', ')
  const values = cols.map((c, i) => sqlLiteral(row[i] ?? null, c)).join(', ')
  return `INSERT INTO ${table} (${names}) VALUES (${values});`
}

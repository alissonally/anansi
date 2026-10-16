// SQL templates generated from a table's columns or a routine's parameters.
import type { ColumnInfo, RoutineDetail } from '../api/types'
import { forceQuoteIdent as q } from '../utils/text'

export type StatementKind = 'select' | 'insert' | 'update' | 'delete'

function placeholder(c: ColumnInfo): string {
  const t = c.dataType.toLowerCase()
  if (/int|decimal|numeric|float|double|bit|year/.test(t)) return '0'
  if (t === 'date') return "'2000-01-01'"
  if (t === 'datetime' || t === 'timestamp') return 'NOW()'
  if (t === 'time') return "'00:00:00'"
  if (t === 'json') return "'{}'"
  return "''"
}

/** SELECT/INSERT/UPDATE/DELETE skeleton for a table. */
export function tableStatement(kind: StatementKind, db: string, table: string, cols: ColumnInfo[]): string {
  const t = `${q(db)}.${q(table)}`
  const names = cols.map((c) => q(c.name))
  const keys = cols.filter((c) => c.key === 'PRI')
  const where = (keys.length ? keys : cols.slice(0, 1)).map((c) => `${q(c.name)} = ${placeholder(c)}`).join(' AND ')
  switch (kind) {
    case 'select':
      return `SELECT ${names.join(',\n       ')}\nFROM ${t}\nWHERE ${where};\n`
    case 'insert': {
      const ins = cols.filter((c) => !/auto_increment|VIRTUAL|STORED/i.test(c.extra ?? ''))
      return `INSERT INTO ${t}\n  (${ins.map((c) => q(c.name)).join(', ')})\nVALUES\n  (${ins.map(placeholder).join(', ')});\n`
    }
    case 'update': {
      const set = cols.filter((c) => c.key !== 'PRI' && !/VIRTUAL|STORED/i.test(c.extra ?? ''))
      return `UPDATE ${t}\nSET ${set.map((c) => `${q(c.name)} = ${placeholder(c)}`).join(',\n    ')}\nWHERE ${where};\n`
    }
    case 'delete':
      return `DELETE FROM ${t}\nWHERE ${where};\n`
  }
}

/** CALL proc(...) or SELECT func(...) for a routine. */
export function routineCall(db: string, r: RoutineDetail): string {
  const args = (r.params ?? []).map((p) => (p.mode === 'OUT' || p.mode === 'INOUT' ? `@${p.name}` : `/* ${p.name} ${p.type} */ NULL`))
  const name = `${q(db)}.${q(r.name)}`
  if (r.type === 'FUNCTION') return `SELECT ${name}(${args.join(', ')});\n`
  const outs = (r.params ?? []).filter((p) => p.mode === 'OUT' || p.mode === 'INOUT')
  return `CALL ${name}(${args.join(', ')});\n${outs.length ? `SELECT ${outs.map((p) => `@${p.name}`).join(', ')};\n` : ''}`
}

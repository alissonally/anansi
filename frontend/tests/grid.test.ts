import { describe, expect, it } from 'vitest'
import { combineFilter, displayValue, editorKindOf, makeRow, parseEnumValues, quickFilter, rowEdit, searchFilter, type GridColumn } from '../src/utils/grid'
import type { ColumnInfo } from '../src/api/types'

function col(name: string, extra: Partial<GridColumn> = {}): GridColumn {
  return { name, type: 'VARCHAR', binary: false, numeric: false, editor: 'text', editable: true, ...extra }
}

describe('grid model', () => {
  it('parses ENUM/SET values with quotes', () => {
    expect(parseEnumValues("enum('a','b''c','d,e')")).toEqual(['a', "b'c", 'd,e'])
    expect(parseEnumValues("set('x')")).toEqual(['x'])
    expect(parseEnumValues('int')).toEqual([])
  })

  it('chooses editors by type', () => {
    const info = (dataType: string, columnType = dataType): ColumnInfo => ({ name: 'c', position: 1, dataType, columnType, nullable: true })
    expect(editorKindOf(info('tinyint', 'tinyint(1)'))).toBe('bool')
    expect(editorKindOf(info('tinyint', 'tinyint(4) unsigned'))).toBe('number')
    expect(editorKindOf(info('json'))).toBe('json')
    expect(editorKindOf(info('longblob'))).toBe('binary')
    expect(editorKindOf(info('timestamp', 'timestamp(3)'))).toBe('datetime')
    expect(editorKindOf(info('point'))).toBe('spatial')
  })

  it('builds updates identified by the key and skips unchanged rows', () => {
    const cols = [col('id', { numeric: true }), col('nome'), col('foto', { binary: true })]
    const r = makeRow([1, 'Ana', '0xCAFE'])
    expect(rowEdit(r, cols, { kind: 'primary', columns: ['id'] })).toBeNull()
    r.edits.set(1, { kind: 'value', text: 'Bia' })
    expect(rowEdit(r, cols, { kind: 'primary', columns: ['id'] })).toEqual({
      op: 'update', where: [{ column: 'id', value: { kind: 'value', text: '1' } }], values: [{ column: 'nome', value: { kind: 'value', text: 'Bia' } }],
    })
    // Without a key every column identifies the row; binaries as hex.
    const e = rowEdit(r, cols, { kind: 'none', columns: null })!
    expect(e.where.map((w) => w.value)).toEqual([{ kind: 'value', text: '1' }, { kind: 'value', text: 'Ana' }, { kind: 'hex', text: 'CAFE' }])
    r.deleted = true
    expect(rowEdit(r, cols, { kind: 'primary', columns: ['id'] })!.op).toBe('delete')
    const n = makeRow(null)
    expect(rowEdit(n, cols, { kind: 'primary', columns: ['id'] })).toEqual({ op: 'insert', where: [], values: [] })
    n.deleted = true
    expect(rowEdit(n, cols, { kind: 'primary', columns: ['id'] })).toBeNull()
  })

  it('never identifies a row by a truncated value', () => {
    const cols = [col('a'), col('b')]
    const r = makeRow(['x', 'long…'], [1])
    r.deleted = true
    expect(rowEdit(r, cols, { kind: 'none', columns: null })!.where.map((w) => w.column)).toEqual(['a'])
  })

  it('generates quick filters like HeidiSQL', () => {
    const n = col('qtd', { numeric: true })
    const s = col('nome')
    expect(quickFilter('=', n, [5])).toBe('`qtd` = 5')
    expect(quickFilter('in', s, ["O'Neil", 'Ana', null])).toBe("`nome` IN ('O''Neil', 'Ana') OR `nome` IS NULL")
    expect(quickFilter('notin', s, ['a'])).toBe("`nome` NOT IN ('a')")
    expect(quickFilter('like', s, ['50%'])).toBe("`nome` LIKE '%50\\\\%%'")
    expect(quickFilter('=', s, [null])).toBe('`nome` IS NULL')
    expect(quickFilter('!=', s, [null])).toBe('`nome` IS NOT NULL')
    expect(combineFilter('a = 1', 'b = 2', true)).toBe('(a = 1) AND (b = 2)')
    expect(combineFilter('a = 1', 'b = 2', false)).toBe('b = 2')
    expect(searchFilter('x', [s, col('bin', { editor: 'binary' })])).toBe("CAST(`nome` AS CHAR) LIKE '%x%'")
  })

  it('displays pending values', () => {
    expect(displayValue({ kind: 'null', text: '' }).text).toBe('NULL')
    expect(displayValue({ kind: 'expression', text: 'NOW()' }).text).toBe('ƒ NOW()')
    expect(displayValue(null).cls).toBe('isnull')
  })
})

import { describe, expect, it } from 'vitest'
import { statementAt, statementRanges } from '../src/editor/statement'
import { cellToText, rowToInsert, rowsToTsv } from '../src/utils/cells'
import { formatDuration, utf8ByteOffset } from '../src/utils/text'
import { toAppError } from '../src/api/errors'
import type { ResultColumn } from '../src/api/types'

describe('statement ranges', () => {
  it('ignores delimiters in strings, identifiers and comments', () => {
    const sql = "SELECT ';'; -- a;b\nSELECT `x;y`; /* ; */ SELECT \"q;\""
    const r = statementRanges(sql).map((x) => sql.slice(x.start, x.end))
    expect(r).toEqual(["SELECT ';'", "-- a;b\nSELECT `x;y`", '/* ; */ SELECT "q;"'])
  })
  it('finds the statement under the cursor', () => {
    const sql = 'SELECT 1;\nSELECT 2;\n\nSELECT 3'
    expect(sql.slice(...Object.values(statementAt(sql, 12)!) as [number, number])).toBe('SELECT 2')
    expect(sql.slice(...Object.values(statementAt(sql, sql.length)!) as [number, number])).toBe('SELECT 3')
  })
})

describe('cells', () => {
  const cols: ResultColumn[] = [
    { name: 'id', type: 'INT', binary: false, numeric: true },
    { name: 'name', type: 'VARCHAR', binary: false, numeric: false },
    { name: 'raw', type: 'BLOB', binary: true, numeric: false },
  ]
  it('keeps NULL distinct from the string NULL', () => {
    expect(cellToText(null)).toBe('NULL')
    expect(rowsToTsv(cols, [[1, null, 'NULL']], false)).toBe('1\t\\N\tNULL')
  })
  it('builds INSERT statements with escaped literals', () => {
    expect(rowToInsert('`t`', cols, ['9007199254740993', "O'Brien \\ x", '0x00FF'])).toBe(
      "INSERT INTO `t` (`id`, `name`, `raw`) VALUES (9007199254740993, 'O''Brien \\\\ x', 0x00FF);",
    )
  })
})

describe('text', () => {
  it('converts UTF-16 offsets to UTF-8 byte offsets', () => {
    expect(utf8ByteOffset('aé😀b', 4)).toBe(7)
  })
  it('formats durations', () => {
    expect(formatDuration(0.2)).toBe('< 1 ms')
    expect(formatDuration(1500)).toBe('1.50 s')
  })
})

describe('errors', () => {
  it('decodes backend JSON errors', () => {
    const e = toAppError('{"code":"not_connected","message":"a conexão não está aberta"}')
    expect(e.code).toBe('not_connected')
    expect(toAppError(new Error('boom')).code).toBe('internal')
  })
})

import { describe, expect, it } from 'vitest'
import { findParams, paramLiteral, substituteParams } from '../src/editor/params'

describe('bind parameters', () => {
  it('finds :name outside strings, comments and assignments', () => {
    const sql = "SELECT :id, ':nao', `:nem`, @v := 1, a::text -- :comentario\n/* :bloco */ FROM t WHERE x = :id AND y = :nome#:fim"
    expect(findParams(sql)).toEqual(['id', 'nome'])
  })
  it('renders literals', () => {
    expect(paramLiteral('42')).toBe('42')
    expect(paramLiteral('NULL')).toBe('NULL')
    expect(paramLiteral("O'Neil")).toBe("'O''Neil'")
    expect(paramLiteral("'já citado'")).toBe("'já citado'")
  })
  it('substitutes every occurrence', () => {
    expect(substituteParams('SELECT * FROM t WHERE a = :a OR b = :a AND c = :c', { a: '1', c: 'x' })).toBe("SELECT * FROM t WHERE a = 1 OR b = 1 AND c = 'x'")
    expect(substituteParams("SELECT ':a', :b", { a: '1' })).toBe("SELECT ':a', :b")
  })
})

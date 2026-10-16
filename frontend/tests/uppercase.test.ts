import { describe, expect, it } from 'vitest'
import { inCode, keywordBefore } from '../src/editor/uppercase'

const apply = (text: string, typed = ' ') => {
  const r = keywordBefore(text + typed, text.length, typed)
  return r ? text.slice(0, r.start) + r.upper + text.slice(r.end) : text
}

describe('auto uppercase', () => {
  it('upper-cases keywords in code', () => {
    expect(apply('select')).toBe('SELECT')
    expect(apply('SELECT a from')).toBe('SELECT a FROM')
    expect(apply('where x in', '(')).toBe('where x IN')
  })
  it('leaves names, strings and comments alone', () => {
    expect(apply('SELECT nome')).toBe('SELECT nome')
    expect(apply("SELECT 'from")).toBe("SELECT 'from")
    expect(apply('SELECT `select')).toBe('SELECT `select')
    expect(apply('-- comment from')).toBe('-- comment from')
    expect(apply('/* where')).toBe('/* where')
    expect(apply('SELECT t.from')).toBe('SELECT t.from')
    expect(apply('SET @set')).toBe('SET @set')
    expect(apply('SELECT my_select')).toBe('SELECT my_select')
    expect(apply('select', 'x')).toBe('select')
  })
  it('tracks string and comment state', () => {
    expect(inCode("a = 'x' AND ")).toBe(true)
    expect(inCode("a = 'it''s")).toBe(false)
    expect(inCode('/* a */ b')).toBe(true)
    expect(inCode("'a\\'b' c")).toBe(true)
  })
})

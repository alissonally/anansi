import { describe, expect, it } from 'vitest'
import { groupDigits, unixToLocal } from '../src/utils/display'

describe('display formatting', () => {
  it('groups digits exactly', () => {
    expect(groupDigits('1234567', '.', ',')).toBe('1.234.567')
    expect(groupDigits('-1234567.891', '.', ',')).toBe('-1.234.567,891')
    expect(groupDigits('12345678901234567890', ',', '.')).toBe('12,345,678,901,234,567,890')
    expect(groupDigits('999', '.', ',')).toBe('999')
    expect(groupDigits('1e10', '.', ',')).toBe('1e10')
  })
  it('converts UNIX timestamps', () => {
    const s = unixToLocal('0')!
    expect(s).toMatch(/^19(69|70)-\d\d-\d\d \d\d:\d\d:\d\d$/)
    expect(unixToLocal('abc')).toBeNull()
    expect(unixToLocal('1.5')).toBeNull()
    expect(unixToLocal(String(Date.UTC(2024, 0, 2, 3, 4, 5) / 1000))).toBe((() => {
      const d = new Date(Date.UTC(2024, 0, 2, 3, 4, 5))
      const p = (n: number) => String(n).padStart(2, '0')
      return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
    })())
  })
})

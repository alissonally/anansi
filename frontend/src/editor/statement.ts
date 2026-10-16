// Lightweight statement boundary detection for the editor (highlighting the
// statement that "Run statement" will execute). The authoritative split is
// done by the Go backend; this mirrors its rules for ';', quotes and comments.

export interface Range {
  start: number
  end: number
}

/** Returns [start, end) UTF-16 ranges of statements in sql. */
export function statementRanges(sql: string): Range[] {
  const out: Range[] = []
  let segStart = 0
  let i = 0
  const n = sql.length
  const push = (end: number) => {
    let s = segStart
    let e = end
    while (s < e && /\s/.test(sql[s]!)) s++
    while (e > s && /\s/.test(sql[e - 1]!)) e--
    if (e > s) out.push({ start: s, end: e })
  }
  while (i < n) {
    const c = sql[i]!
    if (c === '\'' || c === '"' || c === '`') {
      i++
      while (i < n) {
        if (sql[i] === '\\' && c !== '`') { i += 2; continue }
        if (sql[i] === c) {
          if (sql[i + 1] === c) { i += 2; continue }
          break
        }
        i++
      }
      i++
      continue
    }
    if (c === '#' || (c === '-' && sql[i + 1] === '-' && (i + 2 >= n || /\s/.test(sql[i + 2]!)))) {
      while (i < n && sql[i] !== '\n') i++
      continue
    }
    if (c === '/' && sql[i + 1] === '*') {
      const end = sql.indexOf('*/', i + 2)
      i = end < 0 ? n : end + 2
      continue
    }
    if (c === ';') {
      push(i)
      segStart = i + 1
    }
    i++
  }
  push(n)
  return out
}

/** Statement containing offset, or the closest one before it. */
export function statementAt(sql: string, offset: number): Range | null {
  const ranges = statementRanges(sql)
  let best: Range | null = null
  for (const r of ranges) {
    if (offset >= r.start && offset <= r.end) return r
    if (r.end <= offset) best = r
  }
  return best ?? ranges[0] ?? null
}

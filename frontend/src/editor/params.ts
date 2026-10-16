// Bind parameters (:name) of the SQL tabs, like HeidiSQL's "Bind parameters":
// detected outside strings, quoted identifiers and comments, and replaced by
// SQL literals before running.

interface Hit {
  name: string
  start: number
  end: number
}

function scan(sql: string): Hit[] {
  const out: Hit[] = []
  let i = 0
  const n = sql.length
  while (i < n) {
    const c = sql[i]!
    const next = sql[i + 1]
    if (c === "'" || c === '"' || c === '`') {
      i++
      while (i < n && sql[i] !== c) i += sql[i] === '\\' && c !== '`' ? 2 : 1
      i++
      continue
    }
    if ((c === '-' && next === '-') || c === '#') {
      while (i < n && sql[i] !== '\n') i++
      continue
    }
    if (c === '/' && next === '*') {
      const e = sql.indexOf('*/', i + 2)
      i = e < 0 ? n : e + 2
      continue
    }
    if (c === ':' && next !== undefined && /[A-Za-z_]/.test(next) && !(i > 0 && /[\w:]/.test(sql[i - 1]!))) {
      let j = i + 1
      while (j < n && /\w/.test(sql[j]!)) j++
      out.push({ name: sql.slice(i + 1, j), start: i, end: j })
      i = j
      continue
    }
    i++
  }
  return out
}

/** Distinct parameter names in order of appearance. */
export function findParams(sql: string): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const h of scan(sql)) {
    if (!seen.has(h.name)) {
      seen.add(h.name)
      out.push(h.name)
    }
  }
  return out
}

/** SQL literal for a typed value: numbers, NULL and quoted text stay as typed. */
export function paramLiteral(v: string): string {
  const t = v.trim()
  if (/^-?\d+(\.\d+)?([eE][-+]?\d+)?$/.test(t) || /^null$/i.test(t)) return t
  if (/^'.*'$/s.test(t) || /^0x[0-9a-f]+$/i.test(t)) return t
  return "'" + v.replace(/\\/g, '\\\\').replace(/'/g, "''") + "'"
}

/** Replaces every :name with its literal; unknown names are kept. */
export function substituteParams(sql: string, values: Record<string, string>): string {
  let out = ''
  let last = 0
  for (const h of scan(sql)) {
    if (!(h.name in values)) continue
    out += sql.slice(last, h.start) + paramLiteral(values[h.name]!)
    last = h.end
  }
  return out + sql.slice(last)
}

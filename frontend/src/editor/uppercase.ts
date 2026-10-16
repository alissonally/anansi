// Upper-cases SQL keywords as they are typed (HeidiSQL "Auto uppercase").
// Pure helpers; the editor calls keywordBefore() after a separator is typed.

export const KEYWORDS = new Set(`
ADD ALL ALTER AND ANY AS ASC AUTO_INCREMENT BEGIN BETWEEN BIGINT BINARY BLOB BOTH BY CALL CASCADE CASE CHANGE CHAR CHARACTER CHARSET
CHECK COLLATE COLUMN COMMENT COMMIT CONSTRAINT CREATE CROSS CURRENT_DATE CURRENT_TIME CURRENT_TIMESTAMP DATABASE DATE DATETIME DECIMAL
DECLARE DEFAULT DELETE DESC DESCRIBE DISTINCT DIV DO DOUBLE DROP DUPLICATE EACH ELSE ELSEIF END ENGINE ENUM ESCAPE EVENT EXISTS
EXPLAIN FALSE FETCH FLOAT FOR FOREIGN FROM FULL FULLTEXT FUNCTION GRANT GROUP HAVING IF IGNORE IN INDEX INNER INSERT INT INTEGER
INTERVAL INTO IS JOIN JSON KEY KEYS KILL LEADING LEFT LIKE LIMIT LONGTEXT LOOP MEDIUMINT MEDIUMTEXT MOD MODIFY NATURAL NOT NULL
NUMERIC OFFSET ON OR ORDER OUTER PARTITION PRIMARY PROCEDURE REFERENCES REGEXP RENAME REPEAT REPLACE RETURN RETURNS REVOKE RIGHT
RLIKE ROLLBACK ROW ROWS SCHEMA SELECT SET SHOW SIGNAL SMALLINT START TABLE TEMPORARY TEXT THEN TIME TIMESTAMP TINYINT TINYTEXT TO
TRAILING TRANSACTION TRIGGER TRUE TRUNCATE UNION UNIQUE UNSIGNED UPDATE USE USING VALUES VARBINARY VARCHAR VIEW WHEN WHERE WHILE WITH
XOR YEAR ZEROFILL
`.split(/\s+/).filter(Boolean))

const SEPARATORS = /^[\s,;()=<>+\-*/]$/

/** True when the end of text is outside strings, quoted names and comments. */
export function inCode(text: string): boolean {
  let i = 0
  const n = text.length
  while (i < n) {
    const c = text[i]!
    if (c === "'" || c === '"' || c === '`') {
      const q = c
      i++
      while (i < n && text[i] !== q) i += text[i] === '\\' && q !== '`' ? 2 : 1
      if (i >= n) return false
      i++
    } else if (c === '#' || (c === '-' && text[i + 1] === '-' && /\s/.test(text[i + 2] ?? ' '))) {
      const e = text.indexOf('\n', i)
      if (e < 0) return false
      i = e + 1
    } else if (c === '/' && text[i + 1] === '*') {
      const e = text.indexOf('*/', i + 2)
      if (e < 0) return false
      i = e + 2
    } else i++
  }
  return true
}

/**
 * After `typed` was inserted at offset `at` of `text`, returns the keyword
 * right before it to upper-case, as [start, end) offsets and replacement.
 */
export function keywordBefore(text: string, at: number, typed: string): { start: number; end: number; upper: string } | null {
  if (!SEPARATORS.test(typed)) return null
  let s = at
  while (s > 0 && /[A-Za-z_]/.test(text[s - 1]!)) s--
  if (s === at || (s > 0 && /[\w$.@`]/.test(text[s - 1]!))) return null // part of a longer name, qualified or a variable
  const word = text.slice(s, at)
  const upper = word.toUpperCase()
  if (word === upper || !KEYWORDS.has(upper)) return null
  // Function-like names before "(" are left alone only when not keywords.
  if (!inCode(text.slice(0, s))) return null
  return { start: s, end: at, upper }
}

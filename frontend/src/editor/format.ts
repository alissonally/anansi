import { format } from 'sql-formatter'

/** Formats SQL with the MySQL/MariaDB dialect. Throws on unparsable input. */
export function formatSql(sql: string, dialect: 'mysql' | 'mariadb' = 'mysql'): string {
  return format(sql, { language: dialect, keywordCase: 'upper', tabWidth: 2, linesBetweenQueries: 1 })
}

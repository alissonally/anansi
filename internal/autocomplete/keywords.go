package autocomplete

import (
	"sort"
	"strings"
)

// reservedWords are MySQL 8 / MariaDB reserved words (cannot be used as
// unquoted identifiers or aliases). Source: MySQL 8.4 reference manual,
// "Keywords and Reserved Words", plus MariaDB additions.
var reservedWords = toSet(`ACCESSIBLE ADD ALL ALTER ANALYZE AND AS ASC ASENSITIVE BEFORE BETWEEN BIGINT BINARY BLOB
BOTH BY CALL CASCADE CASE CHANGE CHAR CHARACTER CHECK COLLATE COLUMN CONDITION CONSTRAINT CONTINUE CONVERT
CREATE CROSS CUBE CUME_DIST CURRENT_DATE CURRENT_ROLE CURRENT_TIME CURRENT_TIMESTAMP CURRENT_USER CURSOR DATABASE
DATABASES DAY_HOUR DAY_MICROSECOND DAY_MINUTE DAY_SECOND DEC DECIMAL DECLARE DEFAULT DELAYED DELETE DENSE_RANK
DESC DESCRIBE DETERMINISTIC DISTINCT DISTINCTROW DIV DOUBLE DROP DUAL EACH ELSE ELSEIF EMPTY ENCLOSED ESCAPED
EXCEPT EXISTS EXIT EXPLAIN FALSE FETCH FIRST_VALUE FLOAT FLOAT4 FLOAT8 FOR FORCE FOREIGN FROM FULLTEXT FUNCTION
GENERATED GET GRANT GROUP GROUPING GROUPS HAVING HIGH_PRIORITY HOUR_MICROSECOND HOUR_MINUTE HOUR_SECOND IF IGNORE
IN INDEX INFILE INNER INOUT INSENSITIVE INSERT INT INT1 INT2 INT3 INT4 INT8 INTEGER INTERSECT INTERVAL INTO
IO_AFTER_GTIDS IO_BEFORE_GTIDS IS ITERATE JOIN JSON_TABLE KEY KEYS KILL LAG LAST_VALUE LATERAL LEAD LEADING LEAVE
LEFT LIKE LIMIT LINEAR LINES LOAD LOCALTIME LOCALTIMESTAMP LOCK LONG LONGBLOB LONGTEXT LOOP LOW_PRIORITY
MASTER_BIND MASTER_SSL_VERIFY_SERVER_CERT MATCH MAXVALUE MEDIUMBLOB MEDIUMINT MEDIUMTEXT MIDDLEINT
MINUTE_MICROSECOND MINUTE_SECOND MOD MODIFIES NATURAL NOT NO_WRITE_TO_BINLOG NTH_VALUE NTILE NULL NUMERIC OF ON
OPTIMIZE OPTIMIZER_COSTS OPTION OPTIONALLY OR ORDER OUT OUTER OUTFILE OVER PARTITION PERCENT_RANK PRECISION
PRIMARY PROCEDURE PURGE RANGE RANK READ READS READ_WRITE REAL RECURSIVE REFERENCES REGEXP RELEASE RENAME REPEAT
REPLACE REQUIRE RESIGNAL RESTRICT RETURN RETURNING REVOKE RIGHT RLIKE ROW ROWS ROW_NUMBER SCHEMA SCHEMAS
SECOND_MICROSECOND SELECT SENSITIVE SEPARATOR SET SHOW SIGNAL SMALLINT SPATIAL SPECIFIC SQL SQLEXCEPTION SQLSTATE
SQLWARNING SQL_BIG_RESULT SQL_CALC_FOUND_ROWS SQL_SMALL_RESULT SSL STARTING STORED STRAIGHT_JOIN SYSTEM TABLE
TERMINATED THEN TINYBLOB TINYINT TINYTEXT TO TRAILING TRIGGER TRUE UNDO UNION UNIQUE UNLOCK UNSIGNED UPDATE USAGE
USE USING UTC_DATE UTC_TIME UTC_TIMESTAMP VALUES VARBINARY VARCHAR VARCHARACTER VARYING VIRTUAL WHEN WHERE WHILE
WINDOW WITH WRITE XOR YEAR_MONTH ZEROFILL`)

// clauseWords cannot be table aliases even when they are not reserved, so
// they end a table reference.
var clauseWords = toSet(`WHERE JOIN INNER LEFT RIGHT CROSS NATURAL STRAIGHT_JOIN OUTER ON USING GROUP ORDER HAVING
LIMIT UNION EXCEPT INTERSECT SET VALUES VALUE SELECT WINDOW FOR LOCK PARTITION INTO FROM USE FORCE IGNORE AS
RETURNING OFFSET`)

// statementKeywords start statements.
var statementKeywords = []string{
	"SELECT", "INSERT INTO", "UPDATE", "DELETE FROM", "REPLACE INTO", "WITH", "CREATE TABLE", "CREATE VIEW",
	"CREATE INDEX", "CREATE DATABASE", "ALTER TABLE", "DROP TABLE", "DROP VIEW", "DROP INDEX", "TRUNCATE TABLE",
	"RENAME TABLE", "SHOW DATABASES", "SHOW TABLES", "SHOW COLUMNS FROM", "SHOW INDEX FROM", "SHOW CREATE TABLE",
	"SHOW PROCESSLIST", "SHOW VARIABLES", "SHOW STATUS", "SHOW GRANTS", "DESCRIBE", "EXPLAIN", "USE", "SET",
	"START TRANSACTION", "BEGIN", "COMMIT", "ROLLBACK", "SAVEPOINT", "CALL", "GRANT", "REVOKE", "ANALYZE TABLE",
	"OPTIMIZE TABLE", "LOCK TABLES", "UNLOCK TABLES",
}

// keywordsAfter lists the keywords that typically follow each clause.
var keywordsAfter = map[string][]string{
	"select":     {"FROM", "AS", "DISTINCT", "CASE", "WHEN", "THEN", "ELSE", "END", "AND", "OR", "NOT", "NULL", "IS", "IN", "LIKE", "BETWEEN", "OVER", "PARTITION BY"},
	"afterTable": {"WHERE", "JOIN", "INNER JOIN", "LEFT JOIN", "RIGHT JOIN", "CROSS JOIN", "ON", "USING", "AS", "GROUP BY", "ORDER BY", "HAVING", "LIMIT", "UNION", "SET", "FOR UPDATE"},
	"where":      {"AND", "OR", "NOT", "NULL", "IS", "IS NOT NULL", "IN", "LIKE", "BETWEEN", "EXISTS", "GROUP BY", "ORDER BY", "LIMIT", "HAVING", "CASE", "WHEN", "THEN", "ELSE", "END", "TRUE", "FALSE"},
	"on":         {"AND", "OR", "NOT", "NULL", "IS", "IN", "WHERE", "JOIN", "INNER JOIN", "LEFT JOIN", "RIGHT JOIN", "GROUP BY", "ORDER BY", "LIMIT"},
	"groupby":    {"HAVING", "ORDER BY", "LIMIT", "WITH ROLLUP", "ASC", "DESC"},
	"orderby":    {"ASC", "DESC", "LIMIT", "NULLS FIRST", "NULLS LAST"},
	"having":     {"AND", "OR", "NOT", "ORDER BY", "LIMIT"},
	"set":        {"WHERE", "DEFAULT", "NULL", "ORDER BY", "LIMIT"},
	"values":     {"NULL", "DEFAULT", "ON DUPLICATE KEY UPDATE"},
	"limit":      {"OFFSET"},
	"into":       {"VALUES", "SELECT", "SET"},
}

// functions are common MySQL/MariaDB built-in functions with a short
// signature shown as detail.
var functions = map[string]string{
	"COUNT": "COUNT(expr)", "SUM": "SUM(expr)", "AVG": "AVG(expr)", "MIN": "MIN(expr)", "MAX": "MAX(expr)",
	"GROUP_CONCAT": "GROUP_CONCAT(expr [ORDER BY ...] [SEPARATOR str])", "JSON_ARRAYAGG": "JSON_ARRAYAGG(expr)",
	"JSON_OBJECTAGG": "JSON_OBJECTAGG(key, value)", "STD": "STD(expr)", "VARIANCE": "VARIANCE(expr)",
	"BIT_AND": "BIT_AND(expr)", "BIT_OR": "BIT_OR(expr)",
	"CONCAT": "CONCAT(str1, str2, ...)", "CONCAT_WS": "CONCAT_WS(sep, str1, ...)", "SUBSTRING": "SUBSTRING(str, pos [, len])",
	"SUBSTRING_INDEX": "SUBSTRING_INDEX(str, delim, count)", "LEFT": "LEFT(str, len)", "RIGHT": "RIGHT(str, len)",
	"LENGTH": "LENGTH(str)", "CHAR_LENGTH": "CHAR_LENGTH(str)", "LOWER": "LOWER(str)", "UPPER": "UPPER(str)",
	"TRIM": "TRIM([BOTH|LEADING|TRAILING] [remstr FROM] str)", "LTRIM": "LTRIM(str)", "RTRIM": "RTRIM(str)",
	"REPLACE": "REPLACE(str, from, to)", "REVERSE": "REVERSE(str)", "LPAD": "LPAD(str, len, pad)", "RPAD": "RPAD(str, len, pad)",
	"LOCATE": "LOCATE(substr, str [, pos])", "INSTR": "INSTR(str, substr)", "FORMAT": "FORMAT(x, d)",
	"REGEXP_LIKE": "REGEXP_LIKE(expr, pat)", "REGEXP_REPLACE": "REGEXP_REPLACE(expr, pat, repl)", "REGEXP_SUBSTR": "REGEXP_SUBSTR(expr, pat)",
	"HEX": "HEX(x)", "UNHEX": "UNHEX(str)", "MD5": "MD5(str)", "SHA2": "SHA2(str, len)", "UUID": "UUID()", "TO_BASE64": "TO_BASE64(str)",
	"FROM_BASE64": "FROM_BASE64(str)", "ROUND": "ROUND(x [, d])", "CEIL": "CEIL(x)", "FLOOR": "FLOOR(x)", "ABS": "ABS(x)",
	"MOD": "MOD(n, m)", "POWER": "POWER(x, y)", "SQRT": "SQRT(x)", "RAND": "RAND()", "TRUNCATE": "TRUNCATE(x, d)", "SIGN": "SIGN(x)",
	"NOW": "NOW()", "CURDATE": "CURDATE()", "CURTIME": "CURTIME()", "SYSDATE": "SYSDATE()", "UTC_TIMESTAMP": "UTC_TIMESTAMP()",
	"DATE": "DATE(expr)", "TIME": "TIME(expr)", "YEAR": "YEAR(date)", "MONTH": "MONTH(date)", "DAY": "DAY(date)",
	"HOUR": "HOUR(time)", "MINUTE": "MINUTE(time)", "SECOND": "SECOND(time)", "DAYOFWEEK": "DAYOFWEEK(date)", "WEEK": "WEEK(date)",
	"DATE_FORMAT": "DATE_FORMAT(date, format)", "STR_TO_DATE": "STR_TO_DATE(str, format)", "DATE_ADD": "DATE_ADD(date, INTERVAL expr unit)",
	"DATE_SUB": "DATE_SUB(date, INTERVAL expr unit)", "DATEDIFF": "DATEDIFF(d1, d2)", "TIMESTAMPDIFF": "TIMESTAMPDIFF(unit, t1, t2)",
	"UNIX_TIMESTAMP": "UNIX_TIMESTAMP([date])", "FROM_UNIXTIME": "FROM_UNIXTIME(ts [, format])", "LAST_DAY": "LAST_DAY(date)",
	"CONVERT_TZ": "CONVERT_TZ(dt, from_tz, to_tz)", "EXTRACT": "EXTRACT(unit FROM date)",
	"IF": "IF(cond, then, else)", "IFNULL": "IFNULL(expr, alt)", "NULLIF": "NULLIF(a, b)", "COALESCE": "COALESCE(v1, v2, ...)",
	"GREATEST": "GREATEST(v1, v2, ...)", "LEAST": "LEAST(v1, v2, ...)", "CAST": "CAST(expr AS type)", "CONVERT": "CONVERT(expr, type)",
	"JSON_EXTRACT": "JSON_EXTRACT(doc, path)", "JSON_UNQUOTE": "JSON_UNQUOTE(json)", "JSON_OBJECT": "JSON_OBJECT(k, v, ...)",
	"JSON_ARRAY": "JSON_ARRAY(v, ...)", "JSON_CONTAINS": "JSON_CONTAINS(target, candidate [, path])", "JSON_SET": "JSON_SET(doc, path, val)",
	"JSON_LENGTH": "JSON_LENGTH(doc [, path])", "JSON_KEYS": "JSON_KEYS(doc)", "JSON_VALID": "JSON_VALID(val)",
	"ROW_NUMBER": "ROW_NUMBER() OVER (...)", "RANK": "RANK() OVER (...)", "DENSE_RANK": "DENSE_RANK() OVER (...)",
	"LAG": "LAG(expr [, n]) OVER (...)", "LEAD": "LEAD(expr [, n]) OVER (...)", "FIRST_VALUE": "FIRST_VALUE(expr) OVER (...)",
	"LAST_VALUE": "LAST_VALUE(expr) OVER (...)", "NTILE": "NTILE(n) OVER (...)",
	"DATABASE": "DATABASE()", "USER": "USER()", "CURRENT_USER": "CURRENT_USER()", "VERSION": "VERSION()",
	"LAST_INSERT_ID": "LAST_INSERT_ID()", "ROW_COUNT": "ROW_COUNT()", "FOUND_ROWS": "FOUND_ROWS()", "CONNECTION_ID": "CONNECTION_ID()",
	"INET_ATON": "INET_ATON(ip)", "INET_NTOA": "INET_NTOA(n)", "SLEEP": "SLEEP(seconds)", "BENCHMARK": "BENCHMARK(count, expr)",
}

// dataTypes are column types, suggested in CREATE/ALTER and CAST contexts.
var dataTypes = []string{
	"INT", "TINYINT", "SMALLINT", "MEDIUMINT", "BIGINT", "DECIMAL", "NUMERIC", "FLOAT", "DOUBLE", "BIT", "BOOLEAN",
	"CHAR", "VARCHAR", "BINARY", "VARBINARY", "TEXT", "TINYTEXT", "MEDIUMTEXT", "LONGTEXT", "BLOB", "TINYBLOB",
	"MEDIUMBLOB", "LONGBLOB", "ENUM", "SET", "DATE", "TIME", "DATETIME", "TIMESTAMP", "YEAR", "JSON", "GEOMETRY",
	"POINT", "UUID", "INET4", "INET6", "UNSIGNED", "SIGNED",
}

// ddlKeywords are suggested inside CREATE/ALTER statements.
var ddlKeywords = []string{
	"PRIMARY KEY", "FOREIGN KEY", "REFERENCES", "UNIQUE", "INDEX", "KEY", "NOT NULL", "NULL", "DEFAULT", "AUTO_INCREMENT",
	"COMMENT", "CONSTRAINT", "CHECK", "ON DELETE", "ON UPDATE", "CASCADE", "RESTRICT", "SET NULL", "ADD COLUMN",
	"DROP COLUMN", "MODIFY COLUMN", "CHANGE COLUMN", "RENAME TO", "ADD INDEX", "DROP INDEX", "ENGINE", "CHARSET",
	"COLLATE", "IF NOT EXISTS", "IF EXISTS", "AFTER", "FIRST", "CURRENT_TIMESTAMP",
}

func toSet(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(s) {
		m[w] = true
	}
	return m
}

// IsReserved reports whether w is a reserved word.
func IsReserved(w string) bool { return reservedWords[strings.ToUpper(w)] }

// Function is a built-in function with its signature (query helpers panel).
type Function struct {
	Name      string `json:"name"`
	Signature string `json:"signature"`
}

// Vocabulary returns the reserved words and the built-in functions known to
// the autocompletion, sorted by name.
func Vocabulary() (keywords []string, funcs []Function) {
	for w := range reservedWords {
		keywords = append(keywords, w)
	}
	sort.Strings(keywords)
	for n, sig := range functions {
		funcs = append(funcs, Function{Name: n, Signature: sig})
	}
	sort.Slice(funcs, func(i, j int) bool { return funcs[i].Name < funcs[j].Name })
	return keywords, funcs
}

package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"anansi/internal/database"
)

var _ database.DDLBuilder = (*Driver)(nil)

// typeRule describes which arguments a column type accepts.
type typeRule int

const (
	argNone      typeRule = iota // TEXT, DATE, JSON ...
	argOptInt                    // INT[(n)], CHAR[(n)], DATETIME[(fsp)]
	argReqInt                    // VARCHAR(n)
	argPrecision                 // DECIMAL[(p[,s])], FLOAT[(p,s)]
	argValues                    // ENUM('a',...), SET('a',...)
)

type typeInfo struct {
	rule     typeRule
	numeric  bool
	integer  bool
	temporal bool // accepts CURRENT_TIMESTAMP default / ON UPDATE
}

var columnTypes = map[string]typeInfo{
	"TINYINT": {argOptInt, true, true, false}, "SMALLINT": {argOptInt, true, true, false}, "MEDIUMINT": {argOptInt, true, true, false},
	"INT": {argOptInt, true, true, false}, "BIGINT": {argOptInt, true, true, false},
	"DECIMAL": {argPrecision, true, false, false}, "NUMERIC": {argPrecision, true, false, false},
	"FLOAT": {argPrecision, true, false, false}, "DOUBLE": {argPrecision, true, false, false},
	"BIT": {argOptInt, false, false, false}, "BOOLEAN": {argNone, false, false, false},
	"CHAR": {argOptInt, false, false, false}, "VARCHAR": {argReqInt, false, false, false},
	"BINARY": {argOptInt, false, false, false}, "VARBINARY": {argReqInt, false, false, false},
	"TINYTEXT": {argNone, false, false, false}, "TEXT": {argNone, false, false, false},
	"MEDIUMTEXT": {argNone, false, false, false}, "LONGTEXT": {argNone, false, false, false},
	"TINYBLOB": {argNone, false, false, false}, "BLOB": {argNone, false, false, false},
	"MEDIUMBLOB": {argNone, false, false, false}, "LONGBLOB": {argNone, false, false, false},
	"ENUM": {argValues, false, false, false}, "SET": {argValues, false, false, false},
	"DATE": {argNone, false, false, false}, "TIME": {argOptInt, false, false, false},
	"DATETIME": {argOptInt, false, false, true}, "TIMESTAMP": {argOptInt, false, false, true},
	"YEAR": {argNone, false, false, false}, "JSON": {argNone, false, false, false},
	"GEOMETRY": {argNone, false, false, false}, "POINT": {argNone, false, false, false},
	"LINESTRING": {argNone, false, false, false}, "POLYGON": {argNone, false, false, false},
	// MariaDB only; MySQL rejects them with a clear server error.
	"UUID": {argNone, false, false, false}, "INET4": {argNone, false, false, false}, "INET6": {argNone, false, false, false},
}

// typeOrder is the order offered in the UI.
var typeOrder = []string{
	"INT", "BIGINT", "TINYINT", "SMALLINT", "MEDIUMINT", "DECIMAL", "FLOAT", "DOUBLE", "BOOLEAN", "BIT",
	"VARCHAR", "CHAR", "TEXT", "TINYTEXT", "MEDIUMTEXT", "LONGTEXT", "ENUM", "SET", "JSON",
	"DATE", "DATETIME", "TIMESTAMP", "TIME", "YEAR",
	"BINARY", "VARBINARY", "BLOB", "TINYBLOB", "MEDIUMBLOB", "LONGBLOB",
	"GEOMETRY", "POINT", "LINESTRING", "POLYGON", "UUID", "INET4", "INET6", "NUMERIC",
}

var (
	optionNameRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)
	intArgRe     = regexp.MustCompile(`^\d{1,5}$`)
	precisionRe  = regexp.MustCompile(`^\d{1,2}(,\d{1,2})?$`)
	numberRe     = regexp.MustCompile(`^-?(\d+(\.\d*)?|\.\d+)([eE][-+]?\d+)?$`)
)

// ddlError builds a validation error.
func ddlError(format string, a ...any) error {
	return &database.DBError{Category: "validation", Message: fmt.Sprintf(format, a...)}
}

// checkObjectName validates database and table names.
func checkObjectName(kind, name string) error {
	if strings.TrimSpace(name) == "" {
		return ddlError("informe o nome do %s", kind)
	}
	if utf8.RuneCountInString(name) > 64 {
		return ddlError("o nome do %s deve ter no máximo 64 caracteres", kind)
	}
	if strings.HasSuffix(name, " ") || strings.ContainsAny(name, "/\\.\x00") {
		return ddlError("nome de %s inválido: não use '/', '\\', '.' nem espaço no final", kind)
	}
	return nil
}

func checkColumnName(name string) error {
	if strings.TrimSpace(name) == "" {
		return ddlError("todas as colunas precisam de nome")
	}
	if utf8.RuneCountInString(name) > 64 || strings.HasSuffix(name, " ") || strings.ContainsRune(name, 0) {
		return ddlError("nome de coluna inválido: %q", name)
	}
	return nil
}

func checkOption(label, v string) error {
	if v != "" && !optionNameRe.MatchString(v) {
		return ddlError("%s inválido: %q", label, v)
	}
	return nil
}

// quoteString returns a single-quoted SQL string literal.
func quoteString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// CreateDatabaseSQL implements database.DDLBuilder.
func (d *Driver) CreateDatabaseSQL(spec database.CreateDatabaseSpec) (string, error) {
	if err := checkObjectName("database", spec.Name); err != nil {
		return "", err
	}
	if err := checkOption("charset", spec.Charset); err != nil {
		return "", err
	}
	if err := checkOption("collation", spec.Collation); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("CREATE DATABASE ")
	if spec.IfNotExists {
		b.WriteString("IF NOT EXISTS ")
	}
	b.WriteString(QuoteIdent(spec.Name))
	if spec.Charset != "" {
		b.WriteString(" CHARACTER SET " + spec.Charset)
	}
	if spec.Collation != "" {
		b.WriteString(" COLLATE " + spec.Collation)
	}
	return b.String(), nil
}

// columnTypeSQL renders the type with its arguments.
func columnTypeSQL(c database.ColumnSpec) (string, typeInfo, error) {
	t := strings.ToUpper(strings.TrimSpace(c.Type))
	info, ok := columnTypes[t]
	if !ok {
		return "", info, ddlError("tipo desconhecido na coluna %q: %q", c.Name, c.Type)
	}
	arg := strings.ReplaceAll(strings.TrimSpace(c.Length), " ", "")
	switch info.rule {
	case argNone:
		if arg != "" {
			return "", info, ddlError("o tipo %s da coluna %q não aceita tamanho", t, c.Name)
		}
	case argOptInt, argReqInt:
		if arg == "" && info.rule == argReqInt {
			return "", info, ddlError("informe o tamanho do %s da coluna %q", t, c.Name)
		}
		if arg != "" {
			if !intArgRe.MatchString(arg) {
				return "", info, ddlError("tamanho inválido para %s na coluna %q: %q", t, c.Name, c.Length)
			}
			if (t == "TIME" || t == "DATETIME" || t == "TIMESTAMP") && (len(arg) > 1 || arg[0] > '6') {
				return "", info, ddlError("a precisão de %s na coluna %q deve estar entre 0 e 6", t, c.Name)
			}
			t += "(" + arg + ")"
		}
	case argPrecision:
		if arg != "" {
			if !precisionRe.MatchString(arg) {
				return "", info, ddlError("precisão inválida para %s na coluna %q (use p ou p,s): %q", t, c.Name, c.Length)
			}
			t += "(" + arg + ")"
		}
	case argValues:
		values := c.Values
		if len(values) == 0 && strings.TrimSpace(c.Length) != "" {
			for _, v := range strings.Split(c.Length, ",") {
				values = append(values, strings.TrimSpace(v))
			}
		}
		if len(values) == 0 {
			return "", info, ddlError("informe os valores do %s da coluna %q", t, c.Name)
		}
		quoted := make([]string, len(values))
		for i, v := range values {
			quoted[i] = quoteString(v)
		}
		t += "(" + strings.Join(quoted, ", ") + ")"
	}
	if c.Unsigned {
		if !info.numeric {
			return "", info, ddlError("UNSIGNED só se aplica a tipos numéricos (coluna %q)", c.Name)
		}
		t += " UNSIGNED"
	}
	return t, info, nil
}

func columnSQL(c database.ColumnSpec) (string, error) {
	typ, info, err := columnTypeSQL(c)
	if err != nil {
		return "", err
	}
	parts := []string{QuoteIdent(c.Name), typ}
	if c.Nullable && !c.PrimaryKey {
		parts = append(parts, "NULL")
	} else {
		parts = append(parts, "NOT NULL")
	}
	switch c.DefaultKind {
	case database.DefaultNone:
	case database.DefaultNull:
		if !c.Nullable || c.PrimaryKey {
			return "", ddlError("a coluna %q é NOT NULL e não pode ter DEFAULT NULL", c.Name)
		}
		parts = append(parts, "DEFAULT NULL")
	case database.DefaultValue:
		if info.numeric && numberRe.MatchString(strings.TrimSpace(c.Default)) {
			parts = append(parts, "DEFAULT "+strings.TrimSpace(c.Default))
		} else {
			parts = append(parts, "DEFAULT "+quoteString(c.Default))
		}
	case database.DefaultCurrentTimestamp:
		if !info.temporal {
			return "", ddlError("CURRENT_TIMESTAMP só é permitido em DATETIME ou TIMESTAMP (coluna %q)", c.Name)
		}
		parts = append(parts, "DEFAULT CURRENT_TIMESTAMP"+fsp(c))
	case database.DefaultExpression:
		expr := strings.TrimSpace(c.Default)
		if expr == "" {
			return "", ddlError("informe a expressão do DEFAULT da coluna %q", c.Name)
		}
		parts = append(parts, "DEFAULT ("+expr+")")
	default:
		return "", ddlError("tipo de DEFAULT inválido na coluna %q", c.Name)
	}
	if c.OnUpdateCurrentTimestamp {
		if !info.temporal {
			return "", ddlError("ON UPDATE CURRENT_TIMESTAMP só é permitido em DATETIME ou TIMESTAMP (coluna %q)", c.Name)
		}
		parts = append(parts, "ON UPDATE CURRENT_TIMESTAMP"+fsp(c))
	}
	if c.AutoIncrement {
		if !info.integer {
			return "", ddlError("AUTO_INCREMENT exige um tipo inteiro (coluna %q)", c.Name)
		}
		if c.DefaultKind != database.DefaultNone {
			return "", ddlError("a coluna AUTO_INCREMENT %q não pode ter DEFAULT", c.Name)
		}
		parts = append(parts, "AUTO_INCREMENT")
	}
	if c.Unique && !c.PrimaryKey {
		parts = append(parts, "UNIQUE")
	}
	if c.Comment != "" {
		parts = append(parts, "COMMENT "+quoteString(c.Comment))
	}
	return strings.Join(parts, " "), nil
}

// fsp repeats the fractional seconds precision of the column in
// CURRENT_TIMESTAMP, which MySQL requires to match.
func fsp(c database.ColumnSpec) string {
	if a := strings.TrimSpace(c.Length); a != "" && a != "0" {
		return "(" + a + ")"
	}
	return ""
}

// CreateTableSQL implements database.DDLBuilder.
func (d *Driver) CreateTableSQL(spec database.CreateTableSpec) (string, error) {
	if err := checkObjectName("database", spec.Database); err != nil {
		return "", err
	}
	if err := checkObjectName("tabela", spec.Name); err != nil {
		return "", err
	}
	for label, v := range map[string]string{"engine": spec.Engine, "charset": spec.Charset, "collation": spec.Collation} {
		if err := checkOption(label, v); err != nil {
			return "", err
		}
	}
	if len(spec.Columns) == 0 {
		return "", ddlError("a tabela precisa de pelo menos uma coluna")
	}
	seen := map[string]bool{}
	var lines, pk []string
	autoInc := ""
	for _, c := range spec.Columns {
		if err := checkColumnName(c.Name); err != nil {
			return "", err
		}
		key := strings.ToLower(c.Name)
		if seen[key] {
			return "", ddlError("coluna duplicada: %q", c.Name)
		}
		seen[key] = true
		line, err := columnSQL(c)
		if err != nil {
			return "", err
		}
		lines = append(lines, line)
		if c.PrimaryKey {
			pk = append(pk, QuoteIdent(c.Name))
		}
		if c.AutoIncrement {
			if autoInc != "" {
				return "", ddlError("só pode haver uma coluna AUTO_INCREMENT")
			}
			autoInc = c.Name
		}
	}
	if len(pk) > 0 {
		lines = append(lines, "PRIMARY KEY ("+strings.Join(pk, ", ")+")")
	}
	indexed := map[string]bool{}
	for _, c := range spec.Columns {
		if c.PrimaryKey && strings.EqualFold(QuoteIdent(c.Name), pk[0]) || c.Unique {
			indexed[strings.ToLower(c.Name)] = true
		}
	}
	names := map[string]bool{}
	for i, ix := range spec.Indexes {
		if len(ix.Columns) == 0 {
			return "", ddlError("o índice %d não tem colunas", i+1)
		}
		cols := make([]string, len(ix.Columns))
		for j, c := range ix.Columns {
			if !seen[strings.ToLower(c)] {
				return "", ddlError("o índice %q usa a coluna inexistente %q", ix.Name, c)
			}
			cols[j] = QuoteIdent(c)
		}
		name := strings.TrimSpace(ix.Name)
		if name == "" {
			prefix := "idx"
			if ix.Unique {
				prefix = "uq"
			}
			name = prefix + "_" + strings.Join(ix.Columns, "_")
			if r := []rune(name); len(r) > 64 {
				name = string(r[:64])
			}
		}
		if err := checkColumnName(name); err != nil {
			return "", ddlError("nome de índice inválido: %q", ix.Name)
		}
		if names[strings.ToLower(name)] || strings.EqualFold(name, "PRIMARY") {
			return "", ddlError("nome de índice duplicado: %q", name)
		}
		names[strings.ToLower(name)] = true
		kind := "KEY"
		if ix.Unique {
			kind = "UNIQUE KEY"
		}
		lines = append(lines, fmt.Sprintf("%s %s (%s)", kind, QuoteIdent(name), strings.Join(cols, ", ")))
		indexed[strings.ToLower(ix.Columns[0])] = true
	}
	if autoInc != "" && !indexed[strings.ToLower(autoInc)] {
		return "", ddlError("a coluna AUTO_INCREMENT %q precisa ser a primeira coluna da chave primária ou de um índice", autoInc)
	}

	var b strings.Builder
	b.WriteString("CREATE TABLE ")
	if spec.IfNotExists {
		b.WriteString("IF NOT EXISTS ")
	}
	b.WriteString(QuoteIdent(spec.Database) + "." + QuoteIdent(spec.Name) + " (\n  ")
	b.WriteString(strings.Join(lines, ",\n  "))
	b.WriteString("\n)")
	if spec.Engine != "" {
		b.WriteString(" ENGINE=" + spec.Engine)
	}
	if spec.Charset != "" {
		b.WriteString(" DEFAULT CHARSET=" + spec.Charset)
	}
	if spec.Collation != "" {
		b.WriteString(" COLLATE=" + spec.Collation)
	}
	if spec.Comment != "" {
		b.WriteString(" COMMENT=" + quoteString(spec.Comment))
	}
	return b.String(), nil
}

// DDLOptions implements database.DDLBuilder.
func (d *Driver) DDLOptions(ctx context.Context, db *sql.DB) (database.DDLOptions, error) {
	out := database.DDLOptions{Types: typeOrder}
	byName := map[string]*database.CharsetInfo{}

	rows, err := db.QueryContext(ctx, `SELECT CHARACTER_SET_NAME, DEFAULT_COLLATE_NAME, COALESCE(DESCRIPTION, '') FROM information_schema.CHARACTER_SETS ORDER BY CHARACTER_SET_NAME`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var c database.CharsetInfo
		if err := rows.Scan(&c.Name, &c.DefaultCollation, &c.Description); err != nil {
			rows.Close()
			return out, err
		}
		out.Charsets = append(out.Charsets, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	for i := range out.Charsets {
		byName[out.Charsets[i].Name] = &out.Charsets[i]
	}

	// COLLATION_CHARACTER_SET_APPLICABILITY exists in MySQL and MariaDB; in
	// MariaDB 10.10+ it also lists the UCA 14 collations shared by charsets.
	rows, err = db.QueryContext(ctx, `SELECT COLLATION_NAME, CHARACTER_SET_NAME FROM information_schema.COLLATION_CHARACTER_SET_APPLICABILITY`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var coll, cs sql.NullString
		if err := rows.Scan(&coll, &cs); err != nil {
			rows.Close()
			return out, err
		}
		if c := byName[cs.String]; c != nil && coll.Valid {
			name := coll.String
			// MariaDB lists UCA 14 collations without the charset prefix.
			if !strings.HasPrefix(name, c.Name+"_") && strings.HasPrefix(name, "uca1400") {
				name = c.Name + "_" + name
			}
			c.Collations = append(c.Collations, name)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	for i := range out.Charsets {
		c := &out.Charsets[i]
		if c.DefaultCollation != "" && !contains(c.Collations, c.DefaultCollation) {
			c.Collations = append(c.Collations, c.DefaultCollation)
		}
		sort.Strings(c.Collations)
	}

	rows, err = db.QueryContext(ctx, `SELECT ENGINE, SUPPORT FROM information_schema.ENGINES WHERE SUPPORT IN ('YES', 'DEFAULT') ORDER BY ENGINE`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var engine, support string
		if err := rows.Scan(&engine, &support); err != nil {
			rows.Close()
			return out, err
		}
		out.Engines = append(out.Engines, engine)
		if support == "DEFAULT" {
			out.DefaultEngine = engine
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	err = db.QueryRowContext(ctx, `SELECT @@character_set_server, @@collation_server`).Scan(&out.DefaultCharset, &out.DefaultCollation)
	return out, err
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

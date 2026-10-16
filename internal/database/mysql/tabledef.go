package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"anansi/internal/database"
	"anansi/internal/sqllex"
)

var _ database.ObjectEditor = (*Driver)(nil)

// editorTypes extends the CREATE TABLE dialog types with every type the
// table editor accepts.
var editorTypes = func() map[string]typeInfo {
	m := map[string]typeInfo{}
	for k, v := range columnTypes {
		m[k] = v
	}
	for _, t := range []string{"MULTIPOINT", "MULTILINESTRING", "MULTIPOLYGON", "GEOMETRYCOLLECTION", "GEOMCOLLECTION"} {
		m[t] = typeInfo{rule: argNone}
	}
	m["INTEGER"] = typeInfo{argOptInt, true, true, false}
	m["REAL"] = typeInfo{argPrecision, true, false, false}
	m["BOOL"] = typeInfo{argNone, false, false, false}
	m["VECTOR"] = typeInfo{argOptInt, false, false, false}
	return m
}()

var (
	textTypes    = map[string]bool{"CHAR": true, "VARCHAR": true, "TINYTEXT": true, "TEXT": true, "MEDIUMTEXT": true, "LONGTEXT": true, "ENUM": true, "SET": true}
	spatialTypes = map[string]bool{"GEOMETRY": true, "POINT": true, "LINESTRING": true, "POLYGON": true, "MULTIPOINT": true, "MULTILINESTRING": true, "MULTIPOLYGON": true, "GEOMETRYCOLLECTION": true, "GEOMCOLLECTION": true}
	fkActions    = map[string]bool{"": true, "RESTRICT": true, "CASCADE": true, "SET NULL": true, "NO ACTION": true, "SET DEFAULT": true}
	rowFormats   = map[string]bool{"": true, "DEFAULT": true, "DYNAMIC": true, "FIXED": true, "COMPRESSED": true, "REDUNDANT": true, "COMPACT": true, "PAGE": true}
	plainNumber  = regexp.MustCompile(`^\d{1,20}$`)
	timestampFn  = regexp.MustCompile(`(?i)^(CURRENT_TIMESTAMP|NOW|LOCALTIME|LOCALTIMESTAMP|CURRENT_DATE|CURDATE|CURRENT_TIME|CURTIME|UTC_TIMESTAMP)(\(\d?\))?$`)
)

// checkExpr validates SQL typed by the user that is embedded in DDL: it must
// be non-empty, have balanced parentheses, closed strings/identifiers and no
// statement separator outside strings.
func checkExpr(label, s string) error {
	if strings.TrimSpace(s) == "" {
		return ddlError("informe %s", label)
	}
	depth := 0
	for _, t := range sqllex.Tokenize(s) {
		if t.Open {
			return ddlError("%s: aspas, identificador ou comentário não fechado", label)
		}
		if t.Kind == sqllex.Punct {
			switch t.Text {
			case "(":
				depth++
			case ")":
				depth--
				if depth < 0 {
					return ddlError("%s: parênteses desbalanceados", label)
				}
			case ";":
				return ddlError("%s não pode conter ';'", label)
			}
		}
	}
	if depth != 0 {
		return ddlError("%s: parênteses desbalanceados", label)
	}
	return nil
}

// ---- column SQL ------------------------------------------------------------

// typeArgs validates and renders the type arguments of a column.
func typeArgs(c database.ColumnDef, t string, info typeInfo) (string, error) {
	arg := strings.TrimSpace(c.Length)
	switch info.rule {
	case argNone:
		if arg != "" {
			return "", ddlError("o tipo %s da coluna %q não aceita tamanho", t, c.Name)
		}
		return "", nil
	case argOptInt, argReqInt:
		if arg == "" {
			if info.rule == argReqInt {
				return "", ddlError("informe o tamanho do %s da coluna %q", t, c.Name)
			}
			return "", nil
		}
		if !intArgRe.MatchString(arg) {
			return "", ddlError("tamanho inválido para %s na coluna %q: %q", t, c.Name, c.Length)
		}
		return "(" + arg + ")", nil
	case argPrecision:
		a := strings.ReplaceAll(arg, " ", "")
		if a == "" {
			return "", nil
		}
		if !precisionRe.MatchString(a) {
			return "", ddlError("precisão inválida para %s na coluna %q (use p ou p,s): %q", t, c.Name, c.Length)
		}
		return "(" + a + ")", nil
	case argValues:
		vals, err := parseEnumArgs(arg)
		if err != nil || len(vals) == 0 {
			return "", ddlError("informe os valores do %s da coluna %q entre aspas simples, separados por vírgula", t, c.Name)
		}
		q := make([]string, len(vals))
		for i, v := range vals {
			q[i] = quoteString(v)
		}
		return "(" + strings.Join(q, ",") + ")", nil
	}
	return "", nil
}

// parseEnumArgs splits "'a','b”c'" into values; unquoted lists ("a, b")
// are accepted too.
func parseEnumArgs(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if !strings.HasPrefix(s, "'") {
		var out []string
		for _, p := range strings.Split(s, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out, nil
	}
	var out []string
	i := 0
	for i < len(s) {
		for i < len(s) && (s[i] == ' ' || s[i] == ',') {
			i++
		}
		if i >= len(s) {
			break
		}
		if s[i] != '\'' {
			return nil, fmt.Errorf("valor sem aspas")
		}
		i++
		var b strings.Builder
		closed := false
		for i < len(s) {
			ch := s[i]
			if ch == '\\' && i+1 < len(s) {
				b.WriteByte(s[i+1])
				i += 2
				continue
			}
			if ch == '\'' {
				if i+1 < len(s) && s[i+1] == '\'' {
					b.WriteByte('\'')
					i += 2
					continue
				}
				i++
				closed = true
				break
			}
			b.WriteByte(ch)
			i++
		}
		if !closed {
			return nil, fmt.Errorf("aspas não fechadas")
		}
		out = append(out, b.String())
	}
	return out, nil
}

func defaultSQL(c database.ColumnDef, info typeInfo) (string, error) {
	switch c.DefaultKind {
	case "":
		return "", nil
	case "null":
		if !c.Nullable {
			return "", ddlError("a coluna %q é NOT NULL e não pode ter DEFAULT NULL", c.Name)
		}
		return "DEFAULT NULL", nil
	case "value":
		v := c.Default
		if info.numeric && numberRe.MatchString(strings.TrimSpace(v)) {
			return "DEFAULT " + strings.TrimSpace(v), nil
		}
		if t := strings.ToUpper(strings.TrimSpace(c.Type)); t == "BIT" && (plainNumber.MatchString(strings.TrimSpace(v)) || strings.HasPrefix(strings.ToLower(strings.TrimSpace(v)), "b'")) {
			return "DEFAULT " + strings.TrimSpace(v), nil
		}
		return "DEFAULT " + quoteString(v), nil
	case "expression":
		e := strings.TrimSpace(c.Default)
		if err := checkExpr(fmt.Sprintf("a expressão do DEFAULT da coluna %q", c.Name), e); err != nil {
			return "", err
		}
		if timestampFn.MatchString(e) || (strings.HasPrefix(e, "(") && strings.HasSuffix(e, ")")) {
			return "DEFAULT " + e, nil
		}
		return "DEFAULT (" + e + ")", nil
	}
	return "", ddlError("tipo de DEFAULT inválido na coluna %q", c.Name)
}

// ColumnDefSQL renders a column definition (without position).
func ColumnDefSQL(c database.ColumnDef, mariadb bool) (string, error) {
	if err := checkColumnName(c.Name); err != nil {
		return "", err
	}
	t := strings.ToUpper(strings.TrimSpace(c.Type))
	info, ok := editorTypes[t]
	if !ok {
		return "", ddlError("tipo desconhecido na coluna %q: %q", c.Name, c.Type)
	}
	args, err := typeArgs(c, t, info)
	if err != nil {
		return "", err
	}
	parts := []string{QuoteIdent(c.Name), t + args}
	if c.Unsigned {
		if !info.numeric {
			return "", ddlError("UNSIGNED só se aplica a tipos numéricos (coluna %q)", c.Name)
		}
		parts = append(parts, "UNSIGNED")
	}
	if c.Zerofill {
		if !info.numeric {
			return "", ddlError("ZEROFILL só se aplica a tipos numéricos (coluna %q)", c.Name)
		}
		parts = append(parts, "ZEROFILL")
	}
	generated := strings.TrimSpace(c.Generated) != ""
	if c.Collation != "" && textTypes[t] && !generated {
		if err := checkOption("collation", c.Collation); err != nil {
			return "", err
		}
		parts = append(parts, "COLLATE "+c.Collation)
	}
	if generated {
		if err := checkExpr(fmt.Sprintf("a expressão da coluna gerada %q", c.Name), c.Generated); err != nil {
			return "", err
		}
		kind := strings.ToUpper(strings.TrimSpace(c.GeneratedKind))
		switch kind {
		case "", "VIRTUAL":
			kind = "VIRTUAL"
		case "STORED", "PERSISTENT":
			if mariadb {
				kind = "PERSISTENT"
			} else {
				kind = "STORED"
			}
		default:
			return "", ddlError("tipo de coluna gerada inválido na coluna %q", c.Name)
		}
		g := strings.TrimSpace(c.Generated)
		if !(strings.HasPrefix(g, "(") && strings.HasSuffix(g, ")")) {
			g = "(" + g + ")"
		}
		parts = append(parts, "AS "+g+" "+kind)
		if !mariadb {
			if c.Nullable {
				parts = append(parts, "NULL")
			} else {
				parts = append(parts, "NOT NULL")
			}
		}
	} else {
		if c.Nullable {
			parts = append(parts, "NULL")
		} else {
			parts = append(parts, "NOT NULL")
		}
		if c.SRID != "" {
			if !spatialTypes[t] || !plainNumber.MatchString(c.SRID) {
				return "", ddlError("SRID inválido na coluna %q", c.Name)
			}
			parts = append(parts, "SRID "+c.SRID)
		}
		if c.AutoIncrement {
			if !info.integer {
				return "", ddlError("AUTO_INCREMENT exige um tipo inteiro (coluna %q)", c.Name)
			}
			if c.DefaultKind != "" {
				return "", ddlError("a coluna AUTO_INCREMENT %q não pode ter DEFAULT", c.Name)
			}
			parts = append(parts, "AUTO_INCREMENT")
		} else {
			d, err := defaultSQL(c, info)
			if err != nil {
				return "", err
			}
			if d != "" {
				parts = append(parts, d)
			}
		}
		if u := strings.TrimSpace(c.OnUpdate); u != "" {
			if err := checkExpr(fmt.Sprintf("o ON UPDATE da coluna %q", c.Name), u); err != nil {
				return "", err
			}
			parts = append(parts, "ON UPDATE "+u)
		}
	}
	if c.Invisible {
		parts = append(parts, "INVISIBLE")
	}
	if c.Comment != "" {
		parts = append(parts, "COMMENT "+quoteString(c.Comment))
	}
	return strings.Join(parts, " "), nil
}

// ---- index, FK, check SQL ----------------------------------------------------

func indexParts(ix database.IndexDef) (string, error) {
	if len(ix.Parts) == 0 {
		return "", ddlError("o índice %q não tem colunas", ix.Name)
	}
	out := make([]string, len(ix.Parts))
	for i, p := range ix.Parts {
		var s string
		if e := strings.TrimSpace(p.Expression); e != "" {
			if err := checkExpr(fmt.Sprintf("a expressão do índice %q", ix.Name), e); err != nil {
				return "", err
			}
			if !(strings.HasPrefix(e, "(") && strings.HasSuffix(e, ")")) {
				e = "(" + e + ")"
			}
			s = e
		} else {
			if strings.TrimSpace(p.Column) == "" {
				return "", ddlError("o índice %q tem uma parte sem coluna", ix.Name)
			}
			s = QuoteIdent(p.Column)
			if p.Length > 0 {
				s += "(" + strconv.Itoa(p.Length) + ")"
			}
		}
		if p.Desc {
			s += " DESC"
		}
		out[i] = s
	}
	return "(" + strings.Join(out, ", ") + ")", nil
}

func indexName(ix database.IndexDef) string {
	if n := strings.TrimSpace(ix.Name); n != "" {
		return n
	}
	var cols []string
	for _, p := range ix.Parts {
		if p.Column != "" {
			cols = append(cols, p.Column)
		}
	}
	prefix := "idx"
	if strings.EqualFold(ix.Kind, "UNIQUE") {
		prefix = "uq"
	}
	n := prefix + "_" + strings.Join(cols, "_")
	if r := []rune(n); len(r) > 64 {
		n = string(r[:64])
	}
	return n
}

// IndexDefSQL renders an index for CREATE TABLE / ALTER TABLE ADD.
func IndexDefSQL(ix database.IndexDef, mariadb bool) (string, error) {
	parts, err := indexParts(ix)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	kind := strings.ToUpper(strings.TrimSpace(ix.Kind))
	switch kind {
	case "PRIMARY":
		b.WriteString("PRIMARY KEY " + parts)
	case "KEY", "INDEX", "":
		b.WriteString("INDEX " + QuoteIdent(indexName(ix)) + " " + parts)
	case "UNIQUE", "FULLTEXT", "SPATIAL":
		b.WriteString(kind + " INDEX " + QuoteIdent(indexName(ix)) + " " + parts)
	default:
		return "", ddlError("tipo de índice inválido: %q", ix.Kind)
	}
	switch a := strings.ToUpper(ix.Algorithm); a {
	case "":
	case "BTREE", "HASH", "RTREE":
		b.WriteString(" USING " + a)
	default:
		return "", ddlError("algoritmo de índice inválido: %q", ix.Algorithm)
	}
	if ix.Comment != "" {
		b.WriteString(" COMMENT " + quoteString(ix.Comment))
	}
	if ix.Invisible && kind != "PRIMARY" {
		if mariadb {
			b.WriteString(" IGNORED")
		} else {
			b.WriteString(" INVISIBLE")
		}
	}
	return b.String(), nil
}

// ForeignKeyDefSQL renders CONSTRAINT ... FOREIGN KEY.
func ForeignKeyDefSQL(fk database.ForeignKeyDef, table string) (string, error) {
	if len(fk.Columns) == 0 || len(fk.Columns) != len(fk.RefColumns) {
		return "", ddlError("a chave estrangeira %q precisa do mesmo número de colunas locais e referenciadas", fk.Name)
	}
	if strings.TrimSpace(fk.RefTable) == "" {
		return "", ddlError("informe a tabela referenciada pela chave estrangeira %q", fk.Name)
	}
	q := func(cols []string) string {
		out := make([]string, len(cols))
		for i, c := range cols {
			out[i] = QuoteIdent(c)
		}
		return strings.Join(out, ", ")
	}
	name := strings.TrimSpace(fk.Name)
	if name == "" {
		name = "fk_" + table + "_" + fk.RefTable
		if r := []rune(name); len(r) > 64 {
			name = string(r[:64])
		}
	}
	ref := QuoteIdent(fk.RefTable)
	if fk.RefDatabase != "" {
		ref = QuoteIdent(fk.RefDatabase) + "." + ref
	}
	s := fmt.Sprintf("CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)", QuoteIdent(name), q(fk.Columns), ref, q(fk.RefColumns))
	for _, a := range []struct{ label, v string }{{"ON UPDATE", fk.OnUpdate}, {"ON DELETE", fk.OnDelete}} {
		v := strings.ToUpper(strings.TrimSpace(a.v))
		if !fkActions[v] {
			return "", ddlError("ação inválida em %s: %q", a.label, a.v)
		}
		if v != "" {
			s += " " + a.label + " " + v
		}
	}
	return s, nil
}

// CheckDefSQL renders CONSTRAINT ... CHECK.
func CheckDefSQL(c database.CheckDef) (string, error) {
	if err := checkExpr(fmt.Sprintf("a expressão da restrição %q", c.Name), c.Clause); err != nil {
		return "", err
	}
	clause := strings.TrimSpace(c.Clause)
	if !(strings.HasPrefix(clause, "(") && strings.HasSuffix(clause, ")")) {
		clause = "(" + clause + ")"
	}
	if strings.TrimSpace(c.Name) == "" {
		return "CHECK " + clause, nil
	}
	return "CONSTRAINT " + QuoteIdent(c.Name) + " CHECK " + clause, nil
}

// ---- table options -------------------------------------------------------------

func optionsSQL(o database.TableOptions, cur *database.TableOptions) ([]string, error) {
	var out []string
	var c database.TableOptions
	if cur != nil {
		c = *cur
	}
	changed := func(a, b string) bool { return cur == nil || a != b }
	if o.Engine != "" && changed(o.Engine, c.Engine) {
		if err := checkOption("engine", o.Engine); err != nil {
			return nil, err
		}
		out = append(out, "ENGINE="+o.Engine)
	}
	if o.Collation != "" && changed(o.Collation, c.Collation) {
		if err := checkOption("collation", o.Collation); err != nil {
			return nil, err
		}
		cs := o.Collation
		if i := strings.IndexByte(cs, '_'); i > 0 {
			cs = cs[:i]
		}
		if cur != nil && o.ConvertData {
			out = append(out, "CONVERT TO CHARACTER SET "+cs+" COLLATE "+o.Collation)
		} else {
			out = append(out, "DEFAULT CHARACTER SET "+cs+" COLLATE "+o.Collation)
		}
	}
	if cur == nil && o.Comment != "" || cur != nil && o.Comment != c.Comment {
		out = append(out, "COMMENT="+quoteString(o.Comment))
	}
	if rf := strings.ToUpper(o.RowFormat); (cur == nil && rf != "") || (cur != nil && rf != strings.ToUpper(c.RowFormat)) {
		if !rowFormats[rf] {
			return nil, ddlError("formato de linha inválido: %q", o.RowFormat)
		}
		if rf == "" {
			rf = "DEFAULT"
		}
		out = append(out, "ROW_FORMAT="+rf)
	}
	for _, n := range []struct{ name, v, cur string }{
		{"AUTO_INCREMENT", o.AutoIncrement, c.AutoIncrement},
		{"AVG_ROW_LENGTH", o.AvgRowLength, c.AvgRowLength},
		{"MAX_ROWS", o.MaxRows, c.MaxRows},
	} {
		v := strings.TrimSpace(n.v)
		if v == "" || (cur != nil && v == n.cur) {
			continue
		}
		if !plainNumber.MatchString(v) {
			return nil, ddlError("%s deve ser um número inteiro", n.name)
		}
		out = append(out, n.name+"="+v)
	}
	if cur == nil && o.Checksum || cur != nil && o.Checksum != c.Checksum {
		if o.Checksum {
			out = append(out, "CHECKSUM=1")
		} else {
			out = append(out, "CHECKSUM=0")
		}
	}
	return out, nil
}

func checkPartition(p string) error {
	p = strings.TrimSpace(p)
	if p == "" {
		return nil
	}
	if !strings.HasPrefix(strings.ToUpper(p), "PARTITION BY") {
		return ddlError("o particionamento deve começar com PARTITION BY")
	}
	return checkExpr("o particionamento", p)
}

func validateDef(def database.TableDef) error {
	if err := checkObjectName("database", def.Database); err != nil {
		return err
	}
	if err := checkObjectName("tabela", def.Name); err != nil {
		return err
	}
	if len(def.Columns) == 0 {
		return ddlError("a tabela precisa de pelo menos uma coluna")
	}
	seen := map[string]bool{}
	for _, c := range def.Columns {
		k := strings.ToLower(strings.TrimSpace(c.Name))
		if seen[k] {
			return ddlError("coluna duplicada: %q", c.Name)
		}
		seen[k] = true
	}
	names := map[string]bool{}
	pk := 0
	for _, ix := range def.Indexes {
		if strings.EqualFold(ix.Kind, "PRIMARY") {
			pk++
			for _, p := range ix.Parts {
				for _, c := range def.Columns {
					if strings.EqualFold(c.Name, p.Column) && c.Nullable {
						return ddlError("a coluna %q faz parte da chave primária e não pode aceitar NULL", c.Name)
					}
				}
			}
			continue
		}
		n := strings.ToLower(indexName(ix))
		if names[n] {
			return ddlError("nome de índice duplicado: %q", indexName(ix))
		}
		names[n] = true
		for _, p := range ix.Parts {
			if p.Expression == "" && !seen[strings.ToLower(p.Column)] {
				return ddlError("o índice %q usa a coluna inexistente %q", indexName(ix), p.Column)
			}
		}
	}
	if pk > 1 {
		return ddlError("só pode haver uma chave primária")
	}
	for _, fk := range def.ForeignKeys {
		for _, c := range fk.Columns {
			if !seen[strings.ToLower(c)] {
				return ddlError("a chave estrangeira %q usa a coluna inexistente %q", fk.Name, c)
			}
		}
	}
	return checkPartition(def.Options.Partition)
}

// CreateTableDefSQL implements database.ObjectEditor.
func (d *Driver) CreateTableDefSQL(def database.TableDef) (string, error) {
	if err := validateDef(def); err != nil {
		return "", err
	}
	maria := def.MariaDB
	var lines []string
	for _, c := range def.Columns {
		s, err := ColumnDefSQL(c, maria)
		if err != nil {
			return "", err
		}
		lines = append(lines, s)
	}
	for _, ix := range def.Indexes {
		s, err := IndexDefSQL(ix, maria)
		if err != nil {
			return "", err
		}
		lines = append(lines, s)
	}
	for _, fk := range def.ForeignKeys {
		s, err := ForeignKeyDefSQL(fk, def.Name)
		if err != nil {
			return "", err
		}
		lines = append(lines, s)
	}
	for _, c := range def.Checks {
		s, err := CheckDefSQL(c)
		if err != nil {
			return "", err
		}
		lines = append(lines, s)
	}
	opts, err := optionsSQL(def.Options, nil)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("CREATE TABLE " + QuoteIdent(def.Database) + "." + QuoteIdent(def.Name) + " (\n  ")
	b.WriteString(strings.Join(lines, ",\n  "))
	b.WriteString("\n)")
	if len(opts) > 0 {
		b.WriteString("\n" + strings.Join(opts, "\n"))
	}
	if p := strings.TrimSpace(def.Options.Partition); p != "" {
		b.WriteString("\n" + p)
	}
	return b.String(), nil
}

// ---- ALTER TABLE diff -----------------------------------------------------------

func sameColumn(a, b database.ColumnDef) bool {
	a.OrigName, b.OrigName = "", ""
	a.Type, b.Type = strings.ToUpper(strings.TrimSpace(a.Type)), strings.ToUpper(strings.TrimSpace(b.Type))
	a.GeneratedKind, b.GeneratedKind = normGen(a.GeneratedKind), normGen(b.GeneratedKind)
	a.Length, b.Length = normLength(a.Length), normLength(b.Length)
	return a == b
}

func normGen(k string) string {
	k = strings.ToUpper(strings.TrimSpace(k))
	if k == "PERSISTENT" {
		return "STORED"
	}
	return k
}

func normLength(l string) string {
	if vals, err := parseEnumArgs(l); err == nil && strings.HasPrefix(strings.TrimSpace(l), "'") {
		return strings.Join(vals, "\x00")
	}
	return strings.ReplaceAll(strings.TrimSpace(l), " ", "")
}

func sameIndex(a, b database.IndexDef) bool {
	if !strings.EqualFold(a.Kind, b.Kind) || !strings.EqualFold(a.Algorithm, b.Algorithm) || a.Comment != b.Comment || len(a.Parts) != len(b.Parts) || a.Name != b.Name {
		return false
	}
	for i := range a.Parts {
		pa, pb := a.Parts[i], b.Parts[i]
		if !strings.EqualFold(pa.Column, pb.Column) || pa.Length != pb.Length || pa.Desc != pb.Desc || strings.TrimSpace(pa.Expression) != strings.TrimSpace(pb.Expression) {
			return false
		}
	}
	return true
}

func sameFK(a, b database.ForeignKeyDef) bool {
	eq := func(x, y []string) bool {
		if len(x) != len(y) {
			return false
		}
		for i := range x {
			if !strings.EqualFold(x[i], y[i]) {
				return false
			}
		}
		return true
	}
	norm := func(s string) string {
		s = strings.ToUpper(strings.TrimSpace(s))
		if s == "" {
			return "NO ACTION"
		}
		return s
	}
	return a.Name == b.Name && eq(a.Columns, b.Columns) && eq(a.RefColumns, b.RefColumns) && strings.EqualFold(a.RefTable, b.RefTable) &&
		(a.RefDatabase == b.RefDatabase || a.RefDatabase == "" || b.RefDatabase == "") && norm(a.OnUpdate) == norm(b.OnUpdate) && norm(a.OnDelete) == norm(b.OnDelete)
}

// AlterTableSQL implements database.ObjectEditor. It follows HeidiSQL's
// order: foreign keys being changed are dropped in a statement of their
// own, then one ALTER TABLE carries every other change, then the table is
// renamed.
func (d *Driver) AlterTableSQL(cur, def database.TableDef) ([]string, error) {
	if err := validateDef(def); err != nil {
		return nil, err
	}
	maria := def.MariaDB || cur.MariaDB
	table := QuoteIdent(cur.Database) + "." + QuoteIdent(cur.Name)
	var pre, clauses, post []string

	// Columns.
	curByName := map[string]database.ColumnDef{}
	for _, c := range cur.Columns {
		curByName[strings.ToLower(c.Name)] = c
	}
	kept := map[string]bool{}
	for _, c := range def.Columns {
		if c.OrigName != "" {
			if _, ok := curByName[strings.ToLower(c.OrigName)]; !ok {
				return nil, ddlError("a coluna %q não existe mais na tabela; recarregue o editor", c.OrigName)
			}
			kept[strings.ToLower(c.OrigName)] = true
		}
	}
	// Predecessor of each surviving column in the current table.
	curPred := map[string]string{}
	prev := ""
	for _, c := range cur.Columns {
		k := strings.ToLower(c.Name)
		if !kept[k] {
			continue
		}
		curPred[k] = prev
		prev = k
	}
	newPrevOrig := "" // last surviving column (by original name), ignoring added ones
	newPrev := ""     // last column in the final layout (new name)
	for _, c := range def.Columns {
		colSQL, err := ColumnDefSQL(c, maria)
		if err != nil {
			return nil, err
		}
		pos := " FIRST"
		if newPrev != "" {
			pos = " AFTER " + QuoteIdent(newPrev)
		}
		if c.OrigName == "" {
			clauses = append(clauses, "ADD COLUMN "+colSQL+pos)
		} else {
			k := strings.ToLower(c.OrigName)
			moved := curPred[k] != newPrevOrig
			if !sameColumn(curByName[k], c) || moved {
				s := "CHANGE COLUMN " + QuoteIdent(curByName[k].Name) + " " + colSQL
				if moved {
					s += pos
				}
				clauses = append(clauses, s)
			}
			newPrevOrig = k
		}
		newPrev = c.Name
	}
	var drops []string
	for _, c := range cur.Columns {
		if !kept[strings.ToLower(c.Name)] {
			drops = append(drops, "DROP COLUMN "+QuoteIdent(c.Name))
		}
	}

	// Indexes.
	curIdx := map[string]database.IndexDef{}
	for _, ix := range cur.Indexes {
		curIdx[ix.Name] = ix
	}
	var idxDrops, idxAdds []string
	keptIdx := map[string]bool{}
	for _, ix := range def.Indexes {
		if strings.EqualFold(ix.Kind, "PRIMARY") {
			ix.Name = "PRIMARY"
		}
		s, err := IndexDefSQL(ix, maria)
		if err != nil {
			return nil, err
		}
		orig := ix.OrigName
		if strings.EqualFold(ix.Kind, "PRIMARY") && orig == "" {
			if _, ok := curIdx["PRIMARY"]; ok {
				orig = "PRIMARY"
			}
		}
		if orig != "" {
			c, ok := curIdx[orig]
			if !ok {
				return nil, ddlError("o índice %q não existe mais; recarregue o editor", orig)
			}
			keptIdx[orig] = true
			named := ix
			if named.Name == "" {
				named.Name = indexName(ix)
			}
			if sameIndex(c, named) {
				if c.Invisible != ix.Invisible {
					vis := map[bool]string{true: "INVISIBLE", false: "VISIBLE"}
					if maria {
						vis = map[bool]string{true: "IGNORED", false: "NOT IGNORED"}
					}
					post = append(post, "ALTER TABLE "+table+" ALTER INDEX "+QuoteIdent(c.Name)+" "+vis[ix.Invisible])
				}
				continue
			}
			idxDrops = append(idxDrops, dropIndex(c))
		}
		idxAdds = append(idxAdds, "ADD "+s)
	}
	for _, ix := range cur.Indexes {
		if !keptIdx[ix.Name] {
			idxDrops = append(idxDrops, dropIndex(ix))
		}
	}

	// Foreign keys.
	curFK := map[string]database.ForeignKeyDef{}
	for _, fk := range cur.ForeignKeys {
		curFK[fk.Name] = fk
	}
	keptFK := map[string]bool{}
	var fkAdds []string
	for _, fk := range def.ForeignKeys {
		s, err := ForeignKeyDefSQL(fk, def.Name)
		if err != nil {
			return nil, err
		}
		if fk.OrigName != "" {
			c, ok := curFK[fk.OrigName]
			if !ok {
				return nil, ddlError("a chave estrangeira %q não existe mais; recarregue o editor", fk.OrigName)
			}
			keptFK[fk.OrigName] = true
			if sameFK(c, fk) {
				continue
			}
			pre = append(pre, "ALTER TABLE "+table+" DROP FOREIGN KEY "+QuoteIdent(c.Name))
		}
		fkAdds = append(fkAdds, "ADD "+s)
	}
	for _, fk := range cur.ForeignKeys {
		if !keptFK[fk.Name] {
			pre = append(pre, "ALTER TABLE "+table+" DROP FOREIGN KEY "+QuoteIdent(fk.Name))
		}
	}

	// Checks.
	dropCheck := "DROP CHECK "
	if maria {
		dropCheck = "DROP CONSTRAINT "
	}
	curChk := map[string]database.CheckDef{}
	for _, c := range cur.Checks {
		curChk[c.Name] = c
	}
	keptChk := map[string]bool{}
	var chkDrops, chkAdds []string
	for _, c := range def.Checks {
		s, err := CheckDefSQL(c)
		if err != nil {
			return nil, err
		}
		if c.OrigName != "" {
			o, ok := curChk[c.OrigName]
			if !ok {
				return nil, ddlError("a restrição %q não existe mais; recarregue o editor", c.OrigName)
			}
			keptChk[c.OrigName] = true
			if o.Name == c.Name && strings.TrimSpace(o.Clause) == strings.TrimSpace(c.Clause) {
				continue
			}
			chkDrops = append(chkDrops, dropCheck+QuoteIdent(o.Name))
		}
		chkAdds = append(chkAdds, "ADD "+s)
	}
	for _, c := range cur.Checks {
		if !keptChk[c.Name] {
			chkDrops = append(chkDrops, dropCheck+QuoteIdent(c.Name))
		}
	}

	opts, err := optionsSQL(def.Options, &cur.Options)
	if err != nil {
		return nil, err
	}

	all := append(append(append(append(append(append(append(idxDrops, chkDrops...), clauses...), drops...), idxAdds...), fkAdds...), chkAdds...), opts...)
	var out []string
	out = append(out, pre...)
	partChanged := strings.TrimSpace(def.Options.Partition) != strings.TrimSpace(cur.Options.Partition)
	if len(all) > 0 || partChanged {
		s := "ALTER TABLE " + table
		if len(all) > 0 {
			s += "\n  " + strings.Join(all, ",\n  ")
		}
		if partChanged {
			p := strings.TrimSpace(def.Options.Partition)
			if p == "" {
				p = "REMOVE PARTITIONING"
			}
			s += "\n  " + p
		}
		out = append(out, s)
	}
	out = append(out, post...)
	if def.Name != cur.Name {
		out = append(out, "RENAME TABLE "+table+" TO "+QuoteIdent(cur.Database)+"."+QuoteIdent(def.Name))
	}
	return out, nil
}

func dropIndex(ix database.IndexDef) string {
	if ix.Name == "PRIMARY" || strings.EqualFold(ix.Kind, "PRIMARY") {
		return "DROP PRIMARY KEY"
	}
	return "DROP INDEX " + QuoteIdent(ix.Name)
}

// ---- loading ----------------------------------------------------------------------

var (
	onUpdateRe  = regexp.MustCompile(`(?i)on update (\w+(?:\(\d*\))?)`)
	generatedRe = regexp.MustCompile(`(?i)\b(VIRTUAL|STORED|PERSISTENT) GENERATED\b`)
	optionRe    = regexp.MustCompile(`(?i)\b(row_format|checksum|max_rows|avg_row_length)=(\S+)`)
	partitionRe = regexp.MustCompile(`(?is)\n\s*(?:/\*!\d+\s*)?(PARTITION BY .*?)(?:\s*\*/)?\s*$`)
)

// splitColumnType parses COLUMN_TYPE ("int(10) unsigned zerofill",
// "enum('a','b')") into base type, arguments and flags.
func splitColumnType(ct string) (typ, args string, unsigned, zerofill bool) {
	s := strings.TrimSpace(ct)
	i := strings.IndexByte(s, '(')
	if i < 0 {
		f := strings.Fields(s)
		if len(f) == 0 {
			return "", "", false, false
		}
		typ = strings.ToUpper(f[0])
		rest := strings.ToLower(strings.Join(f[1:], " "))
		return typ, "", strings.Contains(rest, "unsigned"), strings.Contains(rest, "zerofill")
	}
	typ = strings.ToUpper(strings.TrimSpace(s[:i]))
	var j int
	if typ == "ENUM" || typ == "SET" {
		j = strings.LastIndexByte(s, ')')
	} else {
		j = strings.IndexByte(s[i:], ')') + i
	}
	if j <= i {
		return typ, "", false, false
	}
	args = s[i+1 : j]
	rest := strings.ToLower(s[j+1:])
	return typ, args, strings.Contains(rest, "unsigned"), strings.Contains(rest, "zerofill")
}

// mariaDefault decodes MariaDB's COLUMN_DEFAULT: quoted literals, NULL, or
// an expression.
func mariaDefault(v string) (kind, val string) {
	switch {
	case v == "NULL":
		return "null", ""
	case strings.HasPrefix(v, "'") && strings.HasSuffix(v, "'") && len(v) >= 2:
		vals, err := parseEnumArgs(v)
		if err == nil && len(vals) == 1 {
			return "value", vals[0]
		}
		return "value", strings.ReplaceAll(v[1:len(v)-1], "''", "'")
	case numberRe.MatchString(v):
		return "value", v
	}
	return "expression", v
}

// LoadTableDef implements database.ObjectEditor.
func (d *Driver) LoadTableDef(ctx context.Context, db *sql.DB, database_, table string) (database.TableDef, error) {
	def := database.TableDef{Database: database_, Name: table, OrigName: table}
	in := newInspector(db)
	maria, known := in.mariadb()
	if !known {
		var v string
		if err := db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&v); err == nil {
			maria = strings.Contains(strings.ToLower(v), "mariadb")
		}
	}
	def.MariaDB = maria

	srid := ""
	if !maria {
		srid = ", SRS_ID"
	}
	q := `SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_DEFAULT, EXTRA, COLUMN_COMMENT, COLLATION_NAME, GENERATION_EXPRESSION` + srid + `
FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? ORDER BY ORDINAL_POSITION`
	rows, err := db.QueryContext(ctx, q, database_, table)
	if err != nil {
		return def, err
	}
	for rows.Next() {
		var name, ctype, nullable, extra string
		var dflt, comment, coll, gen, srs sql.NullString
		dest := []any{&name, &ctype, &nullable, &dflt, &extra, &comment, &coll, &gen}
		if !maria {
			dest = append(dest, &srs)
		}
		if err := rows.Scan(dest...); err != nil {
			rows.Close()
			return def, err
		}
		c := database.ColumnDef{OrigName: name, Name: name, Nullable: strings.EqualFold(nullable, "YES"), Comment: comment.String, Collation: coll.String, SRID: srs.String}
		c.Type, c.Length, c.Unsigned, c.Zerofill = splitColumnType(ctype)
		if c.Type == "ENUM" || c.Type == "SET" {
			if vals, err := parseEnumArgs(c.Length); err == nil {
				q := make([]string, len(vals))
				for i, v := range vals {
					q[i] = quoteString(v)
				}
				c.Length = strings.Join(q, ",")
			}
		}
		lx := strings.ToLower(extra)
		c.AutoIncrement = strings.Contains(lx, "auto_increment")
		c.Invisible = strings.Contains(lx, "invisible")
		if m := generatedRe.FindStringSubmatch(extra); m != nil {
			c.GeneratedKind = strings.ToUpper(m[1])
			c.Generated = strings.TrimSpace(gen.String)
		}
		if m := onUpdateRe.FindStringSubmatch(extra); m != nil {
			c.OnUpdate = strings.ToUpper(m[1])
		}
		switch {
		case c.Generated != "" || c.AutoIncrement:
		case maria:
			if dflt.Valid {
				c.DefaultKind, c.Default = mariaDefault(dflt.String)
			}
		case dflt.Valid:
			if strings.Contains(lx, "default_generated") {
				c.DefaultKind, c.Default = "expression", dflt.String
			} else {
				c.DefaultKind, c.Default = "value", dflt.String
			}
		case c.Nullable:
			c.DefaultKind = "null"
		}
		if c.DefaultKind == "expression" && strings.EqualFold(strings.TrimSuffix(c.Default, "()"), "current_timestamp") {
			c.Default = "CURRENT_TIMESTAMP"
		}
		if c.OnUpdate == "CURRENT_TIMESTAMP()" {
			c.OnUpdate = "CURRENT_TIMESTAMP"
		}
		if !textTypes[c.Type] {
			c.Collation = ""
		}
		def.Columns = append(def.Columns, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return def, err
	}
	if len(def.Columns) == 0 {
		return def, ddlError("tabela %q não encontrada em %q", table, database_)
	}

	if def.Indexes, err = d.loadIndexes(ctx, db, database_, table, maria); err != nil {
		return def, err
	}
	fks, err := in.ForeignKeys(ctx, database_, table)
	if err != nil {
		return def, err
	}
	for _, fk := range fks {
		ref := fk.RefDatabase
		if ref == database_ {
			ref = ""
		}
		def.ForeignKeys = append(def.ForeignKeys, database.ForeignKeyDef{OrigName: fk.Name, Name: fk.Name, Columns: fk.Columns, RefDatabase: ref, RefTable: fk.RefTable, RefColumns: fk.RefColumns, OnUpdate: fk.OnUpdate, OnDelete: fk.OnDelete})
	}
	checks, err := in.CheckConstraints(ctx, database_, table)
	if err != nil {
		return def, err
	}
	for _, c := range checks {
		def.Checks = append(def.Checks, database.CheckDef{OrigName: c.Name, Name: c.Name, Clause: c.Clause})
	}

	var engine, coll, comment, opts, autoInc sql.NullString
	err = in.withFreshStats(ctx, func(q queryer) error {
		r, err := q.QueryContext(ctx, `SELECT ENGINE, TABLE_COLLATION, TABLE_COMMENT, CREATE_OPTIONS, AUTO_INCREMENT FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?`, database_, table)
		if err != nil {
			return err
		}
		defer r.Close()
		if r.Next() {
			return r.Scan(&engine, &coll, &comment, &opts, &autoInc)
		}
		return r.Err()
	})
	if err != nil {
		return def, err
	}
	def.Options = database.TableOptions{Engine: engine.String, Collation: coll.String, Comment: comment.String, AutoIncrement: autoInc.String}
	for _, m := range optionRe.FindAllStringSubmatch(opts.String, -1) {
		switch strings.ToLower(m[1]) {
		case "row_format":
			def.Options.RowFormat = strings.ToUpper(m[2])
		case "checksum":
			def.Options.Checksum = m[2] == "1"
		case "max_rows":
			def.Options.MaxRows = m[2]
		case "avg_row_length":
			def.Options.AvgRowLength = m[2]
		}
	}
	// MariaDB does not flag partitioned tables in CREATE_OPTIONS.
	if maria || strings.Contains(strings.ToLower(opts.String), "partitioned") {
		if ddl, err := in.CreateStatement(ctx, database_, database.ObjTable, table); err == nil {
			if m := partitionRe.FindStringSubmatch(ddl); m != nil {
				def.Options.Partition = strings.TrimSpace(m[1])
			}
		}
	}
	return def, nil
}

func (d *Driver) loadIndexes(ctx context.Context, db *sql.DB, database_, table string, maria bool) ([]database.IndexDef, error) {
	extra := ", EXPRESSION, IS_VISIBLE"
	if maria {
		extra = ", IGNORED"
	}
	base := `SELECT INDEX_NAME, NON_UNIQUE, COLUMN_NAME, SUB_PART, COLLATION, INDEX_TYPE, INDEX_COMMENT%s
FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
ORDER BY INDEX_NAME = 'PRIMARY' DESC, INDEX_NAME, SEQ_IN_INDEX`
	rows, err := db.QueryContext(ctx, fmt.Sprintf(base, extra), database_, table)
	if isErr(err, 1054) { // older servers without the extra columns
		extra = ""
		rows, err = db.QueryContext(ctx, fmt.Sprintf(base, ""), database_, table)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []database.IndexDef
	pos := map[string]int{}
	for rows.Next() {
		var name, typ string
		var nonUnique int
		var col, coll, comment, expr, vis sql.NullString
		var sub sql.NullInt64
		dest := []any{&name, &nonUnique, &col, &sub, &coll, &typ, &comment}
		switch {
		case extra == "":
		case maria:
			dest = append(dest, &vis)
		default:
			dest = append(dest, &expr, &vis)
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		p, ok := pos[name]
		if !ok {
			ix := database.IndexDef{OrigName: name, Name: name, Comment: comment.String}
			switch {
			case name == "PRIMARY":
				ix.Kind = "PRIMARY"
			case strings.EqualFold(typ, "FULLTEXT"), strings.EqualFold(typ, "SPATIAL"):
				ix.Kind = strings.ToUpper(typ)
			case nonUnique == 0:
				ix.Kind = "UNIQUE"
			default:
				ix.Kind = "KEY"
			}
			if strings.EqualFold(typ, "HASH") {
				ix.Algorithm = "HASH"
			}
			if maria {
				ix.Invisible = strings.EqualFold(vis.String, "YES")
			} else if vis.Valid {
				ix.Invisible = strings.EqualFold(vis.String, "NO")
			}
			out = append(out, ix)
			p = len(out) - 1
			pos[name] = p
		}
		part := database.IndexPart{Column: col.String, Length: int(sub.Int64), Desc: coll.String == "D"}
		if !col.Valid && expr.Valid {
			part.Expression = expr.String
		}
		out[p].Parts = append(out[p].Parts, part)
	}
	return out, rows.Err()
}

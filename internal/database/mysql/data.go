package mysql

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"anansi/internal/database"
)

var _ database.DataSQL = (*Driver)(nil)

// FindKey implements database.DataSQL.
func (d *Driver) FindKey(cols []database.ColumnInfo, idx []database.IndexInfo) database.TableKey {
	notNull := map[string]bool{}
	for _, c := range cols {
		notNull[strings.ToLower(c.Name)] = !c.Nullable
	}
	for _, i := range idx {
		if i.Primary {
			return database.TableKey{Kind: "primary", Name: i.Name, Columns: append([]string(nil), i.Columns...)}
		}
	}
	for _, i := range idx {
		if !i.Unique || len(i.Columns) == 0 {
			continue
		}
		ok := true
		for _, c := range i.Columns {
			if !notNull[strings.ToLower(c)] {
				ok = false // nullable or functional index part
				break
			}
		}
		if ok {
			return database.TableKey{Kind: "unique", Name: i.Name, Columns: append([]string(nil), i.Columns...)}
		}
	}
	return database.TableKey{Kind: "none"}
}

func findCol(cols []database.ColumnInfo, name string) (database.ColumnInfo, bool) {
	for _, c := range cols {
		if c.Name == name {
			return c, true
		}
	}
	for _, c := range cols {
		if strings.EqualFold(c.Name, name) {
			return c, true
		}
	}
	return database.ColumnInfo{}, false
}

func colKind(c database.ColumnInfo) string {
	t := strings.ToLower(c.DataType)
	switch t {
	case "tinytext", "text", "mediumtext", "longtext", "json":
		return "longtext"
	case "tinyblob", "blob", "mediumblob", "longblob":
		return "longbinary"
	case "binary", "varbinary":
		return "binary"
	case "bit":
		return "bit"
	case "float", "double", "real":
		return "float"
	case "date", "datetime", "timestamp", "time", "year":
		return "temporal"
	case "geometry", "point", "linestring", "polygon", "multipoint", "multilinestring", "multipolygon", "geometrycollection", "geomcollection":
		return "spatial"
	}
	if t == "varchar" || t == "char" {
		if n := typeLength(c.ColumnType); n > 1024 {
			return "longtext"
		}
	}
	return "plain"
}

// typeLength extracts N from "varchar(N)".
func typeLength(columnType string) int {
	i := strings.IndexByte(columnType, '(')
	j := strings.IndexByte(columnType, ')')
	if i < 0 || j < i {
		return 0
	}
	n, _ := strconv.Atoi(columnType[i+1 : j])
	return n
}

func cleanFilter(f string) string {
	f = strings.TrimSpace(f)
	if len(f) >= 5 && strings.EqualFold(f[:5], "WHERE") && (len(f) == 5 || f[5] == ' ' || f[5] == '\n' || f[5] == '\t' || f[5] == '(') {
		f = strings.TrimSpace(f[5:])
	}
	return strings.TrimRight(f, "; \t\n")
}

func tableRef(db, table string) (string, error) {
	if err := checkObjectName("database", db); err != nil {
		return "", err
	}
	if strings.TrimSpace(table) == "" || strings.ContainsRune(table, 0) {
		return "", ddlError("tabela inválida")
	}
	return QuoteIdent(db) + "." + QuoteIdent(table), nil
}

// SelectSQL implements database.DataSQL.
func (d *Driver) SelectSQL(spec database.BrowseSpec, cols []database.ColumnInfo, key database.TableKey) (database.SelectPlan, error) {
	var plan database.SelectPlan
	from, err := tableRef(spec.Database, spec.Table)
	if err != nil {
		return plan, err
	}
	if len(cols) == 0 {
		return plan, ddlError("a tabela %q não tem colunas visíveis (privilégio insuficiente?)", spec.Table)
	}
	var chosen []database.ColumnInfo
	seen := map[string]bool{}
	add := func(name string) error {
		c, ok := findCol(cols, name)
		if !ok {
			return ddlError("coluna inexistente: %q", name)
		}
		if !seen[c.Name] {
			seen[c.Name] = true
			chosen = append(chosen, c)
		}
		return nil
	}
	if len(spec.Columns) == 0 {
		for _, c := range cols {
			_ = add(c.Name)
		}
	} else {
		for _, n := range spec.Columns {
			if err := add(n); err != nil {
				return plan, err
			}
		}
	}
	if key.Kind != "none" {
		for _, n := range key.Columns {
			if err := add(n); err != nil {
				return plan, err
			}
		}
	}
	partialOK := spec.PartialLength > 0 && key.Kind != "none"
	var exprs, flags []string
	for _, c := range chosen {
		q := QuoteIdent(c.Name)
		partial := false
		if partialOK && !isKeyCol(key, c.Name) {
			switch colKind(c) {
			case "longtext":
				exprs = append(exprs, fmt.Sprintf("LEFT(%s, %d) AS %s", q, spec.PartialLength, q))
				flags = append(flags, fmt.Sprintf("(CHAR_LENGTH(%s) > %d)", q, spec.PartialLength))
				partial = true
			case "longbinary":
				exprs = append(exprs, fmt.Sprintf("LEFT(%s, %d) AS %s", q, spec.PartialLength/2, q))
				flags = append(flags, fmt.Sprintf("(LENGTH(%s) > %d)", q, spec.PartialLength/2))
				partial = true
			}
		}
		if !partial {
			exprs = append(exprs, q)
		}
		plan.Columns = append(plan.Columns, c.Name)
		plan.Partial = append(plan.Partial, partial)
	}
	// Truncation flags follow the real columns, one per partial column.
	exprs = append(exprs, flags...)
	var b strings.Builder
	b.WriteString("SELECT ")
	b.WriteString(strings.Join(exprs, ", "))
	b.WriteString(" FROM ")
	b.WriteString(from)
	if f := cleanFilter(spec.Filter); f != "" {
		b.WriteString(" WHERE " + f)
	}
	if len(spec.Sort) > 0 {
		var terms []string
		for _, s := range spec.Sort {
			c, ok := findCol(cols, s.Column)
			if !ok {
				return plan, ddlError("coluna de ordenação inexistente: %q", s.Column)
			}
			t := QuoteIdent(c.Name)
			if s.Desc {
				t += " DESC"
			}
			terms = append(terms, t)
		}
		b.WriteString(" ORDER BY " + strings.Join(terms, ", "))
	}
	limit := spec.Limit
	if limit <= 0 {
		limit = 1000
	}
	if spec.NoLimit {
		plan.SQL = b.String()
		return plan, nil
	}
	if spec.Offset > 0 {
		fmt.Fprintf(&b, " LIMIT %d, %d", spec.Offset, limit)
	} else {
		fmt.Fprintf(&b, " LIMIT %d", limit)
	}
	plan.SQL = b.String()
	return plan, nil
}

func isKeyCol(k database.TableKey, name string) bool {
	for _, c := range k.Columns {
		if strings.EqualFold(c, name) {
			return true
		}
	}
	return false
}

// CountSQL implements database.DataSQL.
func (d *Driver) CountSQL(db, table, filter string) (string, error) {
	from, err := tableRef(db, table)
	if err != nil {
		return "", err
	}
	s := "SELECT COUNT(*) FROM " + from
	if f := cleanFilter(filter); f != "" {
		s += " WHERE " + f
	}
	return s, nil
}

// argument converts a cell value into an SQL fragment and its argument.
func argument(c database.ColumnInfo, v database.CellValue) (frag string, arg any, hasArg bool, err error) {
	switch v.Kind {
	case database.ValueNull:
		return "NULL", nil, false, nil
	case database.ValueDefault:
		return "DEFAULT", nil, false, nil
	case database.ValueExpression:
		e := strings.TrimSpace(v.Text)
		if e == "" || strings.Contains(e, ";") {
			return "", nil, false, ddlError("expressão inválida para a coluna %q", c.Name)
		}
		return "(" + e + ")", nil, false, nil
	case database.ValueHex:
		h := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(v.Text), "0x"), "0X")
		b, err := hex.DecodeString(h)
		if err != nil {
			return "", nil, false, ddlError("valor hexadecimal inválido na coluna %q", c.Name)
		}
		return "?", b, true, nil
	case database.ValueText, "":
		if colKind(c) == "bit" {
			t := strings.TrimSpace(v.Text)
			if n, err := strconv.ParseUint(t, 10, 64); err == nil {
				return "?", n, true, nil
			}
			if strings.HasPrefix(t, "b'") || strings.HasPrefix(t, "B'") {
				bits := strings.TrimSuffix(t[2:], "'")
				if n, err := strconv.ParseUint(bits, 2, 64); err == nil {
					return "?", n, true, nil
				}
			}
			return "", nil, false, ddlError("valor inválido para a coluna BIT %q: use um número ou b'0101'", c.Name)
		}
		return "?", v.Text, true, nil
	}
	return "", nil, false, ddlError("tipo de valor inválido: %q", v.Kind)
}

// whereClause builds the row identification.
func whereClause(cols []database.ColumnInfo, key database.TableKey, where []database.ColumnValue) (string, []any, bool, error) {
	if len(where) == 0 {
		return "", nil, false, ddlError("a linha não pode ser identificada (sem valores de chave)")
	}
	var parts []string
	var args []any
	covered := map[string]bool{}
	for _, w := range where {
		c, ok := findCol(cols, w.Column)
		if !ok {
			return "", nil, false, ddlError("coluna inexistente: %q", w.Column)
		}
		q := QuoteIdent(c.Name)
		covered[strings.ToLower(c.Name)] = true
		switch w.Value.Kind {
		case database.ValueNull:
			parts = append(parts, q+" IS NULL")
			continue
		case database.ValueHex:
			h := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(w.Value.Text), "0x"), "0X")
			b, err := hex.DecodeString(h)
			if err != nil {
				return "", nil, false, ddlError("valor hexadecimal inválido na coluna %q", c.Name)
			}
			if colKind(c) == "spatial" {
				parts = append(parts, "CAST("+q+" AS BINARY) = ?")
			} else {
				parts = append(parts, q+" = ?")
			}
			args = append(args, b)
			continue
		case database.ValueText, "":
		default:
			return "", nil, false, ddlError("valor de identificação inválido na coluna %q", c.Name)
		}
		switch colKind(c) {
		case "float", "temporal", "spatial":
			// Compare the text the grid showed, avoiding float rounding and
			// fractional-second formatting differences.
			parts = append(parts, "CAST("+q+" AS CHAR) = ?")
		default:
			if strings.EqualFold(c.DataType, "json") {
				parts = append(parts, "CAST("+q+" AS CHAR) = ?")
			} else {
				parts = append(parts, q+" = ?")
			}
		}
		args = append(args, w.Value.Text)
	}
	byKey := key.Kind != "none" && len(key.Columns) > 0
	for _, kc := range key.Columns {
		if !covered[strings.ToLower(kc)] {
			byKey = false
		}
	}
	return strings.Join(parts, " AND "), args, byKey, nil
}

func display(sql string, args []any) string {
	var b strings.Builder
	i := 0
	for _, r := range sql {
		if r == '?' && i < len(args) {
			switch v := args[i].(type) {
			case nil:
				b.WriteString("NULL")
			case []byte:
				if len(v) == 0 {
					b.WriteString("X''")
				} else {
					b.WriteString("0x" + strings.ToUpper(hex.EncodeToString(v)))
				}
			case string:
				b.WriteString(quoteString(v))
			default:
				fmt.Fprint(&b, v)
			}
			i++
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// RowSQL implements database.DataSQL.
func (d *Driver) RowSQL(db, table string, cols []database.ColumnInfo, key database.TableKey, e database.RowEdit) (database.Statement, error) {
	var st database.Statement
	from, err := tableRef(db, table)
	if err != nil {
		return st, err
	}
	var b strings.Builder
	var args []any
	switch e.Op {
	case database.EditInsert:
		var names, vals []string
		for _, v := range e.Values {
			c, ok := findCol(cols, v.Column)
			if !ok {
				return st, ddlError("coluna inexistente: %q", v.Column)
			}
			frag, arg, has, err := argument(c, v.Value)
			if err != nil {
				return st, err
			}
			names = append(names, QuoteIdent(c.Name))
			vals = append(vals, frag)
			if has {
				args = append(args, arg)
			}
		}
		fmt.Fprintf(&b, "INSERT INTO %s (%s) VALUES (%s)", from, strings.Join(names, ", "), strings.Join(vals, ", "))
	case database.EditUpdate:
		if len(e.Values) == 0 {
			return st, ddlError("nenhuma coluna alterada")
		}
		var sets []string
		for _, v := range e.Values {
			c, ok := findCol(cols, v.Column)
			if !ok {
				return st, ddlError("coluna inexistente: %q", v.Column)
			}
			frag, arg, has, err := argument(c, v.Value)
			if err != nil {
				return st, err
			}
			sets = append(sets, QuoteIdent(c.Name)+" = "+frag)
			if has {
				args = append(args, arg)
			}
		}
		where, wargs, byKey, err := whereClause(cols, key, e.Where)
		if err != nil {
			return st, err
		}
		args = append(args, wargs...)
		fmt.Fprintf(&b, "UPDATE %s SET %s WHERE %s", from, strings.Join(sets, ", "), where)
		if !byKey {
			b.WriteString(" LIMIT 1")
		}
	case database.EditDelete:
		where, wargs, byKey, err := whereClause(cols, key, e.Where)
		if err != nil {
			return st, err
		}
		args = append(args, wargs...)
		fmt.Fprintf(&b, "DELETE FROM %s WHERE %s", from, where)
		if !byKey {
			b.WriteString(" LIMIT 1")
		}
	default:
		return st, ddlError("operação inválida: %q", e.Op)
	}
	st.SQL, st.Args = b.String(), args
	st.Display = display(st.SQL, args)
	return st, nil
}

// FetchRowSQL implements database.DataSQL.
func (d *Driver) FetchRowSQL(db, table string, cols []database.ColumnInfo, key database.TableKey, where []database.ColumnValue, columns []string) (database.Statement, error) {
	var st database.Statement
	from, err := tableRef(db, table)
	if err != nil {
		return st, err
	}
	var sel []string
	for _, n := range columns {
		c, ok := findCol(cols, n)
		if !ok {
			return st, ddlError("coluna inexistente: %q", n)
		}
		sel = append(sel, QuoteIdent(c.Name))
	}
	if len(sel) == 0 {
		sel = []string{"*"}
	}
	w, args, _, err := whereClause(cols, key, where)
	if err != nil {
		return st, err
	}
	st.SQL = fmt.Sprintf("SELECT %s FROM %s WHERE %s LIMIT 1", strings.Join(sel, ", "), from, w)
	st.Args = args
	st.Display = display(st.SQL, args)
	return st, nil
}

// DistinctSQL implements database.DataSQL.
func (d *Driver) DistinctSQL(db, table, column, filter string, limit int) (string, error) {
	from, err := tableRef(db, table)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(column) == "" {
		return "", ddlError("coluna inválida")
	}
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	q := QuoteIdent(column)
	s := "SELECT " + q + ", COUNT(*) AS `__count` FROM " + from
	if f := cleanFilter(filter); f != "" {
		s += " WHERE " + f
	}
	return fmt.Sprintf("%s GROUP BY %s ORDER BY `__count` DESC, %s LIMIT %d", s, q, q, limit), nil
}

// LookupSQL implements database.DataSQL.
func (d *Driver) LookupSQL(db, table, column, labelColumn, search string, limit int) (database.Statement, error) {
	var st database.Statement
	from, err := tableRef(db, table)
	if err != nil {
		return st, err
	}
	if strings.TrimSpace(column) == "" {
		return st, ddlError("coluna inválida")
	}
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	q := QuoteIdent(column)
	label := "NULL AS `__label`"
	order := q
	if labelColumn != "" {
		label = "LEFT(" + QuoteIdent(labelColumn) + ", 256) AS `__label`"
		order = "`__label`, " + q
	}
	s := fmt.Sprintf("SELECT DISTINCT %s, %s FROM %s", q, label, from)
	if search = strings.TrimSpace(search); search != "" {
		// An explicit escape character works with and without
		// NO_BACKSLASH_ESCAPES.
		like := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(search) + "%"
		if labelColumn != "" {
			s += fmt.Sprintf(" WHERE CAST(%s AS CHAR) LIKE ? ESCAPE '!' OR %s LIKE ? ESCAPE '!'", q, QuoteIdent(labelColumn))
			st.Args = []any{like, like}
		} else {
			s += fmt.Sprintf(" WHERE CAST(%s AS CHAR) LIKE ? ESCAPE '!'", q)
			st.Args = []any{like}
		}
	}
	st.SQL = fmt.Sprintf("%s ORDER BY %s LIMIT %d", s, order, limit)
	st.Display = display(st.SQL, st.Args)
	return st, nil
}

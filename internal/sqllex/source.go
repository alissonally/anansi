package sqllex

import "strings"

// SelectItem is one entry of a select list as far as editing is concerned:
// a star (optionally qualified), a plain column reference, or something
// computed (Column == "" and !Star).
type SelectItem struct {
	Star      bool
	Qualifier string // table or alias before "." ("" when absent)
	Column    string
}

// TableSource describes a SELECT that reads a single table, the shape whose
// results can be edited like the table data grid (HeidiSQL "editable
// query results").
type TableSource struct {
	Database string
	Table    string
	Alias    string
	Items    []SelectItem
}

func identText(t Token) (string, bool) {
	switch t.Kind {
	case Word:
		return t.Text, true
	case QuotedIdent:
		return t.Ident(), true
	}
	return "", false
}

// SingleTableSource analyses stmt and reports whether it is a SELECT from
// exactly one base table without joins, grouping, DISTINCT or set
// operations. Subqueries elsewhere (e.g. in WHERE) are allowed.
func SingleTableSource(stmt string) (TableSource, bool) {
	var src TableSource
	var toks []Token
	for _, t := range Tokenize(stmt) {
		if t.Significant() {
			toks = append(toks, t)
		}
	}
	if len(toks) < 4 || !toks[0].IsKeyword("SELECT") {
		return src, false
	}
	// Find FROM at depth 0 and reject constructs that break row identity.
	depth, from := 0, -1
	for i, t := range toks {
		if t.Kind == Punct {
			switch t.Text {
			case "(":
				depth++
			case ")":
				depth--
			}
			continue
		}
		if depth != 0 {
			continue
		}
		if t.IsKeyword("UNION", "INTERSECT", "EXCEPT", "GROUP", "HAVING", "DISTINCT", "DISTINCTROW", "WINDOW", "INTO", "JOIN", "STRAIGHT_JOIN", "NATURAL", "FOR", "LOCK") {
			return src, false
		}
		if t.IsKeyword("FROM") && from < 0 {
			from = i
		}
	}
	if from < 0 || from+1 >= len(toks) {
		return src, false
	}
	// Table reference: [db.]table [[AS] alias]
	i := from + 1
	name, ok := identText(toks[i])
	if !ok {
		return src, false // derived table or something else
	}
	src.Table = name
	i++
	if i+1 < len(toks) && toks[i].Kind == Dot {
		t2, ok := identText(toks[i+1])
		if !ok {
			return src, false
		}
		src.Database, src.Table = src.Table, t2
		i += 2
	}
	if i < len(toks) && toks[i].IsKeyword("AS") {
		i++
	}
	if i < len(toks) {
		if a, ok := identText(toks[i]); ok && !toks[i].IsKeyword("WHERE", "ORDER", "LIMIT", "PARTITION", "USE", "IGNORE", "FORCE", "PROCEDURE") {
			src.Alias = a
			i++
		}
	}
	// Whatever follows must not be another table (comma join).
	if i < len(toks) && toks[i].Kind == Punct && toks[i].Text == "," {
		return src, false
	}
	// Select list items between SELECT (and modifiers) and FROM.
	start := 1
	for start < from && toks[start].IsKeyword("ALL", "HIGH_PRIORITY", "SQL_SMALL_RESULT", "SQL_BIG_RESULT", "SQL_BUFFER_RESULT", "SQL_NO_CACHE", "SQL_CALC_FOUND_ROWS") {
		start++
	}
	var item []Token
	flush := func() {
		src.Items = append(src.Items, classifyItem(item))
		item = nil
	}
	depth = 0
	for _, t := range toks[start:from] {
		if t.Kind == Punct {
			switch t.Text {
			case "(":
				depth++
			case ")":
				depth--
			case ",":
				if depth == 0 {
					flush()
					continue
				}
			}
		}
		item = append(item, t)
	}
	flush()
	return src, len(src.Items) > 0
}

func classifyItem(toks []Token) SelectItem {
	switch {
	case len(toks) == 1 && toks[0].Kind == Operator && toks[0].Text == "*":
		return SelectItem{Star: true}
	case len(toks) == 3 && toks[1].Kind == Dot && toks[2].Kind == Operator && toks[2].Text == "*":
		q, _ := identText(toks[0])
		return SelectItem{Star: true, Qualifier: q}
	}
	// Column, optionally qualified, optionally aliased to itself.
	var parts []string
	i := 0
	for i < len(toks) {
		name, ok := identText(toks[i])
		if !ok || toks[i].IsKeyword("AS") {
			break
		}
		parts = append(parts, name)
		i++
		if i < len(toks) && toks[i].Kind == Dot {
			i++
			continue
		}
		break
	}
	if len(parts) == 0 || len(parts) > 3 {
		return SelectItem{}
	}
	col := parts[len(parts)-1]
	qual := ""
	if len(parts) >= 2 {
		qual = parts[len(parts)-2]
	}
	if i < len(toks) {
		// Alias: allowed only when it keeps the column name.
		j := i
		if toks[j].IsKeyword("AS") {
			j++
		}
		if j != len(toks)-1 {
			return SelectItem{}
		}
		alias, ok := identText(toks[j])
		if !ok && toks[j].Kind == String {
			alias, ok = strings.Trim(toks[j].Text, `'"`), true
		}
		if !ok || !strings.EqualFold(alias, col) {
			return SelectItem{}
		}
	}
	return SelectItem{Qualifier: qual, Column: col}
}

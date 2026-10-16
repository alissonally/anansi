package autocomplete

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"anansi/internal/database"
)

// Catalog provides metadata for completion. Implementations should be
// cached; Complete calls them on every request.
type Catalog interface {
	Databases(ctx context.Context) ([]database.DatabaseInfo, error)
	Tables(ctx context.Context, db string) ([]database.TableInfo, error)
	Columns(ctx context.Context, db, table string) ([]database.ColumnInfo, error)
	Routines(ctx context.Context, db string) ([]database.RoutineInfo, error)
}

// Item kinds.
const (
	KindKeyword  = "keyword"
	KindFunction = "function"
	KindType     = "type"
	KindDatabase = "database"
	KindTable    = "table"
	KindView     = "view"
	KindColumn   = "column"
	KindAlias    = "alias"
	KindRoutine  = "routine"
)

// Item is a completion proposal.
type Item struct {
	Label      string `json:"label"`
	Kind       string `json:"kind"`
	Detail     string `json:"detail,omitempty"`
	InsertText string `json:"insertText"`
	SortText   string `json:"sortText"`
	// Snippet marks InsertText as a Monaco snippet (e.g. "COUNT($0)").
	Snippet bool `json:"snippet,omitempty"`
}

// Request is a completion request. Positions are UTF-16 code units, as used
// by the editor.
type Request struct {
	SQL      string `json:"sql"`
	Offset   int    `json:"offset"`
	Database string `json:"database"` // tab's default database
}

// Response carries the items and the length (UTF-16 units) of the text
// before the cursor that the items replace.
type Response struct {
	Items        []Item `json:"items"`
	ReplaceLen   int    `json:"replaceLen"`
	Clause       string `json:"clause"`
	MetadataNote string `json:"metadataNote,omitempty"`
	Incomplete   bool   `json:"incomplete"`
}

// UTF16ToByte converts a UTF-16 offset into a byte offset of s.
func UTF16ToByte(s string, off int) int {
	units := 0
	for i, r := range s {
		if units >= off {
			return i
		}
		units += len(utf16.Encode([]rune{r}))
	}
	return len(s)
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// Engine produces completions.
type Engine struct{}

// maxItems bounds the size of a response.
const maxItems = 2000

// Complete returns proposals for req using cat for metadata. Metadata
// failures degrade to keyword-only completion.
func (Engine) Complete(ctx context.Context, cat Catalog, req Request) Response {
	pos := UTF16ToByte(req.SQL, req.Offset)
	ac := Analyze(req.SQL, pos)
	resp := Response{Clause: ac.Clause}
	if ac.InString {
		return resp
	}
	resp.ReplaceLen = utf16Len(req.SQL[ac.ReplaceFrom:pos])
	b := &builder{prefix: ac.Prefix, quoted: ac.PrefixQuoted, seen: map[string]bool{}}
	var metaErr error
	note := func(err error) {
		if err != nil && metaErr == nil {
			metaErr = err
		}
	}
	defDB := req.Database

	if len(ac.Qualifiers) > 0 {
		completeQualified(ctx, cat, ac, defDB, b, note)
	} else {
		switch ac.Clause {
		case "start":
			for i, k := range statementKeywords {
				b.add(Item{Label: k, Kind: KindKeyword, InsertText: k, SortText: fmt.Sprintf("1%03d", i)})
			}
		case "use":
			dbs, err := cat.Databases(ctx)
			note(err)
			for _, d := range dbs {
				b.ident(d.Name, KindDatabase, "database", "1")
			}
		case "from", "join", "tableName", "into":
			addTables(ctx, cat, defDB, b, note, "1")
			dbs, err := cat.Databases(ctx)
			note(err)
			for _, d := range dbs {
				b.ident(d.Name, KindDatabase, "database", "3")
			}
			if ac.Clause == "from" {
				b.keyword("SELECT", "4") // derived table
			}
			if ac.Clause == "join" {
				b.keyword("LATERAL", "4")
			}
		case "afterTable", "alterTable", "showTables":
			for i, k := range keywordsAfter["afterTable"] {
				b.add(Item{Label: k, Kind: KindKeyword, InsertText: k, SortText: fmt.Sprintf("1%03d", i)})
			}
			if ac.Clause == "alterTable" {
				for _, k := range ddlKeywords {
					b.keyword(k, "2")
				}
			}
			if ac.Clause == "showTables" {
				b.keyword("FROM", "1")
				b.keyword("LIKE", "1")
			}
		case "call":
			rs, err := cat.Routines(ctx, defDB)
			note(err)
			for _, r := range rs {
				if strings.EqualFold(r.Type, "PROCEDURE") {
					b.ident(r.Name, KindRoutine, "procedure", "1")
				}
			}
		case "insertColumns":
			if ac.InsertTable != nil {
				addColumns(ctx, cat, defDB, *ac.InsertTable, false, b, note, "1")
			}
		case "ddl":
			for _, t := range dataTypes {
				b.add(Item{Label: t, Kind: KindType, InsertText: t, SortText: "2" + t})
			}
			for _, k := range ddlKeywords {
				b.keyword(k, "3")
			}
		case "limit":
			b.keyword("OFFSET", "1")
		case "afterInsertColumns":
			b.keyword("VALUES", "1")
			b.keyword("SELECT", "1")
		default: // select, where, on, groupby, orderby, having, set, values, other
			completeExpression(ctx, cat, ac, defDB, b, note)
		}
	}

	resp.Items = b.items
	if len(resp.Items) > maxItems {
		sort.Slice(resp.Items, func(i, j int) bool { return resp.Items[i].SortText < resp.Items[j].SortText })
		resp.Items = resp.Items[:maxItems]
		resp.Incomplete = true
	}
	if metaErr != nil {
		resp.MetadataNote = "metadados indisponíveis: " + metaErr.Error()
	}
	return resp
}

func completeQualified(ctx context.Context, cat Catalog, ac Context, defDB string, b *builder, note func(error)) {
	q := ac.Qualifiers
	exprClause := ac.Clause != "from" && ac.Clause != "join" && ac.Clause != "tableName" && ac.Clause != "into" && ac.Clause != "use" && ac.Clause != "call"
	if len(q) == 1 {
		// alias. / table. => columns
		if exprClause || ac.Clause == "afterTable" || ac.Clause == "on" {
			for _, r := range ac.Tables {
				if strings.EqualFold(r.Name(), q[0]) {
					if r.Subquery {
						return // columns of derived tables are not resolved
					}
					addColumns(ctx, cat, defDB, r, false, b, note, "1")
					return
				}
			}
			// Unqualified table name not in FROM (e.g. typing the select
			// list first): try a table of the default database.
			if defDB != "" {
				if found := addColumns(ctx, cat, defDB, TableRef{Table: q[0]}, false, b, note, "1"); found {
					return
				}
			}
		}
		// db. => tables (and routines in CALL)
		if ac.Clause == "call" {
			rs, err := cat.Routines(ctx, q[0])
			note(err)
			for _, r := range rs {
				if strings.EqualFold(r.Type, "PROCEDURE") {
					b.ident(r.Name, KindRoutine, "procedure", "1")
				}
			}
			return
		}
		addTables(ctx, cat, q[0], b, note, "1")
		return
	}
	// db.table. => columns
	addColumns(ctx, cat, defDB, TableRef{Database: q[len(q)-2], Table: q[len(q)-1]}, false, b, note, "1")
}

func completeExpression(ctx context.Context, cat Catalog, ac Context, defDB string, b *builder, note func(error)) {
	// Columns of tables in scope.
	multiple := len(ac.Tables) > 1
	for _, r := range ac.Tables {
		if r.Subquery {
			continue
		}
		addColumns(ctx, cat, defDB, r, multiple, b, note, "1")
	}
	// Aliases and table names (for qualification).
	for _, r := range ac.Tables {
		detail := "alias"
		if r.Table != "" {
			detail = "alias de " + r.Table
		}
		if r.Alias == "" {
			detail = "tabela"
		}
		b.ident(r.Name(), KindAlias, detail, "2")
	}
	if len(ac.Tables) == 0 && ac.Clause == "select" {
		// SELECT before FROM: offer tables so the user can qualify.
		addTables(ctx, cat, defDB, b, note, "4")
	}
	kws := keywordsAfter[ac.Clause]
	for _, k := range kws {
		b.keyword(k, "3")
	}
	names := make([]string, 0, len(functions))
	for f := range functions {
		names = append(names, f)
	}
	sort.Strings(names)
	for _, f := range names {
		b.add(Item{Label: f, Kind: KindFunction, Detail: functions[f], InsertText: f + "($0)", Snippet: true, SortText: "5" + f})
	}
	if ac.Clause == "select" || ac.Clause == "where" {
		b.keyword("CAST", "6")
	}
}

func addTables(ctx context.Context, cat Catalog, db string, b *builder, note func(error), rank string) {
	if db == "" {
		return
	}
	ts, err := cat.Tables(ctx, db)
	note(err)
	for _, t := range ts {
		kind := KindTable
		if strings.Contains(t.Type, "view") {
			kind = KindView
		}
		detail := t.Type
		if t.Comment != "" {
			detail += " — " + t.Comment
		}
		b.ident(t.Name, kind, detail, rank)
	}
}

// addColumns adds the columns of r. It reports whether the table was found.
func addColumns(ctx context.Context, cat Catalog, defDB string, r TableRef, qualify bool, b *builder, note func(error), rank string) bool {
	db := r.Database
	if db == "" {
		db = defDB
	}
	if db == "" || r.Table == "" {
		return false
	}
	cols, err := cat.Columns(ctx, db, r.Table)
	note(err)
	if len(cols) == 0 {
		return false
	}
	for _, c := range cols {
		detail := c.ColumnType
		if c.Key == "PRI" {
			detail += " · PK"
		}
		if !c.Nullable {
			detail += " · NOT NULL"
		}
		detail += " — " + r.Name()
		if qualify {
			// Several tables in scope: keep the column label, sort by table.
			b.add(Item{Label: c.Name, Kind: KindColumn, Detail: detail, InsertText: quoteIfNeeded(c.Name, b.quoted), SortText: rank + r.Name() + "\x00" + fmt.Sprintf("%04d", c.Position)})
			continue
		}
		b.add(Item{Label: c.Name, Kind: KindColumn, Detail: detail, InsertText: quoteIfNeeded(c.Name, b.quoted), SortText: rank + fmt.Sprintf("%04d", c.Position)})
	}
	return true
}

type builder struct {
	prefix string
	quoted bool
	items  []Item
	seen   map[string]bool
}

func (b *builder) add(it Item) {
	k := it.Kind + "\x00" + it.Label + "\x00" + it.Detail
	if b.seen[k] {
		return
	}
	if b.prefix != "" && !matchPrefix(it.Label, b.prefix) {
		return
	}
	if b.quoted && (it.Kind == KindKeyword || it.Kind == KindFunction || it.Kind == KindType) {
		return
	}
	b.seen[k] = true
	b.items = append(b.items, it)
}

func (b *builder) keyword(k, rank string) {
	b.add(Item{Label: k, Kind: KindKeyword, InsertText: k, SortText: rank + k})
}

func (b *builder) ident(name, kind, detail, rank string) {
	b.add(Item{Label: name, Kind: kind, Detail: detail, InsertText: quoteIfNeeded(name, b.quoted), SortText: rank + strings.ToLower(name)})
}

// matchPrefix is a case-insensitive prefix match; Monaco applies its own
// fuzzy filter on top, so this only trims obviously unrelated items.
func matchPrefix(label, prefix string) bool {
	l, p := strings.ToLower(label), strings.ToLower(prefix)
	if strings.HasPrefix(l, p) {
		return true
	}
	// Match word starts inside multi-word keywords / snake_case names.
	for _, sep := range []string{" ", "_"} {
		for _, part := range strings.Split(l, sep)[1:] {
			if strings.HasPrefix(part, p) {
				return true
			}
		}
	}
	return false
}

var plainIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$]*$`)

// quoteIfNeeded returns the identifier quoted with backticks when it is not a
// plain identifier, is reserved, or the user started a quoted identifier.
func quoteIfNeeded(name string, forceQuote bool) string {
	if forceQuote || !plainIdent.MatchString(name) || IsReserved(name) || !utf8.ValidString(name) {
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	}
	return name
}

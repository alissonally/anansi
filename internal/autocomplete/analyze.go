// Package autocomplete provides context-aware SQL completion for the
// MySQL/MariaDB dialect.
//
// Strategy: rather than a full grammar (which cannot parse the incomplete
// statements present while typing) the analyzer works on the token stream of
// sqllex and tracks, per parenthesis level, the current clause and the table
// references (with aliases) introduced by FROM/JOIN/UPDATE/INTO. This is
// tolerant to errors after and before the cursor and resolves aliases across
// subquery scopes.
package autocomplete

import (
	"strings"

	"dbstudio/internal/sqllex"
)

// TableRef is a table (or derived table) visible at the cursor.
type TableRef struct {
	Database string `json:"database,omitempty"`
	Table    string `json:"table,omitempty"`
	Alias    string `json:"alias,omitempty"`
	// Subquery marks a derived table (FROM (SELECT ...) AS alias).
	Subquery bool `json:"subquery,omitempty"`
}

// Name returns the name used to qualify columns of the reference.
func (r TableRef) Name() string {
	if r.Alias != "" {
		return r.Alias
	}
	return r.Table
}

// Context is the result of analysing the text around the cursor.
type Context struct {
	// Clause is one of: start, select, from, join, afterTable, on, where,
	// groupby, orderby, having, set, into, insertColumns, values, use, call,
	// tableName, limit, ddl, other.
	Clause string
	// Prefix is the partial identifier being typed (unquoted).
	Prefix string
	// PrefixQuoted reports that the prefix started with a backtick.
	PrefixQuoted bool
	// ReplaceFrom is the byte offset where the completed text starts.
	ReplaceFrom int
	// Qualifiers are the identifiers before the dot(s): "db.tbl." => [db tbl].
	Qualifiers []string
	// Tables are the references visible at the cursor (innermost first).
	Tables []TableRef
	// InsertTable is the target of INSERT/REPLACE INTO, if any.
	InsertTable *TableRef
	// StatementHead is the first keyword of the statement (upper-case).
	StatementHead string
	// InString reports the cursor is inside a string literal or comment;
	// no completion should be offered.
	InString bool
}

type frame struct {
	id     int
	parent int
	clause string
	head   string // first keyword of the (sub)query
}

// Analyze inspects src with the cursor at byte offset pos.
func Analyze(src string, pos int) Context {
	if pos < 0 {
		pos = 0
	}
	if pos > len(src) {
		pos = len(src)
	}
	all := sqllex.Tokenize(src)

	// Statement boundaries: the ';' before and after the cursor.
	stStart, stEnd := 0, len(src)
	for _, t := range all {
		if t.Kind == sqllex.Punct && t.Text == ";" {
			if t.End <= pos {
				stStart = t.End
			} else if t.Start >= pos {
				stEnd = t.Start
				break
			}
		}
	}
	var toks []sqllex.Token
	for _, t := range all {
		if t.Start >= stStart && t.End <= stEnd {
			toks = append(toks, t)
		} else if t.Start < pos && t.End > pos && t.Start >= stStart {
			toks = append(toks, t)
		}
	}

	ctx := Context{ReplaceFrom: pos}

	// Token touching the cursor.
	var cur *sqllex.Token
	for i := range toks {
		if t := &toks[i]; t.Start < pos && pos <= t.End {
			cur = t
			break
		}
	}
	if cur != nil {
		switch cur.Kind {
		case sqllex.String, sqllex.Comment:
			// Inside a literal/comment unless the cursor is right after a
			// closed one (line comments end only at the newline).
			lineComment := cur.Kind == sqllex.Comment && !strings.HasPrefix(cur.Text, "/*")
			if cur.Open || pos < cur.End || lineComment {
				ctx.InString = true
				return ctx
			}
		case sqllex.Word:
			ctx.Prefix = src[cur.Start:pos]
			ctx.ReplaceFrom = cur.Start
		case sqllex.QuotedIdent:
			if cur.Open || pos < cur.End {
				ctx.Prefix = strings.ReplaceAll(src[cur.Start+1:pos], "``", "`")
				ctx.PrefixQuoted = true
				ctx.ReplaceFrom = cur.Start
			}
		case sqllex.Variable:
			ctx.InString = true // variables are not completed
			return ctx
		}
	}

	// Significant tokens before the prefix.
	var before []sqllex.Token
	for _, t := range toks {
		if t.End > ctx.ReplaceFrom {
			break
		}
		if t.Significant() {
			before = append(before, t)
		}
	}

	// Qualifiers: ident . ident . <cursor> (no spaces around the dots).
	i := len(before) - 1
	expect := ctx.ReplaceFrom
	for i >= 1 && before[i].Kind == sqllex.Dot && before[i].End == expect &&
		isIdentOrWord(before[i-1]) && before[i-1].End == before[i].Start {
		ctx.Qualifiers = append([]string{before[i-1].Ident()}, ctx.Qualifiers...)
		expect = before[i-1].Start
		i -= 2
	}
	if len(ctx.Qualifiers) > 0 {
		before = before[:i+1]
	}

	// Walk tokens before the cursor to find the clause of the innermost frame.
	frames := []frame{{id: 0, parent: -1, clause: "start"}}
	nextID := 1
	top := func() *frame { return &frames[len(frames)-1] }
	prevWord := ""
	for idx, t := range before {
		f := top()
		if t.Kind == sqllex.Punct {
			switch t.Text {
			case "(":
				nf := frame{id: nextID, parent: f.id, clause: f.clause}
				nextID++
				switch {
				case f.clause == "into" || (f.clause == "tableName" && f.head == "INSERT"):
					nf.clause = "insertColumns"
				case f.clause == "afterTable" && (f.head == "INSERT" || f.head == "REPLACE"):
					nf.clause = "insertColumns"
				case f.clause == "from" || f.clause == "join":
					nf.clause = "from" // derived table: expects SELECT
				case f.clause == "ddl":
					nf.clause = "ddl"
				}
				frames = append(frames, nf)
				prevWord = ""
				continue
			case ")":
				if len(frames) > 1 {
					closed := frames[len(frames)-1]
					frames = frames[:len(frames)-1]
					f = top()
					// After a derived table or INSERT column list.
					if f.clause == "from" || f.clause == "join" {
						f.clause = "afterTable"
					}
					if closed.clause == "insertColumns" {
						f.clause = "afterInsertColumns"
					}
				}
				prevWord = ""
				continue
			case ",":
				if f.clause == "afterTable" {
					f.clause = "from"
				}
				prevWord = ""
				continue
			}
		}
		if t.Kind != sqllex.Word {
			if isIdent(t) && (f.clause == "from" || f.clause == "join" || f.clause == "tableName" || f.clause == "into") {
				f.clause = "afterTable"
			}
			prevWord = ""
			continue
		}
		w := t.Upper()
		if f.head == "" {
			f.head = w
			if idx == 0 {
				ctx.StatementHead = w
			}
		}
		switch w {
		case "SELECT":
			f.clause = "select"
		case "FROM":
			if f.clause == "ddl" {
				break
			}
			f.clause = "from"
		case "JOIN", "STRAIGHT_JOIN":
			f.clause = "join"
		case "ON", "USING":
			if f.clause == "afterTable" || f.clause == "join" {
				f.clause = "on"
			}
		case "WHERE":
			f.clause = "where"
		case "BY":
			switch prevWord {
			case "GROUP":
				f.clause = "groupby"
			case "ORDER":
				f.clause = "orderby"
			case "PARTITION":
				f.clause = "orderby"
			}
		case "HAVING":
			f.clause = "having"
		case "LIMIT", "OFFSET":
			f.clause = "limit"
		case "UPDATE":
			if prevWord == "KEY" || prevWord == "FOR" {
				f.clause = "set" // ON DUPLICATE KEY UPDATE / FOR UPDATE
			} else {
				f.clause = "tableName"
			}
		case "SET":
			if f.head == "UPDATE" || f.head == "INSERT" || f.head == "REPLACE" {
				f.clause = "set"
			} else if f.head == "SET" {
				f.clause = "other"
			}
		case "INTO":
			if f.head == "INSERT" || f.head == "REPLACE" {
				f.clause = "into"
			} else {
				f.clause = "other"
			}
		case "VALUES", "VALUE":
			f.clause = "values"
		case "USE":
			f.clause = "use"
		case "CALL":
			f.clause = "call"
		case "DESCRIBE", "DESC", "EXPLAIN":
			if idx == 0 {
				f.clause = "tableName" // EXPLAIN SELECT ... switches to select below
			}
		case "TABLE":
			switch prevWord {
			case "CREATE", "TEMPORARY":
				f.clause = "ddl"
			case "ALTER", "DROP", "TRUNCATE", "OPTIMIZE", "ANALYZE", "CHECK", "REPAIR", "RENAME", "CHECKSUM", "LOCK", "EXISTS":
				f.clause = "tableName"
			default:
				if f.head == "SHOW" && prevWord == "CREATE" {
					f.clause = "tableName"
				}
			}
		case "TRUNCATE":
			f.clause = "tableName"
		case "TABLES":
			if f.head == "LOCK" {
				f.clause = "tableName"
			}
		case "CREATE", "ALTER":
			if f.clause == "start" {
				f.clause = "ddl"
			}
		case "AS":
			// keep clause; alias follows
		case "AND", "OR", "NOT":
			if f.clause == "afterTable" {
				f.clause = "on"
			}
		default:
			if !reservedWords[w] && (f.clause == "from" || f.clause == "join" || f.clause == "tableName" || f.clause == "into") {
				f.clause = "afterTable"
			}
		}
		if f.head == "SHOW" && (w == "FROM" || w == "IN") && (prevWord == "COLUMNS" || prevWord == "FIELDS" || prevWord == "INDEX" || prevWord == "INDEXES" || prevWord == "KEYS") {
			f.clause = "tableName"
		}
		if f.head == "SHOW" && w == "TABLES" {
			f.clause = "showTables"
		}
		if f.clause == "showTables" && (w == "FROM" || w == "IN") {
			f.clause = "use"
		}
		if f.head == "ALTER" && f.clause == "afterTable" {
			f.clause = "alterTable"
		}
		prevWord = w
	}
	// A prefix typed right after a table reference in FROM still names the
	// next keyword (or an alias), not a table.
	f := top()
	ctx.Clause = f.clause
	if ctx.StatementHead == "" && len(before) > 0 && before[0].Kind == sqllex.Word {
		ctx.StatementHead = before[0].Upper()
	}

	// Collect visible table references from the whole statement.
	chain := map[int]bool{}
	for _, fr := range frames {
		chain[fr.id] = true
	}
	refs := collectRefs(toks)
	// Innermost scopes first.
	for k := len(frames) - 1; k >= 0; k-- {
		for _, r := range refs {
			if r.frame == frames[k].id {
				ctx.Tables = append(ctx.Tables, r.ref)
			}
		}
	}
	for _, r := range refs {
		if r.insert && chain[r.frame] {
			rr := r.ref
			ctx.InsertTable = &rr
			break
		}
	}
	return ctx
}

func isIdent(t sqllex.Token) bool {
	return t.Kind == sqllex.QuotedIdent || (t.Kind == sqllex.Word && !reservedWords[t.Upper()])
}

type scopedRef struct {
	frame  int
	ref    TableRef
	insert bool
}

// collectRefs extracts table references of every query block. Frame ids are
// assigned in the same order as in Analyze (one per opening parenthesis).
func collectRefs(toks []sqllex.Token) []scopedRef {
	var sig []sqllex.Token
	for _, t := range toks {
		if t.Significant() {
			sig = append(sig, t)
		}
	}
	type fr struct {
		id      int
		inFrom  bool // expecting table references
		insert  bool
		derived bool // this frame is a derived table in FROM
	}
	stack := []fr{{id: 0}}
	nextID := 1
	var out []scopedRef
	head := ""
	// pendingDerived: a derived table just closed; its alias may follow.
	pendingDerivedFrame := -1

	for i := 0; i < len(sig); i++ {
		t := sig[i]
		f := &stack[len(stack)-1]
		if t.Kind == sqllex.Punct {
			switch t.Text {
			case "(":
				isDerived := f.inFrom
				stack = append(stack, fr{id: nextID, derived: isDerived})
				nextID++
				continue
			case ")":
				if len(stack) > 1 {
					closed := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					if closed.derived {
						pendingDerivedFrame = stack[len(stack)-1].id
						// Alias: [AS] name
						j := i + 1
						if j < len(sig) && sig[j].IsKeyword("AS") {
							j++
						}
						if j < len(sig) && isIdent(sig[j]) && !clauseWords[sig[j].Upper()] {
							out = append(out, scopedRef{frame: pendingDerivedFrame, ref: TableRef{Alias: sig[j].Ident(), Subquery: true}})
							i = j
						}
					}
				}
				continue
			case ",":
				continue
			}
		}
		if t.Kind == sqllex.Word {
			w := t.Upper()
			if head == "" {
				head = w
			}
			switch w {
			case "FROM", "JOIN", "STRAIGHT_JOIN":
				f.inFrom = true
				continue
			case "UPDATE":
				if i > 0 && (sig[i-1].IsKeyword("KEY") || sig[i-1].IsKeyword("FOR")) {
					continue
				}
				f.inFrom = true
				continue
			case "INTO":
				if head == "INSERT" || head == "REPLACE" {
					f.inFrom, f.insert = true, true
				}
				continue
			case "TABLE":
				if i > 0 && sig[i-1].IsKeyword("ALTER", "TRUNCATE", "DROP", "DESCRIBE") {
					f.inFrom = true
				}
				continue
			case "DESCRIBE", "DESC":
				if i == 0 {
					f.inFrom = true
				}
				continue
			}
			if clauseWords[w] || (reservedWords[w] && w != "LEFT" && w != "RIGHT") {
				if w != "INNER" && w != "LEFT" && w != "RIGHT" && w != "CROSS" && w != "NATURAL" && w != "OUTER" {
					f.inFrom = false
				}
				continue
			}
		}
		if !f.inFrom || !isIdent(t) {
			continue
		}
		// [db .] table [AS] [alias]
		ref := TableRef{Table: t.Ident()}
		j := i + 1
		if j+1 < len(sig) && sig[j].Kind == sqllex.Dot && isIdentOrWord(sig[j+1]) {
			ref.Database, ref.Table = ref.Table, sig[j+1].Ident()
			j += 2
		}
		if j < len(sig) && sig[j].IsKeyword("AS") {
			j++
		}
		if j < len(sig) && isIdent(sig[j]) && !clauseWords[sig[j].Upper()] {
			ref.Alias = sig[j].Ident()
			j++
		}
		out = append(out, scopedRef{frame: f.id, ref: ref, insert: f.insert})
		if f.insert {
			f.inFrom = false
		}
		i = j - 1
	}
	return out
}

// isIdentOrWord accepts reserved words after a dot (db.order is valid).
func isIdentOrWord(t sqllex.Token) bool {
	return t.Kind == sqllex.QuotedIdent || t.Kind == sqllex.Word
}

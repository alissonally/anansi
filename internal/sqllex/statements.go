package sqllex

import "strings"

// Statement is one SQL statement extracted from a script.
type Statement struct {
	// Text is the statement without the trailing delimiter, trimmed.
	Text string `json:"text"`
	// Start/End are byte offsets of Text inside the original script.
	Start int `json:"start"`
	End   int `json:"end"`
	// Line is the 1-based line on which the statement starts.
	Line int      `json:"line"`
	Kind Category `json:"kind"`
	// Destructive marks statements that can remove data or structure in bulk
	// (DROP, TRUNCATE, DELETE/UPDATE without WHERE).
	Destructive bool `json:"destructive"`
	// Reason is a human-readable explanation when Destructive is true.
	Reason string `json:"reason,omitempty"`
}

// Category groups statements by their effect.
type Category string

const (
	CatRead        Category = "read"        // SELECT, SHOW, DESCRIBE, EXPLAIN ...
	CatWrite       Category = "write"       // INSERT, UPDATE, DELETE ...
	CatDDL         Category = "ddl"         // CREATE, ALTER, DROP, TRUNCATE, RENAME
	CatTransaction Category = "transaction" // BEGIN, COMMIT, ROLLBACK ...
	CatSession     Category = "session"     // USE, SET
	CatOther       Category = "other"
)

// Split breaks a script into statements separated by ';' outside of strings,
// quoted identifiers and comments. Empty statements are dropped.
//
// The client-side DELIMITER command used by the mysql CLI is supported so that
// scripts containing stored procedures can be executed.
func Split(src string) []Statement {
	out, _, _ := split(src, ";", true)
	return out
}

// SplitPartial splits the beginning of a stream: it returns the complete
// statements (terminated by the delimiter), the offset where the unfinished
// remainder starts and the delimiter in effect there (DELIMITER commands are
// honoured). Feed the remainder, plus more input, to the next call.
func SplitPartial(src, delim string) (stmts []Statement, rest int, finalDelim string) {
	return split(src, delim, false)
}

func split(src, delim string, final bool) ([]Statement, int, string) {
	toks := Tokenize(src)
	var out []Statement
	segStart := 0
	i := 0
	lines := lineCounter{src: src, line: 1}
	emit := func(end int) {
		if st, ok := makeStatement(src, segStart, end, &lines); ok {
			out = append(out, st)
		}
	}
	atLineStart := true
	started := false // a statement has begun in the current segment
	for i < len(toks) {
		t := toks[i]
		// DELIMITER <x> must appear at the start of a line.
		if atLineStart && !started && t.IsKeyword("DELIMITER") {
			lineEnd := strings.IndexByte(src[t.End:], '\n')
			if lineEnd < 0 && !final {
				// The DELIMITER line is not complete yet.
				return out, t.Start, delim
			}
			if lineEnd < 0 {
				lineEnd = len(src)
			} else {
				lineEnd += t.End
			}
			if d := strings.TrimSpace(src[t.End:lineEnd]); d != "" {
				delim = d
			}
			for i < len(toks) && toks[i].Start < lineEnd {
				i++
			}
			segStart = lineEnd
			atLineStart = true
			continue
		}
		if t.Kind == Whitespace {
			if strings.Contains(t.Text, "\n") {
				atLineStart = true
			}
			i++
			continue
		}
		atLineStart = false
		if t.Kind == Comment {
			i++
			continue
		}
		at := -1
		if t.Kind != String && t.Kind != QuotedIdent {
			if k := strings.Index(t.Text, delim); k >= 0 {
				at = t.Start + k // e.g. "END$$" is a single word token
			} else if strings.HasPrefix(src[t.Start:], delim) {
				at = t.Start // delimiter spanning several tokens, e.g. "//"
			}
		}
		if at >= 0 {
			emit(at)
			end := at + len(delim)
			for i < len(toks) && toks[i].Start < end {
				i++
			}
			segStart = end
			started = false
			continue
		}
		started = true
		i++
	}
	if !final {
		return out, segStart, delim
	}
	emit(len(src))
	return out, len(src), delim
}

// lineCounter returns 1-based line numbers of increasing offsets in O(n)
// total (statements are emitted in order).
type lineCounter struct {
	src  string
	pos  int
	line int
}

func (c *lineCounter) at(off int) int {
	if off < c.pos {
		c.pos, c.line = 0, 1
	}
	c.line += strings.Count(c.src[c.pos:off], "\n")
	c.pos = off
	return c.line
}

func makeStatement(src string, start, end int, lines *lineCounter) (Statement, bool) {
	seg := src[start:end]
	// Trim leading whitespace and comments so offsets point to real SQL, but
	// keep comments inside the statement.
	toks := Tokenize(seg)
	first, last := -1, -1
	for i, t := range toks {
		if t.Significant() {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return Statement{}, false
	}
	s := start + toks[first].Start
	e := start + toks[last].End
	st := Statement{
		Text:  src[s:e],
		Start: s,
		End:   e,
		Line:  lines.at(s),
	}
	st.Kind, st.Destructive, st.Reason = Classify(st.Text)
	return st, true
}

// StatementAt returns the statement that contains the byte offset pos (or the
// closest preceding one when pos is between statements).
func StatementAt(stmts []Statement, pos int) (Statement, bool) {
	var best Statement
	found := false
	for _, s := range stmts {
		if pos >= s.Start && pos <= s.End {
			return s, true
		}
		if s.End <= pos {
			best, found = s, true
		}
	}
	if !found && len(stmts) > 0 {
		return stmts[0], true
	}
	return best, found
}

// Classify determines the category of a single statement and whether it is
// considered destructive.
func Classify(stmt string) (Category, bool, string) {
	// An executable comment heading the statement is classified by its body.
	for _, t := range Tokenize(stmt) {
		if !t.Significant() {
			continue
		}
		if t.Kind == Conditional {
			return Classify(ConditionalBody(t.Text) + " " + stmt[t.End:])
		}
		break
	}
	var words []Token
	depth := 0
	hasWhere := false
	hasLimit := false
	for _, t := range Tokenize(stmt) {
		if !t.Significant() {
			continue
		}
		if t.Kind == Punct {
			switch t.Text {
			case "(":
				depth++
			case ")":
				depth--
			}
		}
		if depth == 0 && t.IsKeyword("WHERE") {
			hasWhere = true
		}
		if depth == 0 && t.IsKeyword("LIMIT") {
			hasLimit = true
		}
		words = append(words, t)
	}
	if len(words) == 0 {
		return CatOther, false, ""
	}
	// Skip leading parentheses: (SELECT ...) UNION ...
	k := 0
	for k < len(words) && words[k].Text == "(" {
		k++
	}
	if k >= len(words) {
		return CatOther, false, ""
	}
	head := words[k].Upper()
	switch head {
	case "SELECT", "SHOW", "DESCRIBE", "DESC", "EXPLAIN", "VALUES", "TABLE", "HELP", "CHECKSUM", "ANALYZE", "CHECK":
		if head == "SELECT" && containsKeyword(words, "INTO") && containsKeyword(words, "OUTFILE", "DUMPFILE") {
			return CatWrite, false, ""
		}
		return CatRead, false, ""
	case "WITH":
		// CTE: the main statement follows the CTE list.
		for _, w := range words[k+1:] {
			if w.IsKeyword("UPDATE", "DELETE", "INSERT", "REPLACE") {
				return CatWrite, false, ""
			}
		}
		return CatRead, false, ""
	case "INSERT", "REPLACE", "LOAD", "CALL", "DO", "HANDLER", "IMPORT":
		return CatWrite, false, ""
	case "UPDATE":
		if !hasWhere {
			if hasLimit {
				return CatWrite, true, "UPDATE sem cláusula WHERE (limitado por LIMIT) altera linhas arbitrárias"
			}
			return CatWrite, true, "UPDATE sem cláusula WHERE altera todas as linhas da tabela"
		}
		return CatWrite, false, ""
	case "DELETE":
		if !hasWhere {
			return CatWrite, true, "DELETE sem cláusula WHERE remove todas as linhas da tabela"
		}
		return CatWrite, false, ""
	case "DROP":
		return CatDDL, true, "DROP remove objetos e seus dados permanentemente"
	case "TRUNCATE":
		return CatDDL, true, "TRUNCATE remove todas as linhas da tabela"
	case "ALTER":
		if containsKeyword(words, "DROP") {
			return CatDDL, true, "ALTER ... DROP remove colunas, índices ou partições"
		}
		return CatDDL, false, ""
	case "CREATE", "RENAME", "OPTIMIZE", "REPAIR":
		return CatDDL, false, ""
	case "BEGIN", "START", "COMMIT", "ROLLBACK", "SAVEPOINT", "RELEASE", "XA":
		return CatTransaction, false, ""
	case "USE", "SET":
		if head == "SET" && len(words) > k+1 && words[k+1].IsKeyword("TRANSACTION") {
			return CatTransaction, false, ""
		}
		return CatSession, false, ""
	case "SHUTDOWN":
		return CatOther, true, "SHUTDOWN encerra o servidor"
	}
	return CatOther, false, ""
}

func containsKeyword(words []Token, kw ...string) bool {
	for _, w := range words {
		if w.IsKeyword(kw...) {
			return true
		}
	}
	return false
}

// TransactionEffect reports how a statement affects explicit transaction
// state: +1 opens a transaction, -1 ends it, 0 no change. Implicit commits
// caused by DDL are reported as -1 as well.
func TransactionEffect(stmt string) int {
	var words []string
	for _, t := range Tokenize(stmt) {
		if t.Significant() {
			words = append(words, t.Upper())
			if len(words) == 3 {
				break
			}
		}
	}
	if len(words) == 0 {
		return 0
	}
	switch words[0] {
	case "BEGIN":
		return 1
	case "START":
		if len(words) > 1 && words[1] == "TRANSACTION" {
			return 1
		}
	case "COMMIT", "ROLLBACK":
		if len(words) > 1 && words[1] == "TO" {
			return 0 // ROLLBACK TO SAVEPOINT keeps the transaction open
		}
		if len(words) > 2 && words[1] == "WORK" && words[2] == "TO" {
			return 0
		}
		return -1
	case "CREATE", "ALTER", "DROP", "TRUNCATE", "RENAME":
		return -1
	}
	return 0
}

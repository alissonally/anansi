// Package gridexport writes tabular data in the formats of HeidiSQL's "Export
// grid rows" dialog: delimited text, Excel CSV, JSON, JSON Lines, XML, HTML,
// Markdown, LaTeX, Textile/Jira wiki, PHP arrays and SQL statements.
//
// Writers stream: Begin, then Row for every row, then End. Values are the
// JSON-safe cells produced by the query package (nil = SQL NULL, numbers,
// booleans, strings; binary data as "0x…" strings flagged by Column.Binary).
package gridexport

import (
	"bufio"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Column describes an exported column.
type Column struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Numeric bool   `json:"numeric"`
	Binary  bool   `json:"binary"`
	// Key marks columns identifying the row (SQL DELETE/UPDATE formats).
	Key bool `json:"key"`
	// AutoIncrement columns can be left out of INSERTs.
	AutoIncrement bool `json:"autoIncrement"`
}

// Format identifiers.
const (
	FormatCSV        = "csv"
	FormatExcelCSV   = "excel"
	FormatTSV        = "tsv"
	FormatJSON       = "json"
	FormatJSONLines  = "jsonl"
	FormatXML        = "xml"
	FormatHTML       = "html"
	FormatMarkdown   = "markdown"
	FormatLaTeX      = "latex"
	FormatTextile    = "textile"
	FormatJira       = "jira"
	FormatPHP        = "php"
	FormatSQLInsert  = "sql-insert"
	FormatSQLIgnore  = "sql-insert-ignore"
	FormatSQLReplace = "sql-replace"
	FormatSQLDelIns  = "sql-delete-insert"
	FormatSQLUpdate  = "sql-update"
)

// Options configure a writer. Zero values are sensible defaults.
type Options struct {
	Format string `json:"format"`
	// Delimited text (csv, tsv).
	Separator      string `json:"separator"` // default ";" for csv
	Encloser       string `json:"encloser"`  // default `"`
	LineTerminator string `json:"lineTerminator"`
	NullText       string `json:"nullText"` // default \N (csv/tsv), "" elsewhere
	Header         bool   `json:"header"`
	// RemoveLineBreaks replaces CR/LF inside values by spaces.
	RemoveLineBreaks bool `json:"removeLineBreaks"`
	// Table is the target table of SQL formats ("db.t" or "t", unquoted parts)
	// and the name used by JSON/XML/HTML/LaTeX.
	Table string `json:"table"`
	// Query, when set, is recorded in JSON/XML/HTML/SQL output.
	Query string `json:"query"`
	// SkipAutoIncrement leaves AUTO_INCREMENT columns out of SQL INSERTs.
	SkipAutoIncrement bool `json:"skipAutoIncrement"`
	// RowsPerInsert > 1 packs rows into multi-row INSERTs.
	RowsPerInsert int `json:"rowsPerInsert"`
}

// Writer streams rows in one format.
type Writer interface {
	Begin(cols []Column) error
	Row(values []any) error
	End() error
}

// Extension returns the usual file extension of a format.
func Extension(format string) string {
	switch format {
	case FormatCSV, FormatExcelCSV:
		return "csv"
	case FormatTSV:
		return "tsv"
	case FormatJSON:
		return "json"
	case FormatJSONLines:
		return "jsonl"
	case FormatXML:
		return "xml"
	case FormatHTML:
		return "html"
	case FormatMarkdown:
		return "md"
	case FormatLaTeX:
		return "tex"
	case FormatTextile, FormatJira:
		return "txt"
	case FormatPHP:
		return "php"
	}
	if strings.HasPrefix(format, "sql") {
		return "sql"
	}
	return "txt"
}

// New returns a writer for opts.Format writing to w.
func New(w io.Writer, opts Options) (Writer, error) {
	bw := bufio.NewWriterSize(w, 64<<10)
	b := base{w: bw, opts: opts}
	switch opts.Format {
	case FormatCSV, FormatTSV, FormatExcelCSV:
		return &delimited{base: b}, nil
	case FormatJSON:
		return &jsonWriter{base: b}, nil
	case FormatJSONLines:
		return &jsonWriter{base: b, lines: true}, nil
	case FormatXML:
		return &xmlWriter{base: b}, nil
	case FormatHTML:
		return &htmlWriter{base: b}, nil
	case FormatMarkdown, FormatTextile, FormatJira:
		return &wikiWriter{base: b}, nil
	case FormatLaTeX:
		return &latexWriter{base: b}, nil
	case FormatPHP:
		return &phpWriter{base: b}, nil
	case FormatSQLInsert, FormatSQLIgnore, FormatSQLReplace, FormatSQLDelIns, FormatSQLUpdate:
		return &sqlWriter{base: b}, nil
	}
	return nil, fmt.Errorf("formato de exportação desconhecido: %q", opts.Format)
}

type base struct {
	w    *bufio.Writer
	opts Options
	cols []Column
}

func (b *base) ws(s string) error {
	_, err := b.w.WriteString(s)
	return err
}

func (b *base) flush() error { return b.w.Flush() }

// text renders a value as plain text ("" for NULL).
func text(v any) (string, bool) {
	switch x := v.(type) {
	case nil:
		return "", true
	case string:
		return x, false
	case bool:
		if x {
			return "1", false
		}
		return "0", false
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1e15 {
			return strconv.FormatInt(int64(x), 10), false
		}
		return strconv.FormatFloat(x, 'g', -1, 64), false
	case json.Number:
		return x.String(), false
	default:
		return fmt.Sprint(x), false
	}
}

func (b *base) cell(v any) (string, bool) {
	s, isNull := text(v)
	if b.opts.RemoveLineBreaks {
		s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
	}
	return s, isNull
}

func (b *base) eol() string {
	if b.opts.LineTerminator != "" {
		return b.opts.LineTerminator
	}
	return "\n"
}

// ---- delimited text --------------------------------------------------------------

type delimited struct {
	base
	sep, enc, null string
}

func (d *delimited) Begin(cols []Column) error {
	d.cols = cols
	d.sep, d.enc, d.null = d.opts.Separator, d.opts.Encloser, d.opts.NullText
	switch d.opts.Format {
	case FormatTSV:
		if d.sep == "" {
			d.sep = "\t"
		}
	case FormatExcelCSV:
		// Excel expects a BOM to detect UTF-8, CRLF and its regional
		// separator (";" in pt-BR).
		if d.sep == "" {
			d.sep = ";"
		}
		if d.enc == "" {
			d.enc = `"`
		}
		if d.opts.LineTerminator == "" {
			d.opts.LineTerminator = "\r\n"
		}
		if err := d.ws("\ufeff"); err != nil {
			return err
		}
	default:
		if d.sep == "" {
			d.sep = ";"
		}
		if d.enc == "" {
			d.enc = `"`
		}
	}
	if d.null == "" && d.opts.Format != FormatExcelCSV {
		d.null = `\N`
	}
	if !d.opts.Header {
		return nil
	}
	names := make([]any, len(cols))
	for i, c := range cols {
		names[i] = c.Name
	}
	return d.line(names, true)
}

func (d *delimited) field(s string) string {
	if d.enc == "" {
		return s
	}
	if d.opts.Format == FormatTSV && !strings.ContainsAny(s, "\t\r\n"+d.enc) {
		return s // TSV: quote only when needed
	}
	return d.enc + strings.ReplaceAll(s, d.enc, d.enc+d.enc) + d.enc
}

func (d *delimited) line(values []any, header bool) error {
	parts := make([]string, len(values))
	for i, v := range values {
		s, isNull := d.cell(v)
		if isNull && !header {
			parts[i] = d.null
			continue
		}
		parts[i] = d.field(s)
	}
	return d.ws(strings.Join(parts, d.sep) + d.eol())
}

func (d *delimited) Row(values []any) error { return d.line(values, false) }
func (d *delimited) End() error             { return d.flush() }

// ---- JSON --------------------------------------------------------------------------

type jsonWriter struct {
	base
	lines bool
	n     int
}

func (j *jsonWriter) Begin(cols []Column) error {
	j.cols = cols
	if j.lines {
		return nil
	}
	head := map[string]any{}
	if j.opts.Query != "" {
		head["query"] = j.opts.Query
	}
	if j.opts.Table != "" {
		head["table"] = j.opts.Table
	}
	b, _ := json.Marshal(head)
	// Reopen the object to append the rows array.
	s := strings.TrimSuffix(string(b), "}")
	if len(head) > 0 {
		s += ","
	}
	return j.ws(s + "\n  \"rows\": [")
}

func (j *jsonWriter) obj(values []any) ([]byte, error) {
	var sb strings.Builder
	sb.WriteByte('{')
	for i, c := range j.cols {
		if i > 0 {
			sb.WriteString(", ")
		}
		k, _ := json.Marshal(c.Name)
		sb.Write(k)
		sb.WriteString(": ")
		var v any
		if i < len(values) {
			v = values[i]
		}
		if s, ok := v.(string); ok && c.Numeric {
			if _, err := strconv.ParseFloat(s, 64); err == nil {
				sb.WriteString(s) // big integers / decimals keep full precision
				continue
			}
		}
		if s, ok := v.(string); ok && j.opts.RemoveLineBreaks {
			v = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
		}
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		sb.Write(b)
	}
	sb.WriteByte('}')
	return []byte(sb.String()), nil
}

func (j *jsonWriter) Row(values []any) error {
	b, err := j.obj(values)
	if err != nil {
		return err
	}
	if j.lines {
		return j.ws(string(b) + "\n")
	}
	sep := ","
	if j.n == 0 {
		sep = ""
	}
	j.n++
	return j.ws(sep + "\n    " + string(b))
}

func (j *jsonWriter) End() error {
	if !j.lines {
		if err := j.ws("\n  ]\n}\n"); err != nil {
			return err
		}
	}
	return j.flush()
}

// ---- XML ---------------------------------------------------------------------------

type xmlWriter struct{ base }

func xmlEsc(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '<':
			b.WriteString("&lt;")
		case r == '>':
			b.WriteString("&gt;")
		case r == '&':
			b.WriteString("&amp;")
		case r == '"':
			b.WriteString("&quot;")
		case r == '\t' || r == '\n' || r == '\r' || r >= 0x20 && r != 0xFFFE && r != 0xFFFF:
			b.WriteRune(r)
		default:
			fmt.Fprintf(&b, "&#x%X;", r) // control characters are not allowed raw
		}
	}
	return b.String()
}

func (x *xmlWriter) Begin(cols []Column) error {
	x.cols = cols
	name := x.opts.Table
	if name == "" {
		name = "resultado"
	}
	s := "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<table_data name=\"" + xmlEsc(name) + "\""
	if x.opts.Query != "" {
		s += " query=\"" + xmlEsc(x.opts.Query) + "\""
	}
	return x.ws(s + ">\n")
}

func (x *xmlWriter) Row(values []any) error {
	var b strings.Builder
	b.WriteString("  <row>\n")
	for i, c := range x.cols {
		var v any
		if i < len(values) {
			v = values[i]
		}
		s, isNull := x.cell(v)
		if isNull {
			b.WriteString("    <field name=\"" + xmlEsc(c.Name) + "\" xsi:nil=\"true\" />\n")
			continue
		}
		b.WriteString("    <field name=\"" + xmlEsc(c.Name) + "\">" + xmlEsc(s) + "</field>\n")
	}
	b.WriteString("  </row>\n")
	return x.ws(b.String())
}

func (x *xmlWriter) End() error {
	if err := x.ws("</table_data>\n"); err != nil {
		return err
	}
	return x.flush()
}

// ---- HTML ----------------------------------------------------------------------------

type htmlWriter struct{ base }

func (h *htmlWriter) Begin(cols []Column) error {
	h.cols = cols
	title := h.opts.Table
	if title == "" {
		title = "Resultado"
	}
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"pt-BR\">\n<head>\n<meta charset=\"utf-8\">\n<title>" + html.EscapeString(title) + "</title>\n")
	b.WriteString("<style>body{font-family:system-ui,sans-serif;font-size:13px}table{border-collapse:collapse}th,td{border:1px solid #ccc;padding:3px 6px;vertical-align:top}th{background:#f2f2f2;text-align:left}td.num{text-align:right}td.null{color:#999;font-style:italic}caption{text-align:left;font-weight:600;margin-bottom:6px}</style>\n</head>\n<body>\n")
	if h.opts.Query != "" {
		b.WriteString("<p><code>" + html.EscapeString(h.opts.Query) + "</code></p>\n")
	}
	b.WriteString("<table>\n<caption>" + html.EscapeString(title) + "</caption>\n<thead><tr>")
	for _, c := range cols {
		b.WriteString("<th>" + html.EscapeString(c.Name) + "</th>")
	}
	b.WriteString("</tr></thead>\n<tbody>\n")
	return h.ws(b.String())
}

func (h *htmlWriter) Row(values []any) error {
	var b strings.Builder
	b.WriteString("<tr>")
	for i, c := range h.cols {
		var v any
		if i < len(values) {
			v = values[i]
		}
		s, isNull := h.cell(v)
		switch {
		case isNull:
			b.WriteString("<td class=\"null\">NULL</td>")
		case c.Numeric:
			b.WriteString("<td class=\"num\">" + html.EscapeString(s) + "</td>")
		default:
			b.WriteString("<td>" + strings.ReplaceAll(html.EscapeString(s), "\n", "<br>") + "</td>")
		}
	}
	b.WriteString("</tr>\n")
	return h.ws(b.String())
}

func (h *htmlWriter) End() error {
	if err := h.ws("</tbody>\n</table>\n</body>\n</html>\n"); err != nil {
		return err
	}
	return h.flush()
}

// ---- Markdown / Textile / Jira ------------------------------------------------------

type wikiWriter struct{ base }

func (m *wikiWriter) esc(s string) string {
	s = strings.NewReplacer("\r\n", "<br>", "\n", "<br>", "\r", "<br>").Replace(s)
	return strings.ReplaceAll(s, "|", `\|`)
}

func (m *wikiWriter) Begin(cols []Column) error {
	m.cols = cols
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = m.esc(c.Name)
	}
	switch m.opts.Format {
	case FormatTextile:
		return m.ws("|_. " + strings.Join(names, " |_. ") + " |\n")
	case FormatJira:
		return m.ws("|| " + strings.Join(names, " || ") + " ||\n")
	}
	align := make([]string, len(cols))
	for i, c := range cols {
		if c.Numeric {
			align[i] = "---:"
		} else {
			align[i] = "---"
		}
	}
	return m.ws("| " + strings.Join(names, " | ") + " |\n| " + strings.Join(align, " | ") + " |\n")
}

func (m *wikiWriter) Row(values []any) error {
	parts := make([]string, len(m.cols))
	for i := range m.cols {
		var v any
		if i < len(values) {
			v = values[i]
		}
		s, isNull := m.cell(v)
		if isNull {
			s = "NULL"
		}
		parts[i] = m.esc(s)
	}
	return m.ws("| " + strings.Join(parts, " | ") + " |\n")
}

func (m *wikiWriter) End() error { return m.flush() }

// ---- LaTeX ---------------------------------------------------------------------------

type latexWriter struct{ base }

var latexRepl = strings.NewReplacer(`\`, `\textbackslash{}`, "&", `\&`, "%", `\%`, "$", `\$`, "#", `\#`, "_", `\_`, "{", `\{`, "}", `\}`, "~", `\textasciitilde{}`, "^", `\textasciicircum{}`, "\n", ` \newline `)

func (l *latexWriter) Begin(cols []Column) error {
	l.cols = cols
	spec := make([]string, len(cols))
	names := make([]string, len(cols))
	for i, c := range cols {
		spec[i] = "l"
		if c.Numeric {
			spec[i] = "r"
		}
		names[i] = `\textbf{` + latexRepl.Replace(c.Name) + `}`
	}
	return l.ws(`\begin{tabular}{|` + strings.Join(spec, "|") + "|}\n\\hline\n" + strings.Join(names, " & ") + " \\\\\n\\hline\n")
}

func (l *latexWriter) Row(values []any) error {
	parts := make([]string, len(l.cols))
	for i := range l.cols {
		var v any
		if i < len(values) {
			v = values[i]
		}
		s, isNull := l.cell(v)
		if isNull {
			s = "NULL"
		}
		parts[i] = latexRepl.Replace(s)
	}
	return l.ws(strings.Join(parts, " & ") + " \\\\\n")
}

func (l *latexWriter) End() error {
	if err := l.ws("\\hline\n\\end{tabular}\n"); err != nil {
		return err
	}
	return l.flush()
}

// ---- PHP -------------------------------------------------------------------------------

type phpWriter struct{ base }

func phpString(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, "'", `\'`).Replace(s) + "'"
}

func (p *phpWriter) Begin(cols []Column) error {
	p.cols = cols
	name := p.opts.Table
	if name == "" {
		name = "resultado"
	}
	var b strings.Builder
	for _, r := range name {
		if r < 128 && (r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return p.ws("<?php\n$" + b.String() + " = array(\n")
}

func (p *phpWriter) Row(values []any) error {
	var b strings.Builder
	b.WriteString("  array(")
	for i, c := range p.cols {
		if i > 0 {
			b.WriteString(", ")
		}
		var v any
		if i < len(values) {
			v = values[i]
		}
		b.WriteString(phpString(c.Name) + " => ")
		s, isNull := p.cell(v)
		switch {
		case isNull:
			b.WriteString("NULL")
		case c.Numeric && isNumber(s):
			b.WriteString(s)
		default:
			b.WriteString(phpString(s))
		}
	}
	b.WriteString("),\n")
	return p.ws(b.String())
}

func (p *phpWriter) End() error {
	if err := p.ws(");\n"); err != nil {
		return err
	}
	return p.flush()
}

func isNumber(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil && s != "" && !strings.ContainsAny(s, "xXnN")
}

// ---- SQL -------------------------------------------------------------------------------

// QuoteIdent quotes a MySQL identifier.
func QuoteIdent(s string) string { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }

// QuoteString renders a MySQL string literal with backslash escapes, as
// mysqldump does (the dump header resets SQL_MODE so they are understood).
func QuoteString(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('\'')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case 0:
			b.WriteString(`\0`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`\'`)
		case 0x1A:
			b.WriteString(`\Z`)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('\'')
	return b.String()
}

// Literal renders a cell value as an SQL literal for col.
func Literal(v any, c Column) string {
	s, isNull := text(v)
	switch {
	case isNull:
		return "NULL"
	case c.Binary && strings.HasPrefix(s, "0x"):
		if len(s) == 2 {
			return "''"
		}
		return "_binary " + s
	case c.Numeric && isNumber(s):
		return s
	case !utf8.ValidString(s):
		return "0x" + fmt.Sprintf("%X", s)
	}
	return QuoteString(s)
}

// TableRef renders "db.t" (unquoted parts separated by the first dot) as
// `db`.`t`.
func TableRef(name string) string {
	if name == "" {
		return "`tabela`"
	}
	if i := strings.IndexByte(name, '.'); i > 0 && !strings.HasPrefix(name, "`") {
		return QuoteIdent(name[:i]) + "." + QuoteIdent(name[i+1:])
	}
	return QuoteIdent(name)
}

type sqlWriter struct {
	base
	table   string
	insCols []int
	pending []string
}

func (s *sqlWriter) Begin(cols []Column) error {
	s.cols = cols
	s.table = TableRef(s.opts.Table)
	for i, c := range cols {
		if s.opts.SkipAutoIncrement && c.AutoIncrement {
			continue
		}
		s.insCols = append(s.insCols, i)
	}
	if s.opts.Query != "" {
		if err := s.ws("-- " + strings.ReplaceAll(s.opts.Query, "\n", "\n-- ") + "\n"); err != nil {
			return err
		}
	}
	return nil
}

func (s *sqlWriter) colList() string {
	names := make([]string, len(s.insCols))
	for i, ci := range s.insCols {
		names[i] = QuoteIdent(s.cols[ci].Name)
	}
	return strings.Join(names, ", ")
}

func (s *sqlWriter) where(values []any) string {
	var parts []string
	for i, c := range s.cols {
		if !c.Key {
			continue
		}
		var v any
		if i < len(values) {
			v = values[i]
		}
		if v == nil {
			parts = append(parts, QuoteIdent(c.Name)+" IS NULL")
		} else {
			parts = append(parts, QuoteIdent(c.Name)+"="+Literal(v, c))
		}
	}
	if len(parts) == 0 {
		// Without key columns every column identifies the row.
		for i, c := range s.cols {
			var v any
			if i < len(values) {
				v = values[i]
			}
			if v == nil {
				parts = append(parts, QuoteIdent(c.Name)+" IS NULL")
			} else {
				parts = append(parts, QuoteIdent(c.Name)+"="+Literal(v, c))
			}
		}
	}
	return strings.Join(parts, " AND ")
}

func (s *sqlWriter) Row(values []any) error {
	val := func(i int) any {
		if i < len(values) {
			return values[i]
		}
		return nil
	}
	if s.opts.Format == FormatSQLUpdate {
		var sets []string
		for i, c := range s.cols {
			if c.Key {
				continue
			}
			sets = append(sets, QuoteIdent(c.Name)+"="+Literal(val(i), c))
		}
		if len(sets) == 0 {
			return nil
		}
		return s.ws("UPDATE " + s.table + " SET " + strings.Join(sets, ", ") + " WHERE " + s.where(values) + ";\n")
	}
	lits := make([]string, len(s.insCols))
	for i, ci := range s.insCols {
		lits[i] = Literal(val(ci), s.cols[ci])
	}
	tuple := "(" + strings.Join(lits, ", ") + ")"
	if s.opts.Format == FormatSQLDelIns {
		if err := s.ws("DELETE FROM " + s.table + " WHERE " + s.where(values) + ";\n"); err != nil {
			return err
		}
	}
	if s.opts.RowsPerInsert > 1 && s.opts.Format != FormatSQLDelIns {
		s.pending = append(s.pending, tuple)
		if len(s.pending) >= s.opts.RowsPerInsert {
			return s.flushPending()
		}
		return nil
	}
	return s.ws(s.verb() + " " + s.table + " (" + s.colList() + ") VALUES " + tuple + ";\n")
}

func (s *sqlWriter) verb() string {
	switch s.opts.Format {
	case FormatSQLIgnore:
		return "INSERT IGNORE INTO"
	case FormatSQLReplace:
		return "REPLACE INTO"
	}
	return "INSERT INTO"
}

func (s *sqlWriter) flushPending() error {
	if len(s.pending) == 0 {
		return nil
	}
	err := s.ws(s.verb() + " " + s.table + " (" + s.colList() + ") VALUES\n\t" + strings.Join(s.pending, ",\n\t") + ";\n")
	s.pending = s.pending[:0]
	return err
}

func (s *sqlWriter) End() error {
	if err := s.flushPending(); err != nil {
		return err
	}
	return s.flush()
}

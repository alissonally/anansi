package api

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"anansi/internal/apperr"
	"anansi/internal/autocomplete"
	"anansi/internal/config"
	"anansi/internal/database"
	"anansi/internal/query"
	"anansi/internal/sqllex"
)

// ---- editable query results ---------------------------------------------------

// EditableInfo tells the SQL tab how to edit a result like the table grid.
type EditableInfo struct {
	Database string            `json:"database"`
	Table    string            `json:"table"`
	Key      database.TableKey `json:"key"`
	// Columns holds, per result column, the table column it shows ("" for
	// computed values, which stay read-only).
	Columns []string              `json:"columns"`
	Info    []database.ColumnInfo `json:"info"`
}

// EditableResult reports whether the result of sql (with resultColumns, as
// returned by Execute) can be edited: a SELECT from a single table that has a
// primary or unique NOT NULL key and whose key columns are all in the result.
// It returns nil when editing is not possible.
func (a *App) EditableResult(id, sql, defaultDB string, resultColumns []string) (*EditableInfo, error) {
	src, ok := sqllex.SingleTableSource(sql)
	if !ok {
		return nil, nil
	}
	db := src.Database
	if db == "" {
		db = defaultDB
	}
	if db == "" {
		return nil, nil
	}
	_, g, err := a.dataSQL(id)
	if err != nil {
		return nil, err
	}
	cols, key, err := a.tableMeta(id, db, src.Table, g)
	if err != nil || key.Kind == "none" {
		return nil, nil // unknown table, a view, or no reliable key
	}
	byName := map[string]string{}
	var visible []string
	for _, c := range cols {
		byName[strings.ToLower(c.Name)] = c.Name
		if !strings.Contains(strings.ToUpper(c.Extra), "INVISIBLE") {
			visible = append(visible, c.Name)
		}
	}
	matchesTable := func(q string) bool {
		return q == "" || strings.EqualFold(q, src.Table) || (src.Alias != "" && strings.EqualFold(q, src.Alias))
	}
	var mapping []string
	for _, it := range src.Items {
		switch {
		case it.Star:
			if !matchesTable(it.Qualifier) {
				return nil, nil
			}
			mapping = append(mapping, visible...)
		case it.Column != "" && matchesTable(it.Qualifier):
			mapping = append(mapping, byName[strings.ToLower(it.Column)]) // "" if not a column
		default:
			mapping = append(mapping, "")
		}
	}
	if len(mapping) != len(resultColumns) {
		return nil, nil
	}
	for i, m := range mapping {
		if m != "" && !strings.EqualFold(m, resultColumns[i]) {
			mapping[i] = ""
		}
	}
	for _, k := range key.Columns {
		found := false
		for _, m := range mapping {
			if strings.EqualFold(m, k) {
				found = true
			}
		}
		if !found {
			return nil, nil
		}
	}
	return &EditableInfo{Database: db, Table: src.Table, Key: key, Columns: mapping, Info: cols}, nil
}

// ---- SQL help (server HELP tables) ----------------------------------------------

// HelpItem is a topic or category listed by HELP.
type HelpItem struct {
	Name       string `json:"name"`
	IsCategory bool   `json:"isCategory"`
}

// HelpResult is the answer of the server HELP statement.
type HelpResult struct {
	// Kind: "topic" (Description/Example set), "list" (Items) or "none".
	Kind        string     `json:"kind"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Example     string     `json:"example,omitempty"`
	Items       []HelpItem `json:"items,omitempty"`
}

// SQLHelp queries the server's help for a topic, category or pattern
// ("contents" lists the top categories; % works as a wildcard).
func (a *App) SQLHelp(id, topic string) (*HelpResult, error) {
	sess, err := a.session(id)
	if err != nil {
		return nil, err
	}
	g, ok := sess.Driver.(database.ObjectSQL)
	if !ok {
		return nil, apperr.New(apperr.CodeValidation, "ajuda indisponível para este banco de dados")
	}
	topic = strings.TrimSpace(topic)
	if topic == "" {
		topic = "contents"
	}
	if utf8.RuneCountInString(topic) > 200 {
		return nil, apperr.New(apperr.CodeValidation, "termo de ajuda longo demais")
	}
	res, err := a.queries.Browse(context.Background(), query.BrowseRequest{ConnectionID: id, SQL: "HELP " + g.QuoteString(topic), MaxRows: 2000, Timeout: 20 * time.Second})
	if err != nil {
		return nil, wrap(err)
	}
	out := &HelpResult{Kind: "none", Name: topic}
	if len(res.Rows) == 0 {
		return out, nil
	}
	names := make([]string, len(res.Columns))
	for i, c := range res.Columns {
		names[i] = strings.ToLower(c.Name)
	}
	idx := func(n string) int {
		for i, x := range names {
			if x == n {
				return i
			}
		}
		return -1
	}
	str := func(v any) string {
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
	if d := idx("description"); d >= 0 && len(res.Rows) == 1 {
		out.Kind = "topic"
		out.Name = str(res.Rows[0][idx("name")])
		out.Description = str(res.Rows[0][d])
		if e := idx("example"); e >= 0 {
			out.Example = str(res.Rows[0][e])
		}
		return out, nil
	}
	out.Kind = "list"
	n, c := idx("name"), idx("is_it_category")
	for _, r := range res.Rows {
		if n < 0 {
			break
		}
		it := HelpItem{Name: str(r[n])}
		if c >= 0 {
			it.IsCategory = strings.EqualFold(str(r[c]), "Y")
		}
		out.Items = append(out.Items, it)
	}
	return out, nil
}

// ---- snippets (.sql files in the config directory, like HeidiSQL) ---------------

// Snippet is a reusable SQL file.
type Snippet struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

const maxSnippetBytes = 2 << 20

func (a *App) snippetsDir() (string, error) {
	dir := filepath.Join(a.configDir, "snippets")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func snippetFile(dir, name string) (string, error) {
	n := strings.TrimSpace(strings.TrimSuffix(name, ".sql"))
	if n == "" || utf8.RuneCountInString(n) > 100 || strings.ContainsAny(n, `/\:*?"<>|`) || strings.HasPrefix(n, ".") || strings.ContainsRune(n, 0) {
		return "", apperr.New(apperr.CodeValidation, "nome de snippet inválido (não use / \\ : * ? \" < > |)")
	}
	return filepath.Join(dir, n+".sql"), nil
}

// ListSnippets lists the snippets.
func (a *App) ListSnippets() ([]Snippet, error) {
	dir, err := a.snippetsDir()
	if err != nil {
		return nil, wrap(err)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, wrap(err)
	}
	out := []Snippet{}
	for _, e := range ents {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".sql") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, Snippet{Name: strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())), Size: info.Size(), Modified: info.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

// ReadSnippet returns the content of a snippet.
func (a *App) ReadSnippet(name string) (string, error) {
	dir, err := a.snippetsDir()
	if err != nil {
		return "", wrap(err)
	}
	p, err := snippetFile(dir, name)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(p)
	if err != nil {
		return "", wrap(err)
	}
	if st.Size() > maxSnippetBytes {
		return "", apperr.New(apperr.CodeValidation, "snippet grande demais (limite de 2 MB)")
	}
	b, err := os.ReadFile(p)
	return string(b), wrap(err)
}

// SaveSnippet creates or overwrites a snippet.
func (a *App) SaveSnippet(name, content string) error {
	if len(content) > maxSnippetBytes {
		return apperr.New(apperr.CodeValidation, "snippet grande demais (limite de 2 MB)")
	}
	dir, err := a.snippetsDir()
	if err != nil {
		return wrap(err)
	}
	p, err := snippetFile(dir, name)
	if err != nil {
		return err
	}
	return wrap(config.WriteFileAtomic(p, []byte(content), 0o600))
}

// RenameSnippet renames a snippet; it refuses to overwrite another one.
func (a *App) RenameSnippet(from, to string) error {
	dir, err := a.snippetsDir()
	if err != nil {
		return wrap(err)
	}
	src, err := snippetFile(dir, from)
	if err != nil {
		return err
	}
	dst, err := snippetFile(dir, to)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		return apperr.New(apperr.CodeValidation, "já existe um snippet com esse nome")
	}
	return wrap(os.Rename(src, dst))
}

// DeleteSnippet removes a snippet.
func (a *App) DeleteSnippet(name string) error {
	dir, err := a.snippetsDir()
	if err != nil {
		return wrap(err)
	}
	p, err := snippetFile(dir, name)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return wrap(err)
	}
	return nil
}

// OpenSnippetsFolder opens the snippets directory in the file manager.
func (a *App) OpenSnippetsFolder() error {
	dir, err := a.snippetsDir()
	if err != nil {
		return wrap(err)
	}
	return wrap(exec.Command("xdg-open", dir).Start())
}

// Vocabulary lists SQL keywords and functions for the helpers panel.
type Vocabulary struct {
	Keywords  []string                `json:"keywords"`
	Functions []autocomplete.Function `json:"functions"`
}

// SQLVocabulary returns the keywords and functions known to the editor.
func (a *App) SQLVocabulary() Vocabulary {
	k, f := autocomplete.Vocabulary()
	return Vocabulary{Keywords: k, Functions: f}
}

package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"anansi/internal/apperr"
	"anansi/internal/connections"
	"anansi/internal/database"
	"anansi/internal/query"
)

// maxFullCell caps values read in full (cell viewer/editor of truncated
// cells).
const maxFullCell = 16 << 20

// TableData is one page of the table data grid.
type TableData struct {
	Columns        []query.Column        `json:"columns"`
	Partial        []bool                `json:"partial"`
	Key            database.TableKey     `json:"key"`
	Rows           [][]any               `json:"rows"`
	TruncatedCells [][2]int              `json:"truncatedCells,omitempty"`
	HasMore        bool                  `json:"hasMore"`
	SQL            string                `json:"sql"`
	DurationMs     float64               `json:"durationMs"`
	Info           []database.ColumnInfo `json:"info"`
}

func (a *App) dataSQL(id string) (*connections.Session, database.DataSQL, error) {
	sess, err := a.session(id)
	if err != nil {
		return nil, nil, err
	}
	g, ok := sess.Driver.(database.DataSQL)
	if !ok {
		return nil, nil, apperr.New(apperr.CodeValidation, "edição de dados não suportada por este banco de dados")
	}
	return sess, g, nil
}

// tableMeta returns columns, indexes and the row key of a table.
func (a *App) tableMeta(id, db, table string, g database.DataSQL) ([]database.ColumnInfo, database.TableKey, error) {
	ctx, cancel := metaCtx()
	defer cancel()
	cols, err := a.meta.Columns(ctx, id, db, table)
	if err != nil {
		return nil, database.TableKey{}, err
	}
	if len(cols) == 0 {
		// The cache may predate the table (created in another client).
		a.meta.Invalidate(id, db)
		if cols, err = a.meta.Columns(ctx, id, db, table); err != nil {
			return nil, database.TableKey{}, err
		}
	}
	if len(cols) == 0 {
		return nil, database.TableKey{}, apperr.New(apperr.CodeNotFound, fmt.Sprintf("tabela %q não encontrada em %q", table, db))
	}
	idx, err := a.meta.Indexes(ctx, id, db, table)
	if err != nil {
		return nil, database.TableKey{}, err
	}
	return cols, g.FindKey(cols, idx), nil
}

// BrowseTable reads one page of a table for the data grid. token identifies
// the grid (a new request cancels the previous one of the same grid).
func (a *App) BrowseTable(token, id string, spec database.BrowseSpec) (*TableData, error) {
	_, g, err := a.dataSQL(id)
	if err != nil {
		return nil, err
	}
	cols, key, err := a.tableMeta(id, spec.Database, spec.Table, g)
	if err != nil {
		return nil, wrap(err)
	}
	if spec.Limit <= 0 || spec.Limit > 1_000_000 {
		spec.Limit = 1000
	}
	limit := spec.Limit
	spec.Limit = limit + 1 // one extra row tells whether there is more
	plan, err := g.SelectSQL(spec, cols, key)
	if err != nil {
		return nil, wrap(err)
	}
	res, err := a.queries.Browse(context.Background(), query.BrowseRequest{Token: token, ConnectionID: id, SQL: plan.SQL, MaxRows: limit})
	if err != nil {
		return nil, wrap(err)
	}
	n := len(plan.Columns)
	out := &TableData{Partial: plan.Partial, Key: key, HasMore: res.HasMore, SQL: plan.SQL, DurationMs: res.DurationMs, Info: cols}
	if len(res.Columns) < n {
		return nil, apperr.New(apperr.CodeInternal, "resultado inesperado do servidor")
	}
	out.Columns = res.Columns[:n]
	// Map truncation flags (one extra column per partial column).
	var partialIdx []int
	for i, p := range plan.Partial {
		if p {
			partialIdx = append(partialIdx, i)
		}
	}
	out.Rows = make([][]any, len(res.Rows))
	for r, row := range res.Rows {
		out.Rows[r] = row[:n]
		for j, ci := range partialIdx {
			if v := row[n+j]; v != nil && fmt.Sprint(v) != "0" {
				out.TruncatedCells = append(out.TruncatedCells, [2]int{r, ci})
			}
		}
	}
	for _, t := range res.TruncatedCells {
		if t[1] < n {
			out.TruncatedCells = append(out.TruncatedCells, t)
		}
	}
	return out, nil
}

// CancelBrowse interrupts the running page load of a grid.
func (a *App) CancelBrowse(token string) { a.queries.CancelBrowse(token) }

// CountTableRows runs SELECT COUNT(*) with the grid filter.
func (a *App) CountTableRows(token, id, db, table, filter string) (int64, error) {
	_, g, err := a.dataSQL(id)
	if err != nil {
		return 0, err
	}
	s, err := g.CountSQL(db, table, filter)
	if err != nil {
		return 0, wrap(err)
	}
	res, err := a.queries.Browse(context.Background(), query.BrowseRequest{Token: token + ":count", ConnectionID: id, SQL: s, MaxRows: 1})
	if err != nil {
		return 0, wrap(err)
	}
	if len(res.Rows) == 1 && len(res.Rows[0]) == 1 {
		var n int64
		if _, err := fmt.Sscan(fmt.Sprint(res.Rows[0][0]), &n); err == nil {
			return n, nil
		}
	}
	return 0, apperr.New(apperr.CodeInternal, "contagem indisponível")
}

// FetchRowValues reads the full values (up to 16 MiB each) of one row, used
// when viewing or editing a truncated cell.
func (a *App) FetchRowValues(id, db, table string, where []database.ColumnValue, columns []string) ([]any, error) {
	_, g, err := a.dataSQL(id)
	if err != nil {
		return nil, err
	}
	cols, key, err := a.tableMeta(id, db, table, g)
	if err != nil {
		return nil, wrap(err)
	}
	st, err := g.FetchRowSQL(db, table, cols, key, where, columns)
	if err != nil {
		return nil, wrap(err)
	}
	res, err := a.queries.Browse(context.Background(), query.BrowseRequest{ConnectionID: id, SQL: st.SQL, Args: st.Args, MaxRows: 1, MaxCellBytes: maxFullCell})
	if err != nil {
		return nil, wrap(err)
	}
	if len(res.Rows) == 0 {
		return nil, apperr.New(apperr.CodeNotFound, "a linha não existe mais (alterada ou removida por outra sessão?)")
	}
	return res.Rows[0], nil
}

// RowEditResult reports the outcome of ApplyRowEdits.
type RowEditResult struct {
	Results []query.AdminResult `json:"results"`
	// Applied is true when every statement succeeded and was committed.
	Applied bool `json:"applied"`
}

func (a *App) rowStatements(id, db, table string, edits []database.RowEdit) ([]database.Statement, error) {
	_, g, err := a.dataSQL(id)
	if err != nil {
		return nil, err
	}
	if len(edits) == 0 || len(edits) > 10000 {
		return nil, apperr.New(apperr.CodeValidation, "envie entre 1 e 10.000 alterações")
	}
	cols, key, err := a.tableMeta(id, db, table, g)
	if err != nil {
		return nil, wrap(err)
	}
	out := make([]database.Statement, 0, len(edits))
	for i, e := range edits {
		st, err := g.RowSQL(db, table, cols, key, e)
		if err != nil {
			return nil, wrap(fmt.Errorf("alteração %d: %w", i+1, err))
		}
		out = append(out, st)
	}
	return out, nil
}

// PreviewRowEdits returns the statements ApplyRowEdits would run, with
// values rendered as literals (display only).
func (a *App) PreviewRowEdits(id, db, table string, edits []database.RowEdit) ([]string, error) {
	stmts, err := a.rowStatements(id, db, table, edits)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(stmts))
	for i, s := range stmts {
		out[i] = s.Display
	}
	return out, nil
}

// ApplyRowEdits writes the pending changes of the data grid in a single
// transaction: values are bound as arguments, each statement may affect at
// most one row, and the first failure rolls everything back.
func (a *App) ApplyRowEdits(id, db, table string, edits []database.RowEdit, confirmed bool) (*RowEditResult, error) {
	stmts, err := a.rowStatements(id, db, table, edits)
	if err != nil {
		return nil, err
	}
	req := query.AdminRequest{ConnectionID: id, Confirmed: confirmed, Transaction: true, MaxAffected: 1, Timeout: 5 * time.Minute}
	for _, s := range stmts {
		req.Statements = append(req.Statements, s.SQL)
		req.Args = append(req.Args, s.Args)
		req.Display = append(req.Display, s.Display)
	}
	res, err := a.queries.RunAdmin(context.Background(), req)
	if err != nil {
		return nil, wrap(err)
	}
	// Show literal values to the user instead of placeholders.
	for i := range res {
		if i < len(stmts) {
			res[i].SQL = stmts[i].Display
		}
	}
	ok := len(res) == len(stmts)
	for _, r := range res {
		if r.Error != nil {
			ok = false
		}
	}
	return &RowEditResult{Results: res, Applied: ok}, nil
}

// DistinctValue is a value and how many rows have it.
type DistinctValue struct {
	Value any   `json:"value"`
	Count int64 `json:"count"`
}

// DistinctValues lists the most frequent values of a column (quick filters).
func (a *App) DistinctValues(id, db, table, column, filter string) ([]DistinctValue, error) {
	_, g, err := a.dataSQL(id)
	if err != nil {
		return nil, err
	}
	s, err := g.DistinctSQL(db, table, column, filter, 50)
	if err != nil {
		return nil, wrap(err)
	}
	res, err := a.queries.Browse(context.Background(), query.BrowseRequest{ConnectionID: id, SQL: s, MaxRows: 50, MaxCellBytes: 512, Timeout: time.Minute})
	if err != nil {
		return nil, wrap(err)
	}
	out := make([]DistinctValue, 0, len(res.Rows))
	for _, r := range res.Rows {
		var n int64
		_, _ = fmt.Sscan(fmt.Sprint(r[1]), &n)
		out = append(out, DistinctValue{Value: r[0], Count: n})
	}
	return out, nil
}

// LookupValue is an option of the foreign key dropdown.
type LookupValue struct {
	Value any    `json:"value"`
	Label string `json:"label,omitempty"`
}

// LookupValues lists values of a referenced column, labelled with the
// first text column of the referenced table (FK dropdown).
func (a *App) LookupValues(id, db, table, column, search string) ([]LookupValue, error) {
	_, g, err := a.dataSQL(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := metaCtx()
	cols, err := a.meta.Columns(ctx, id, db, table)
	cancel()
	if err != nil {
		return nil, wrap(err)
	}
	label := ""
	for _, c := range cols {
		t := strings.ToLower(c.DataType)
		if !strings.EqualFold(c.Name, column) && (t == "varchar" || t == "char" || t == "text" || t == "tinytext" || t == "enum") {
			label = c.Name
			break
		}
	}
	st, err := g.LookupSQL(db, table, column, label, search, 1000)
	if err != nil {
		return nil, wrap(err)
	}
	res, err := a.queries.Browse(context.Background(), query.BrowseRequest{ConnectionID: id, SQL: st.SQL, Args: st.Args, MaxRows: 1000, MaxCellBytes: 1024, Timeout: time.Minute})
	if err != nil {
		return nil, wrap(err)
	}
	out := make([]LookupValue, 0, len(res.Rows))
	for _, r := range res.Rows {
		v := LookupValue{Value: r[0]}
		if r[1] != nil {
			v.Label = fmt.Sprint(r[1])
		}
		out = append(out, v)
	}
	return out, nil
}

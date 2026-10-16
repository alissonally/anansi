package csvimport

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Import modes for duplicate keys.
const (
	ModeInsert  = "insert"
	ModeIgnore  = "ignore"
	ModeReplace = "replace"
)

// Target describes where the records go.
type Target struct {
	Database string `json:"database"`
	Table    string `json:"table"`
	// Columns maps each file column (by position) to a table column; ""
	// skips the file column.
	Columns []string `json:"columns"`
	// Numeric marks target columns that receive local number conversion.
	Numeric   []bool `json:"numeric"`
	Mode      string `json:"mode"`
	BatchRows int    `json:"batchRows"`
	// Atomic imports everything in one transaction (all or nothing).
	Atomic bool `json:"atomic"`
	// Thousands/Decimal separators of numeric values in the file ("" = as is).
	Thousands string `json:"thousands"`
	Decimal   string `json:"decimal"`
}

// Progress receives advancement.
type Progress interface {
	Set(done, total int64)
	Message(s string)
}

// Stats summarises an import.
type Stats struct {
	Rows     int64 `json:"rows"`
	Inserted int64 `json:"inserted"` // rows affected
}

func qi(s string) string { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }

// Import reads every record from r and inserts it through conn.
func Import(ctx context.Context, conn *sql.Conn, r *Reader, t Target, total int64, p Progress) (Stats, error) {
	var st Stats
	var cols []string
	var pos []int
	for i, c := range t.Columns {
		if c != "" {
			cols = append(cols, qi(c))
			pos = append(pos, i)
		}
	}
	if len(cols) == 0 {
		return st, errors.New("nenhuma coluna de destino selecionada")
	}
	verb := "INSERT INTO"
	switch t.Mode {
	case ModeIgnore:
		verb = "INSERT IGNORE INTO"
	case ModeReplace:
		verb = "REPLACE INTO"
	}
	batch := t.BatchRows
	if batch <= 0 {
		batch = 500
	}
	if limit := 60000 / len(cols); batch > limit { // placeholder limit is 65535
		batch = limit
	}
	head := verb + " " + qi(t.Database) + "." + qi(t.Table) + " (" + strings.Join(cols, ", ") + ") VALUES "
	tuple := "(" + strings.TrimSuffix(strings.Repeat("?, ", len(cols)), ", ") + ")"

	var execer interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	} = conn
	var tx *sql.Tx
	if t.Atomic {
		var err error
		if tx, err = conn.BeginTx(ctx, nil); err != nil {
			return st, err
		}
		execer = tx
		defer func() { _ = tx.Rollback() }()
	}
	args := make([]any, 0, batch*len(cols))
	rows := 0
	firstLine := 0
	flush := func() error {
		if rows == 0 {
			return nil
		}
		q := head + strings.TrimSuffix(strings.Repeat(tuple+", ", rows), ", ")
		res, err := execer.ExecContext(ctx, q, args...)
		if err != nil {
			return fmt.Errorf("registros das linhas %d a %d: %w", firstLine, r.Line(), err)
		}
		if n, err := res.RowsAffected(); err == nil {
			st.Inserted += n
		}
		args, rows = args[:0], 0
		p.Set(r.BytesRead, total)
		return nil
	}
	for {
		if err := ctx.Err(); err != nil {
			return st, err
		}
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return st, err
		}
		if len(rec) == 1 && rec[0] != nil && *rec[0] == "" {
			continue // blank line
		}
		if rows == 0 {
			firstLine = r.Line()
		}
		for k, i := range pos {
			var v any
			if i < len(rec) && rec[i] != nil {
				s := *rec[i]
				if k < len(t.Numeric) && t.Numeric[k] && (t.Thousands != "" || t.Decimal != "") {
					s = NormalizeNumber(s, t.Thousands, t.Decimal)
				}
				v = s
			}
			args = append(args, v)
		}
		rows++
		st.Rows++
		if rows >= batch {
			if err := flush(); err != nil {
				return st, err
			}
		}
	}
	if err := flush(); err != nil {
		return st, err
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return st, err
		}
	}
	p.Set(total, total)
	return st, nil
}

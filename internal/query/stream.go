package query

import (
	"context"
	"database/sql"
	"errors"

	"dbstudio/internal/apperr"
	"dbstudio/internal/connections"
)

// noTruncate disables the per-cell size cap of exports.
const noTruncate = 1 << 62

// WithConn runs fn on a dedicated pooled connection of a session (exports,
// dumps and imports that need several statements on the same connection,
// e.g. a consistent snapshot).
func (s *Service) WithConn(ctx context.Context, connectionID string, fn func(conn *sql.Conn, sess *connections.Session) error) error {
	sess, err := s.sessions.Get(connectionID)
	if err != nil {
		return err
	}
	conn, err := sess.DB.Conn(ctx)
	if err != nil {
		return apperr.FromDB(sess.Driver.DescribeError(err))
	}
	defer conn.Close()
	return fn(conn, sess)
}

// Queryer runs queries (sql.Conn, sql.DB and sql.Tx satisfy it).
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// StreamRows runs a query and calls onRow for every row with JSON-safe
// values (see convertValue), without truncating cells. onColumns is called
// once before the first row.
func StreamRows(ctx context.Context, q Queryer, query string, args []any, onColumns func([]Column) error, onRow func([]any) error) error {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	cts, err := rows.ColumnTypes()
	if err != nil {
		return err
	}
	cols := describeColumns(cts)
	if onColumns != nil {
		if err := onColumns(cols); err != nil {
			return err
		}
	}
	scan := make([]any, len(cols))
	for i := range scan {
		scan[i] = new(any)
	}
	row := make([]any, len(cols))
	for rows.Next() {
		if err := rows.Scan(scan...); err != nil {
			return err
		}
		for i, p := range scan {
			row[i], _ = convertValueMax(*(p.(*any)), cols[i], noTruncate)
		}
		if err := onRow(row); err != nil {
			return err
		}
	}
	return rows.Err()
}

// DescribeErr converts a driver error with the session password redacted.
func DescribeErr(sess *connections.Session, err error) error {
	if err == nil {
		return nil
	}
	var ae *apperr.Error
	if errors.As(err, &ae) {
		return ae
	}
	de := *sess.Driver.DescribeError(err)
	de.Message = sess.Redact(de.Message)
	return apperr.FromDB(&de)
}

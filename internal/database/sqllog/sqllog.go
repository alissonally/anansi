// Package sqllog wraps a database/sql/driver.Connector so that every
// statement sent to the server — by editor tabs, metadata queries or
// application dialogs — is reported to a callback. It powers the SQL log
// panel (the equivalent of HeidiSQL's log), which shows what the application
// really sends to the server.
//
// The wrapper is transparent: it forwards every optional driver interface the
// wrapped connection implements and never changes arguments or results.
package sqllog

import (
	"context"
	"database/sql/driver"
	"errors"
	"time"
)

// Func receives one executed statement. args holds the arguments of
// prepared statements (nil for text protocol queries). It is called
// synchronously from the goroutine running the query and must be fast.
type Func func(query string, args []driver.NamedValue, d time.Duration, err error)

// Wrap returns a connector reporting statements to fn. A nil fn returns c.
func Wrap(c driver.Connector, fn Func) driver.Connector {
	if fn == nil {
		return c
	}
	return &connector{inner: c, fn: fn}
}

type connector struct {
	inner driver.Connector
	fn    Func
}

func (c *connector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &wconn{inner: conn, fn: c.fn}, nil
}

func (c *connector) Driver() driver.Driver { return c.inner.Driver() }

// wconn forwards to the wrapped connection.
type wconn struct {
	inner driver.Conn
	fn    Func
}

var (
	_ driver.Conn               = (*wconn)(nil)
	_ driver.ConnBeginTx        = (*wconn)(nil)
	_ driver.ConnPrepareContext = (*wconn)(nil)
	_ driver.QueryerContext     = (*wconn)(nil)
	_ driver.ExecerContext      = (*wconn)(nil)
	_ driver.Pinger             = (*wconn)(nil)
	_ driver.SessionResetter    = (*wconn)(nil)
	_ driver.Validator          = (*wconn)(nil)
	_ driver.NamedValueChecker  = (*wconn)(nil)
)

func (w *wconn) report(q string, args []driver.NamedValue, start time.Time, err error) {
	if errors.Is(err, driver.ErrSkip) {
		return // database/sql retries through Prepare; that call is reported
	}
	w.fn(q, args, time.Since(start), err)
}

func (w *wconn) Prepare(query string) (driver.Stmt, error) {
	return w.PrepareContext(context.Background(), query)
}

func (w *wconn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	var st driver.Stmt
	var err error
	if p, ok := w.inner.(driver.ConnPrepareContext); ok {
		st, err = p.PrepareContext(ctx, query)
	} else {
		st, err = w.inner.Prepare(query)
	}
	if err != nil {
		w.fn(query, nil, 0, err)
		return nil, err
	}
	return &wstmt{inner: st, query: query, conn: w}, nil
}

func (w *wconn) Close() error { return w.inner.Close() }

func (w *wconn) Begin() (driver.Tx, error) {
	return w.BeginTx(context.Background(), driver.TxOptions{})
}

func (w *wconn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	start := time.Now()
	var tx driver.Tx
	var err error
	if b, ok := w.inner.(driver.ConnBeginTx); ok {
		tx, err = b.BeginTx(ctx, opts)
	} else {
		tx, err = w.inner.Begin() //nolint:staticcheck // fallback for old drivers
	}
	w.report("START TRANSACTION", nil, start, err)
	if err != nil {
		return nil, err
	}
	return &wtx{inner: tx, conn: w}, nil
}

func (w *wconn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	q, ok := w.inner.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	start := time.Now()
	rows, err := q.QueryContext(ctx, query, args)
	w.report(query, args, start, err)
	return rows, err
}

func (w *wconn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	e, ok := w.inner.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	start := time.Now()
	res, err := e.ExecContext(ctx, query, args)
	w.report(query, args, start, err)
	return res, err
}

func (w *wconn) Ping(ctx context.Context) error {
	if p, ok := w.inner.(driver.Pinger); ok {
		return p.Ping(ctx)
	}
	return nil
}

func (w *wconn) ResetSession(ctx context.Context) error {
	if r, ok := w.inner.(driver.SessionResetter); ok {
		return r.ResetSession(ctx)
	}
	return nil
}

func (w *wconn) IsValid() bool {
	if v, ok := w.inner.(driver.Validator); ok {
		return v.IsValid()
	}
	return true
}

func (w *wconn) CheckNamedValue(nv *driver.NamedValue) error {
	if c, ok := w.inner.(driver.NamedValueChecker); ok {
		return c.CheckNamedValue(nv)
	}
	return driver.ErrSkip
}

type wtx struct {
	inner driver.Tx
	conn  *wconn
}

func (t *wtx) Commit() error {
	start := time.Now()
	err := t.inner.Commit()
	t.conn.report("COMMIT", nil, start, err)
	return err
}

func (t *wtx) Rollback() error {
	start := time.Now()
	err := t.inner.Rollback()
	t.conn.report("ROLLBACK", nil, start, err)
	return err
}

type wstmt struct {
	inner driver.Stmt
	query string
	conn  *wconn
}

var (
	_ driver.StmtExecContext   = (*wstmt)(nil)
	_ driver.StmtQueryContext  = (*wstmt)(nil)
	_ driver.NamedValueChecker = (*wstmt)(nil)
)

func (s *wstmt) Close() error  { return s.inner.Close() }
func (s *wstmt) NumInput() int { return s.inner.NumInput() }

func (s *wstmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.ExecContext(context.Background(), toNamed(args))
}

func (s *wstmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.QueryContext(context.Background(), toNamed(args))
}

func (s *wstmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	start := time.Now()
	var res driver.Result
	var err error
	if e, ok := s.inner.(driver.StmtExecContext); ok {
		res, err = e.ExecContext(ctx, args)
	} else {
		res, err = s.inner.Exec(toValues(args)) //nolint:staticcheck // fallback for old drivers
	}
	s.conn.report(s.query, args, start, err)
	return res, err
}

func (s *wstmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	start := time.Now()
	var rows driver.Rows
	var err error
	if q, ok := s.inner.(driver.StmtQueryContext); ok {
		rows, err = q.QueryContext(ctx, args)
	} else {
		rows, err = s.inner.Query(toValues(args)) //nolint:staticcheck // fallback for old drivers
	}
	s.conn.report(s.query, args, start, err)
	return rows, err
}

func (s *wstmt) CheckNamedValue(nv *driver.NamedValue) error {
	if c, ok := s.inner.(driver.NamedValueChecker); ok {
		return c.CheckNamedValue(nv)
	}
	return driver.ErrSkip
}

func toNamed(args []driver.Value) []driver.NamedValue {
	out := make([]driver.NamedValue, len(args))
	for i, v := range args {
		out[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return out
}

func toValues(args []driver.NamedValue) []driver.Value {
	out := make([]driver.Value, len(args))
	for i, a := range args {
		out[i] = a.Value
	}
	return out
}

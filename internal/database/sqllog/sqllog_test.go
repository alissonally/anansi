package sqllog

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeConn implements the context interfaces; queries with args return
// ErrSkip like go-sql-driver/mysql without interpolateParams.
type fakeConn struct{}

func (fakeConn) Prepare(q string) (driver.Stmt, error) { return fakeStmt{}, nil }
func (fakeConn) Close() error                          { return nil }
func (fakeConn) Begin() (driver.Tx, error)             { return fakeTx{}, nil }
func (fakeConn) QueryContext(_ context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	if len(args) > 0 {
		return nil, driver.ErrSkip
	}
	if strings.Contains(q, "fail") {
		return nil, errors.New("boom")
	}
	return &fakeRows{}, nil
}
func (fakeConn) ExecContext(_ context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	if len(args) > 0 {
		return nil, driver.ErrSkip
	}
	return driver.RowsAffected(1), nil
}

type fakeTx struct{}

func (fakeTx) Commit() error   { return nil }
func (fakeTx) Rollback() error { return nil }

type fakeStmt struct{}

func (fakeStmt) Close() error                                    { return nil }
func (fakeStmt) NumInput() int                                   { return -1 }
func (fakeStmt) Exec(args []driver.Value) (driver.Result, error) { return driver.RowsAffected(1), nil }
func (fakeStmt) Query(args []driver.Value) (driver.Rows, error)  { return &fakeRows{}, nil }

type fakeRows struct{ done bool }

func (*fakeRows) Columns() []string { return []string{"x"} }
func (*fakeRows) Close() error      { return nil }
func (r *fakeRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = int64(1)
	return nil
}

type fakeDriver struct{}

func (fakeDriver) Open(string) (driver.Conn, error) { return fakeConn{}, nil }

type fakeConnector struct{}

func (fakeConnector) Connect(context.Context) (driver.Conn, error) { return fakeConn{}, nil }
func (fakeConnector) Driver() driver.Driver                        { return fakeDriver{} }

func TestWrapReportsEveryStatementOnce(t *testing.T) {
	var mu sync.Mutex
	var got []string
	db := sql.OpenDB(Wrap(fakeConnector{}, func(q string, args []driver.NamedValue, d time.Duration, err error) {
		mu.Lock()
		defer mu.Unlock()
		s := q
		if len(args) > 0 {
			s += " +args"
		}
		if err != nil {
			s += " !err"
		}
		got = append(got, s)
	}))
	defer db.Close()
	var x int
	if err := db.QueryRow("SELECT 1").Scan(&x); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT ?", 5).Scan(&x); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE t SET a = 1"); err != nil {
		t.Fatal(err)
	}
	_, _ = db.Query("SELECT fail")
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.Commit()
	want := []string{"SELECT 1", "SELECT ? +args", "UPDATE t SET a = 1", "SELECT fail !err", "START TRANSACTION", "COMMIT"}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestWrapNilFunc(t *testing.T) {
	c := fakeConnector{}
	if Wrap(c, nil) != driver.Connector(c) {
		t.Fatal("nil func should return the connector unchanged")
	}
}

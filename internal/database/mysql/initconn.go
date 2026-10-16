package mysql

import (
	"context"
	"database/sql/driver"
	"errors"
)

// withInit runs the profile's startup statements on every new physical
// connection (HeidiSQL "startup script"); a failing statement fails the
// connection, so the pool never hands out a half-initialized session.
func withInit(c driver.Connector, stmts []string) driver.Connector {
	if len(stmts) == 0 {
		return c
	}
	return &initConnector{inner: c, stmts: stmts}
}

type initConnector struct {
	inner driver.Connector
	stmts []string
}

func (c *initConnector) Driver() driver.Driver { return c.inner.Driver() }

func (c *initConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	ex, ok := conn.(driver.ExecerContext)
	if !ok {
		_ = conn.Close()
		return nil, errors.New("o driver não executa o script inicial")
	}
	for _, s := range c.stmts {
		if _, err := ex.ExecContext(ctx, s, nil); err != nil {
			_ = conn.Close()
			return nil, &initError{stmt: s, err: err}
		}
	}
	return conn, nil
}

type initError struct {
	stmt string
	err  error
}

func (e *initError) Error() string { return "script inicial (" + e.stmt + "): " + e.err.Error() }
func (e *initError) Unwrap() error { return e.err }

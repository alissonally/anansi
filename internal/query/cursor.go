package query

import (
	"context"
	"database/sql"
	"time"
)

// cursor is an open result set being read page by page. It owns a context
// independent from the execution so it survives the Execute call; cancelling
// it aborts the transfer quickly (the connection is then discarded).
type cursor struct {
	id           string
	rows         *sql.Rows
	cols         []Column
	fetched      int
	hasMore      bool
	limitReached bool
	// exhausted reports that the server sent every row (closing is cheap).
	exhausted bool
	lastUsed  time.Time
	cancel    context.CancelFunc
	scan      []any
	// peeked holds a row read ahead to know whether more rows exist.
	peeked []any
}

func newCursor(rows *sql.Rows) (*cursor, error) {
	cts, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	c := &cursor{rows: rows, cols: describeColumns(cts)}
	c.scan = make([]any, len(c.cols))
	for i := range c.scan {
		c.scan[i] = new(any)
	}
	return c, nil
}

func (c *cursor) readRow() ([]any, bool, error) {
	if !c.rows.Next() {
		return nil, false, c.rows.Err()
	}
	if err := c.rows.Scan(c.scan...); err != nil {
		return nil, false, err
	}
	raw := make([]any, len(c.scan))
	for i, p := range c.scan {
		raw[i] = *(p.(*any))
	}
	return raw, true, nil
}

// fetch reads up to n rows. offset is the absolute index of the first row
// (used to report truncated cells). After the call hasMore tells whether the
// caller may fetch again; at MaxCursorRows the cursor stops (limitReached)
// even if the server has more rows (exhausted == false).
func (c *cursor) fetch(n, offset int) ([][]any, [][2]int, error) {
	if n <= 0 {
		n = 500
	}
	if remaining := MaxCursorRows - c.fetched; n > remaining {
		n = remaining
	}
	out := make([][]any, 0, min(max(n, 0), 1024))
	var trunc [][2]int
	add := func(raw []any) {
		row := make([]any, len(raw))
		for i, v := range raw {
			cv, t := convertValue(v, c.cols[i])
			row[i] = cv
			if t {
				trunc = append(trunc, [2]int{offset + len(out), i})
			}
		}
		out = append(out, row)
	}
	finish := func(err error) ([][]any, [][2]int, error) {
		c.fetched += len(out)
		c.hasMore = false
		c.exhausted = err == nil
		return out, trunc, err
	}
	if c.peeked != nil && n > 0 {
		add(c.peeked)
		c.peeked = nil
	}
	for len(out) < n {
		raw, ok, err := c.readRow()
		if err != nil || !ok {
			return finish(err)
		}
		add(raw)
	}
	// Look ahead one row to know whether there is more.
	raw, ok, err := c.readRow()
	if err != nil || !ok {
		return finish(err)
	}
	c.fetched += len(out)
	c.peeked = raw
	if c.fetched >= MaxCursorRows {
		c.limitReached = true
		c.hasMore = false
		return out, trunc, nil
	}
	c.hasMore = true
	return out, trunc, nil
}

// abort cancels the transfer (fast) and closes the rows.
func (c *cursor) abort() {
	if c.cancel != nil {
		c.cancel()
	}
	_ = c.rows.Close()
}

// close closes the rows, draining any unread data.
func (c *cursor) close() {
	_ = c.rows.Close()
	if c.cancel != nil {
		c.cancel()
	}
}

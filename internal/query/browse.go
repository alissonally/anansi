package query

import (
	"context"
	"time"

	"dbstudio/internal/apperr"
	"dbstudio/internal/connections"
	"dbstudio/internal/database"
)

// BrowseRequest runs one read query for the table data grid (or another
// application view) on a pooled connection.
type BrowseRequest struct {
	// Token identifies the request for cancellation (e.g. the tab id); a new
	// request with the same token cancels the previous one.
	Token        string
	ConnectionID string
	Database     string
	SQL          string
	Args         []any
	// MaxRows caps the rows read; HasMore reports one extra row was seen.
	MaxRows int
	// MaxCellBytes caps each cell (default MaxCellBytes).
	MaxCellBytes int
	Timeout      time.Duration
}

// BrowseResult is the outcome of Browse.
type BrowseResult struct {
	Columns        []Column `json:"columns"`
	Rows           [][]any  `json:"rows"`
	TruncatedCells [][2]int `json:"truncatedCells,omitempty"`
	HasMore        bool     `json:"hasMore"`
	DurationMs     float64  `json:"durationMs"`
}

type browsing struct {
	cancel context.CancelFunc
	connID int64
	sessID string
}

func (s *Service) setBrowse(token string, b *browsing) (prev *browsing) {
	s.browseMu.Lock()
	defer s.browseMu.Unlock()
	if s.browses == nil {
		s.browses = map[string]*browsing{}
	}
	prev = s.browses[token]
	s.browses[token] = b
	return prev
}

// Browse runs a read statement and returns at most MaxRows rows. Reads are
// not subject to confirmations; the SQL is built by the backend.
func (s *Service) Browse(ctx context.Context, req BrowseRequest) (*BrowseResult, error) {
	sess, err := s.sessions.Get(req.ConnectionID)
	if err != nil {
		return nil, err
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
		if t := s.options().Timeout; t > 0 {
			timeout = t
		}
	}
	maxRows := req.MaxRows
	if maxRows <= 0 {
		maxRows = 1000
	}
	maxCell := req.MaxCellBytes
	if maxCell <= 0 {
		maxCell = MaxCellBytes
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := sess.DB.Conn(ctx)
	if err != nil {
		return nil, apperr.FromDB(sess.Driver.DescribeError(err))
	}
	defer conn.Close()
	b := &browsing{cancel: cancel, sessID: sess.ID}
	if c, ok := sess.Driver.(database.Canceller); ok {
		if id, err := c.ConnectionID(ctx, conn); err == nil {
			b.connID = id
		}
	}
	if req.Token != "" {
		if prev := s.setBrowse(req.Token, b); prev != nil {
			s.stopBrowse(prev)
		}
		defer func() {
			s.browseMu.Lock()
			if s.browses[req.Token] == b {
				delete(s.browses, req.Token)
			}
			s.browseMu.Unlock()
		}()
	}
	if req.Database != "" {
		if cd, ok := sess.Driver.(database.CurrentDatabaser); ok {
			if err := cd.UseDatabase(ctx, conn, req.Database); err != nil {
				return nil, s.describe(sess, err)
			}
		}
	}
	start := time.Now()
	rows, err := conn.QueryContext(ctx, req.SQL, req.Args...)
	if err != nil {
		return nil, s.describe(sess, err)
	}
	defer rows.Close()
	cts, err := rows.ColumnTypes()
	if err != nil {
		return nil, s.describe(sess, err)
	}
	out := &BrowseResult{Columns: describeColumns(cts), Rows: [][]any{}}
	scan := make([]any, len(out.Columns))
	for i := range scan {
		scan[i] = new(any)
	}
	for rows.Next() {
		if len(out.Rows) >= maxRows {
			out.HasMore = true
			break
		}
		if err := rows.Scan(scan...); err != nil {
			return nil, s.describe(sess, err)
		}
		row := make([]any, len(scan))
		for i, p := range scan {
			v, t := convertValueMax(*(p.(*any)), out.Columns[i], maxCell)
			row[i] = v
			if t {
				out.TruncatedCells = append(out.TruncatedCells, [2]int{len(out.Rows), i})
			}
		}
		out.Rows = append(out.Rows, row)
	}
	if out.HasMore {
		// Do not download the rest: cancelling makes the driver close the
		// socket, and the connection is discarded instead of drained.
		cancel()
		_ = rows.Close()
	} else if err := rows.Err(); err != nil {
		return nil, s.describe(sess, err)
	}
	out.DurationMs = ms(time.Since(start))
	return out, nil
}

func (s *Service) describe(sess *connections.Session, err error) error {
	de := *sess.Driver.DescribeError(err)
	de.Message = sess.Redact(de.Message)
	return apperr.FromDB(&de)
}

// stopBrowse interrupts a running browse (KILL QUERY, then the context).
func (s *Service) stopBrowse(b *browsing) {
	if b.connID > 0 {
		if sess, err := s.sessions.Get(b.sessID); err == nil {
			if c, ok := sess.Driver.(database.Canceller); ok {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				_ = c.CancelQuery(ctx, sess.DB, b.connID)
				cancel()
			}
		}
	}
	b.cancel()
}

// CancelBrowse interrupts the running Browse of a token.
func (s *Service) CancelBrowse(token string) {
	s.browseMu.Lock()
	b := s.browses[token]
	s.browseMu.Unlock()
	if b != nil {
		s.stopBrowse(b)
	}
}

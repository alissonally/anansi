package api

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"anansi/internal/database/sqllog"
	"anansi/internal/history"
)

// sqlLogCapacity is how many statements the SQL log keeps in memory.
const sqlLogCapacity = 5000

const sqlLogMaxText = 4000

// SQLLogEntry is one statement sent to a server (SQL log panel).
type SQLLogEntry struct {
	ID           int64     `json:"id"`
	At           time.Time `json:"at"`
	ConnectionID string    `json:"connectionId"`
	SQL          string    `json:"sql"`
	DurationMs   float64   `json:"durationMs"`
	Error        string    `json:"error,omitempty"`
}

// sqlLog is an in-memory ring buffer of statements. New entries are pushed
// to the frontend in batches (event "sqllog:entries").
type sqlLog struct {
	mu      sync.Mutex
	next    int64
	entries []SQLLogEntry // ring buffer
	start   int
	pending []SQLLogEntry
	timer   *time.Timer
	emit    func(batch []SQLLogEntry)
	redact  func(connID, s string) string
	// capacity is the number of entries kept (preference).
	capacity int
}

// setCapacity resizes the ring keeping the newest entries.
func (l *sqlLog) setCapacity(n int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n == l.capacity {
		return
	}
	ordered := append(append([]SQLLogEntry(nil), l.entries[l.start:]...), l.entries[:l.start]...)
	if len(ordered) > n {
		ordered = ordered[len(ordered)-n:]
	}
	l.entries, l.start, l.capacity = ordered, 0, n
}

func newSQLLog(emit func([]SQLLogEntry), redact func(connID, s string) string) *sqlLog {
	return &sqlLog{next: 1, emit: emit, redact: redact}
}

// funcFor returns the sqllog.Func used by the session of a profile.
func (l *sqlLog) funcFor(connID string) sqllog.Func {
	return func(query string, args []driver.NamedValue, d time.Duration, err error) {
		text := strings.TrimSpace(history.Sanitize(query))
		if len(args) > 0 {
			text += "  /* " + formatArgs(args) + " */"
		}
		if len(text) > sqlLogMaxText {
			cut := sqlLogMaxText
			for cut > 0 && !utf8.RuneStart(text[cut]) {
				cut--
			}
			text = text[:cut] + " …"
		}
		e := SQLLogEntry{At: time.Now(), ConnectionID: connID, SQL: text, DurationMs: float64(d.Microseconds()) / 1000}
		if err != nil {
			msg := err.Error()
			if l.redact != nil {
				msg = l.redact(connID, msg)
			}
			e.Error = msg
		}
		l.add(e)
	}
}

func formatArgs(args []driver.NamedValue) string {
	parts := make([]string, len(args))
	for i, a := range args {
		switch v := a.Value.(type) {
		case nil:
			parts[i] = "NULL"
		case string:
			parts[i] = "'" + truncate(v, 80) + "'"
		case []byte:
			parts[i] = fmt.Sprintf("<%d bytes>", len(v))
		default:
			parts[i] = truncate(fmt.Sprint(v), 80)
		}
	}
	return strings.Join(parts, ", ")
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func (l *sqlLog) add(e SQLLogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e.ID = l.next
	l.next++
	limit := l.capacity
	if limit <= 0 {
		limit = sqlLogCapacity
	}
	if len(l.entries) < limit {
		l.entries = append(l.entries, e)
	} else {
		l.entries[l.start] = e
		l.start = (l.start + 1) % len(l.entries)
	}
	if l.emit == nil {
		return
	}
	l.pending = append(l.pending, e)
	if len(l.pending) > 500 {
		l.pending = l.pending[len(l.pending)-500:]
	}
	if l.timer == nil {
		l.timer = time.AfterFunc(200*time.Millisecond, l.flush)
	}
}

func (l *sqlLog) flush() {
	l.mu.Lock()
	batch := l.pending
	l.pending = nil
	l.timer = nil
	l.mu.Unlock()
	if len(batch) > 0 {
		l.emit(batch)
	}
}

// list returns entries with ID > since, oldest first, at most limit.
func (l *sqlLog) list(since int64, limit int) []SQLLogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := len(l.entries)
	out := make([]SQLLogEntry, 0, min(n, max(limit, 0)))
	for i := 0; i < n; i++ {
		e := l.entries[(l.start+i)%n]
		if e.ID > since {
			out = append(out, e)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

func (l *sqlLog) clear() {
	l.mu.Lock()
	l.entries, l.start, l.pending = nil, 0, nil
	l.mu.Unlock()
}

// ListSQLLog returns logged statements newer than since (0 = all).
func (a *App) ListSQLLog(since int64, limit int) []SQLLogEntry { return a.sqlLog.list(since, limit) }

// ClearSQLLog empties the SQL log.
func (a *App) ClearSQLLog() { a.sqlLog.clear() }

func (a *App) emitSQLLog(batch []SQLLogEntry) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "sqllog:entries", batch)
	}
}

// Package history records executed statements locally.
package history

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dbstudio/internal/config"
	"dbstudio/internal/sqllex"
)

// Version is the schema version of history.json.
const Version = 1

// MaxSQLBytes caps the stored statement text.
const MaxSQLBytes = 64 * 1024

// Entry is one executed statement.
type Entry struct {
	ID             int64     `json:"id"`
	ConnectionID   string    `json:"connectionId"`
	ConnectionName string    `json:"connectionName"`
	Database       string    `json:"database"`
	SQL            string    `json:"sql"`
	ExecutedAt     time.Time `json:"executedAt"`
	DurationMs     float64   `json:"durationMs"`
	Success        bool      `json:"success"`
	RowCount       int64     `json:"rowCount"`
	Error          string    `json:"error,omitempty"`
}

type file struct {
	NextID  int64   `json:"nextId"`
	Entries []Entry `json:"entries"`
}

// Store keeps the most recent entries (newest last) and persists them with a
// small write delay so bursts of statements produce one write.
type Store struct {
	mu      sync.Mutex
	path    string
	f       file
	limit   int
	dirty   bool
	timer   *time.Timer
	delay   time.Duration
	saveErr error
}

// Open loads history from dir.
func Open(dir string, limit int) (*Store, error) {
	s := &Store{path: filepath.Join(dir, "history.json"), limit: limit, delay: 500 * time.Millisecond}
	f, ok, err := config.Load[file](s.path, Version)
	if ok {
		s.f = f
	}
	if s.f.NextID == 0 {
		s.f.NextID = 1
	}
	return s, err
}

// SetLimit changes the maximum number of entries.
func (s *Store) SetLimit(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.limit = n
	s.trimLocked()
}

func (s *Store) trimLocked() {
	if s.limit > 0 && len(s.f.Entries) > s.limit {
		s.f.Entries = append([]Entry(nil), s.f.Entries[len(s.f.Entries)-s.limit:]...)
		s.dirty = true
	}
}

// Add records an entry. The SQL is sanitized first.
func (s *Store) Add(e Entry) {
	e.SQL = Sanitize(e.SQL)
	if len(e.SQL) > MaxSQLBytes {
		e.SQL = e.SQL[:MaxSQLBytes] + "\n-- (truncado)"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e.ID = s.f.NextID
	s.f.NextID++
	s.f.Entries = append(s.f.Entries, e)
	s.trimLocked()
	s.dirty = true
	s.scheduleLocked()
}

func (s *Store) scheduleLocked() {
	if s.timer != nil {
		return
	}
	s.timer = time.AfterFunc(s.delay, func() { _ = s.Flush() })
}

// Flush writes pending changes.
func (s *Store) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timer = nil
	if !s.dirty {
		return nil
	}
	if err := config.Save(s.path, Version, s.f); err != nil {
		s.saveErr = err
		return err
	}
	s.dirty = false
	return nil
}

// Query filters entries. Newest first.
type Query struct {
	ConnectionID string `json:"connectionId"`
	Search       string `json:"search"`
	Limit        int    `json:"limit"`
}

// List returns entries matching q, newest first.
func (s *Store) List(q Query) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	if q.Limit <= 0 || q.Limit > 1000 {
		q.Limit = 200
	}
	search := strings.ToLower(q.Search)
	var out []Entry
	for i := len(s.f.Entries) - 1; i >= 0 && len(out) < q.Limit; i-- {
		e := s.f.Entries[i]
		if q.ConnectionID != "" && e.ConnectionID != q.ConnectionID {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(e.SQL), search) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// Clear removes entries (all, or those of one connection).
func (s *Store) Clear(connectionID string) error {
	s.mu.Lock()
	if connectionID == "" {
		s.f.Entries = nil
	} else {
		kept := s.f.Entries[:0:0]
		for _, e := range s.f.Entries {
			if e.ConnectionID != connectionID {
				kept = append(kept, e)
			}
		}
		s.f.Entries = kept
	}
	s.dirty = true
	s.mu.Unlock()
	return s.Flush()
}

// Sanitize masks string literals in statements that carry credentials
// (IDENTIFIED BY, PASSWORD, ...) so they are not written to disk.
func Sanitize(sql string) string {
	toks := sqllex.Tokenize(sql)
	sensitive := false
	for _, t := range toks {
		if t.IsKeyword("IDENTIFIED", "PASSWORD", "MASTER_PASSWORD", "SOURCE_PASSWORD", "AES_ENCRYPT", "AES_DECRYPT", "ENCODE", "DECODE") {
			sensitive = true
			break
		}
	}
	if !sensitive {
		return sql
	}
	var b strings.Builder
	for _, t := range toks {
		if t.Kind == sqllex.String {
			b.WriteString("'***'")
			continue
		}
		b.WriteString(t.Text)
	}
	return b.String()
}

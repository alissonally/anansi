// Package metadata discovers and caches catalog information (databases,
// tables, columns, indexes) per session. The cache serves both the object
// tree and the autocomplete engine.
//
// Concurrent requests for the same key share a single query; entries expire
// after a TTL and are invalidated explicitly after DDL statements, USE, or a
// manual refresh.
package metadata

import (
	"context"
	"strings"
	"sync"
	"time"

	"dbstudio/internal/apperr"
	"dbstudio/internal/connections"
	"dbstudio/internal/database"
)

// TTL is how long cached metadata is considered fresh.
const TTL = 5 * time.Minute

// queryTimeout bounds every metadata query.
const queryTimeout = 20 * time.Second

// Sessions is the subset of connections.Manager used here.
type Sessions interface {
	Get(id string) (*connections.Session, error)
	OnClose(fn func(id string))
}

type entry struct {
	at    time.Time
	value any
	err   error
	ready chan struct{}
}

// Cache holds metadata for all sessions.
type Cache struct {
	sessions Sessions
	now      func() time.Time

	mu      sync.Mutex
	entries map[string]*entry // key: session \x00 kind \x00 db \x00 table
}

// NewCache creates the cache and hooks session closing.
func NewCache(sessions Sessions) *Cache {
	c := &Cache{sessions: sessions, now: time.Now, entries: map[string]*entry{}}
	sessions.OnClose(func(id string) { c.Invalidate(id, "") })
	return c
}

func key(parts ...string) string { return strings.Join(parts, "\x00") }

// load returns a cached value or computes it once for concurrent callers.
func (c *Cache) load(ctx context.Context, k string, fn func(ctx context.Context) (any, error)) (any, error) {
	c.mu.Lock()
	if e, ok := c.entries[k]; ok {
		select {
		case <-e.ready:
			if e.err == nil && c.now().Sub(e.at) < TTL {
				c.mu.Unlock()
				return e.value, nil
			}
		default:
			c.mu.Unlock()
			select {
			case <-e.ready:
				return e.value, e.err
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	e := &entry{ready: make(chan struct{})}
	c.entries[k] = e
	c.mu.Unlock()

	qctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	v, err := fn(qctx)
	cancel()

	c.mu.Lock()
	e.value, e.err, e.at = v, err, c.now()
	if err != nil && c.entries[k] == e {
		delete(c.entries, k) // do not cache failures
	}
	close(e.ready)
	c.mu.Unlock()
	return v, err
}

// Invalidate drops cached data of a session; db == "" drops everything for
// the session, otherwise only that database (and the database list).
func (c *Cache) Invalidate(sessionID, db string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix := sessionID + "\x00"
	for k := range c.entries {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		parts := strings.Split(k, "\x00")
		if db == "" || parts[1] == "databases" || (len(parts) > 2 && strings.EqualFold(parts[2], db)) {
			delete(c.entries, k)
		}
	}
}

func (c *Cache) session(id string) (*connections.Session, database.Inspector, error) {
	s, err := c.sessions.Get(id)
	if err != nil {
		return nil, nil, err
	}
	return s, s.Driver.Inspector(s.DB), nil
}

func wrap(s *connections.Session, err error) error {
	if err == nil {
		return nil
	}
	de := *s.Driver.DescribeError(err)
	de.Message = s.Redact(de.Message)
	return apperr.FromDB(&de)
}

// Databases lists databases.
func (c *Cache) Databases(ctx context.Context, sessionID string) ([]database.DatabaseInfo, error) {
	s, in, err := c.session(sessionID)
	if err != nil {
		return nil, err
	}
	v, err := c.load(ctx, key(sessionID, "databases"), func(ctx context.Context) (any, error) {
		return in.Databases(ctx)
	})
	if err != nil {
		return nil, wrap(s, err)
	}
	return v.([]database.DatabaseInfo), nil
}

// Tables lists tables and views of a database.
func (c *Cache) Tables(ctx context.Context, sessionID, db string) ([]database.TableInfo, error) {
	s, in, err := c.session(sessionID)
	if err != nil {
		return nil, err
	}
	v, err := c.load(ctx, key(sessionID, "tables", db), func(ctx context.Context) (any, error) {
		return in.Tables(ctx, db)
	})
	if err != nil {
		return nil, wrap(s, err)
	}
	return v.([]database.TableInfo), nil
}

// AllColumns returns every column of a database, keyed by table name.
func (c *Cache) AllColumns(ctx context.Context, sessionID, db string) (map[string][]database.ColumnInfo, error) {
	s, in, err := c.session(sessionID)
	if err != nil {
		return nil, err
	}
	v, err := c.load(ctx, key(sessionID, "columns", db), func(ctx context.Context) (any, error) {
		return in.AllColumns(ctx, db)
	})
	if err != nil {
		return nil, wrap(s, err)
	}
	return v.(map[string][]database.ColumnInfo), nil
}

// Columns returns the columns of a table (served from AllColumns).
func (c *Cache) Columns(ctx context.Context, sessionID, db, table string) ([]database.ColumnInfo, error) {
	all, err := c.AllColumns(ctx, sessionID, db)
	if err != nil {
		return nil, err
	}
	if cols, ok := all[table]; ok {
		return cols, nil
	}
	for name, cols := range all {
		if strings.EqualFold(name, table) {
			return cols, nil
		}
	}
	return nil, nil
}

// Indexes lists indexes of a table.
func (c *Cache) Indexes(ctx context.Context, sessionID, db, table string) ([]database.IndexInfo, error) {
	s, in, err := c.session(sessionID)
	if err != nil {
		return nil, err
	}
	v, err := c.load(ctx, key(sessionID, "indexes", db, table), func(ctx context.Context) (any, error) {
		return in.Indexes(ctx, db, table)
	})
	if err != nil {
		return nil, wrap(s, err)
	}
	return v.([]database.IndexInfo), nil
}

// Routines lists stored procedures and functions.
func (c *Cache) Routines(ctx context.Context, sessionID, db string) ([]database.RoutineInfo, error) {
	s, in, err := c.session(sessionID)
	if err != nil {
		return nil, err
	}
	v, err := c.load(ctx, key(sessionID, "routines", db), func(ctx context.Context) (any, error) {
		return in.Routines(ctx, db)
	})
	if err != nil {
		return nil, wrap(s, err)
	}
	return v.([]database.RoutineInfo), nil
}

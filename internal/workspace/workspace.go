// Package workspace persists editor tabs (so work survives restarts) and
// saved queries.
package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"anansi/internal/apperr"
	"anansi/internal/config"
)

// Version is the schema version of workspace.json.
const Version = 1

// MaxTabSQLBytes limits persisted tab content.
const MaxTabSQLBytes = 2 * 1024 * 1024

// Tab is the persisted state of an editor tab. Results are not persisted.
type Tab struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	ConnectionID string `json:"connectionId"`
	Database     string `json:"database"`
	SQL          string `json:"sql"`
	FilePath     string `json:"filePath,omitempty"`
	SavedQueryID string `json:"savedQueryId,omitempty"`
	// SavedContent is the content last saved to the file / saved query; used
	// to compute the "unsaved changes" state.
	SavedContent *string `json:"savedContent,omitempty"`
	// Kind is "query" (default) or an object view: "host", "database",
	// "table", "view", "routine", "trigger", "event", "users".
	Kind string `json:"kind,omitempty"`
	// ObjectName/ObjectType identify the object of object tabs (ObjectType
	// is PROCEDURE or FUNCTION for routines).
	ObjectName string `json:"objectName,omitempty"`
	ObjectType string `json:"objectType,omitempty"`
	// View is the selected sub-tab of object tabs.
	View string `json:"view,omitempty"`
}

// State is the persisted workspace.
type State struct {
	Tabs           []Tab  `json:"tabs"`
	ActiveTabID    string `json:"activeTabId"`
	SidebarWidth   int    `json:"sidebarWidth"`
	ResultsHeight  int    `json:"resultsHeight"`
	LogVisible     bool   `json:"logVisible,omitempty"`
	LogHeight      int    `json:"logHeight,omitempty"`
	HelpersVisible bool   `json:"helpersVisible,omitempty"`
	HelpersWidth   int    `json:"helpersWidth,omitempty"`
	// OpenConnections are the sessions open at the last save (reconnect on startup).
	OpenConnections []string `json:"openConnections,omitempty"`
}

// SavedQuery is a named query.
type SavedQuery struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	SQL          string    `json:"sql"`
	ConnectionID string    `json:"connectionId,omitempty"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type queriesFile struct {
	Queries []SavedQuery `json:"queries"`
}

// Store persists workspace state and saved queries.
type Store struct {
	mu      sync.Mutex
	dir     string
	queries []SavedQuery
}

// Open loads saved queries from dir.
func Open(dir string) (*Store, error) {
	s := &Store{dir: dir}
	f, ok, err := config.Load[queriesFile](filepath.Join(dir, "queries.json"), Version)
	if ok {
		s.queries = f.Queries
	}
	return s, err
}

// LoadState returns the persisted tabs.
func (s *Store) LoadState() (State, error) {
	st, _, err := config.Load[State](filepath.Join(s.dir, "workspace.json"), Version)
	return st, err
}

// SaveState persists tabs.
func (s *Store) SaveState(st State) error {
	if len(st.Tabs) > 200 {
		return apperr.New(apperr.CodeValidation, "número máximo de abas excedido")
	}
	for i := range st.Tabs {
		if len(st.Tabs[i].SQL) > MaxTabSQLBytes {
			return apperr.New(apperr.CodeValidation, "o conteúdo da aba \""+st.Tabs[i].Title+"\" é grande demais para ser salvo automaticamente; salve-o em um arquivo")
		}
		if st.Tabs[i].SavedContent != nil && len(*st.Tabs[i].SavedContent) > MaxTabSQLBytes {
			st.Tabs[i].SavedContent = nil
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return config.Save(filepath.Join(s.dir, "workspace.json"), Version, st)
}

// Queries lists saved queries.
func (s *Store) Queries() []SavedQuery {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]SavedQuery(nil), s.queries...)
}

// SaveQuery creates or updates a saved query.
func (s *Store) SaveQuery(q SavedQuery, newID func() string) (SavedQuery, error) {
	q.Name = strings.TrimSpace(q.Name)
	if q.Name == "" {
		return q, apperr.New(apperr.CodeValidation, "informe um nome para a consulta")
	}
	if len(q.SQL) > MaxTabSQLBytes {
		return q, apperr.New(apperr.CodeValidation, "consulta grande demais")
	}
	q.UpdatedAt = time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := append([]SavedQuery(nil), s.queries...)
	found := false
	for i := range s.queries {
		if s.queries[i].ID == q.ID && q.ID != "" {
			s.queries[i] = q
			found = true
		}
	}
	if !found {
		q.ID = newID()
		s.queries = append(s.queries, q)
	}
	if err := config.Save(filepath.Join(s.dir, "queries.json"), Version, queriesFile{Queries: s.queries}); err != nil {
		s.queries = prev
		return q, err
	}
	return q, nil
}

// DeleteQuery removes a saved query.
func (s *Store) DeleteQuery(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := append([]SavedQuery(nil), s.queries...)
	kept := s.queries[:0:0]
	for _, q := range s.queries {
		if q.ID != id {
			kept = append(kept, q)
		}
	}
	s.queries = kept
	if err := config.Save(filepath.Join(s.dir, "queries.json"), Version, queriesFile{Queries: s.queries}); err != nil {
		s.queries = prev
		return err
	}
	return nil
}

// MaxFileBytes limits .sql files opened in the editor.
const MaxFileBytes = 10 * 1024 * 1024

// ReadSQLFile reads a SQL file chosen by the user through the native dialog.
func ReadSQLFile(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() {
		return "", errors.New("não é um arquivo regular")
	}
	if st.Size() > MaxFileBytes {
		return "", apperr.New(apperr.CodeValidation, "arquivo grande demais para o editor (limite de 10 MB)")
	}
	b, err := os.ReadFile(path)
	return string(b), err
}

// WriteSQLFile writes content to a path chosen through the native dialog.
func WriteSQLFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

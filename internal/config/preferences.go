package config

import (
	"path/filepath"
	"sync"
)

// PreferencesVersion is the current schema version of preferences.json.
const PreferencesVersion = 1

// Preferences are user interface and behaviour settings.
type Preferences struct {
	Theme          string `json:"theme"` // "system" | "light" | "dark"
	EditorFontSize int    `json:"editorFontSize"`
	// MaxRows is the number of rows fetched per page of results.
	MaxRows int `json:"maxRows"`
	// QueryTimeoutSeconds limits statement execution (0 = no limit).
	QueryTimeoutSeconds int  `json:"queryTimeoutSeconds"`
	HistoryEnabled      bool `json:"historyEnabled"`
	HistoryLimit        int  `json:"historyLimit"`
	ConfirmDestructive  bool `json:"confirmDestructive"`
	ShowSystemSchemas   bool `json:"showSystemSchemas"`
	// HideWarnings skips SHOW WARNINGS after statements (false = show; the
	// negative form keeps the default for files written before the option).
	HideWarnings bool `json:"hideWarnings"`

	// ReconnectOnStartup reopens the sessions open when the app was closed.
	ReconnectOnStartup bool `json:"reconnectOnStartup"`

	// SQL editor.
	EditorTabSize  int  `json:"editorTabSize"`
	EditorUseTabs  bool `json:"editorUseTabs"`
	EditorWordWrap bool `json:"editorWordWrap"`
	EditorMinimap  bool `json:"editorMinimap"`
	// AutocompleteOff disables suggestions while typing (Ctrl+Space still works).
	AutocompleteOff bool `json:"autocompleteOff"`
	// UppercaseKeywords turns SQL keywords to upper case as they are typed.
	UppercaseKeywords bool `json:"uppercaseKeywords"`

	// Data grids.
	GridMaxColumnWidth int `json:"gridMaxColumnWidth"`
	// GridThousands shows numbers with digit grouping (display only).
	GridThousands bool `json:"gridThousands"`
	// TimestampColumns are integer columns shown as UNIX timestamps:
	// "<connection id>|<database>|<table>|<column>".
	TimestampColumns []string `json:"timestampColumns"`

	// SQLLogLimit is the number of entries kept by the SQL log panel.
	SQLLogLimit int `json:"sqlLogLimit"`

	// Shortcuts overrides the default key of application actions
	// (action id -> "Ctrl+Shift+K"; "" disables the action's key).
	Shortcuts map[string]string `json:"shortcuts"`
}

// DefaultPreferences returns the defaults.
func DefaultPreferences() Preferences {
	return Preferences{
		Theme:               "system",
		EditorFontSize:      14,
		MaxRows:             500,
		QueryTimeoutSeconds: 0,
		HistoryEnabled:      true,
		HistoryLimit:        1000,
		ConfirmDestructive:  true,
		ShowSystemSchemas:   false,
		EditorTabSize:       2,
		GridMaxColumnWidth:  380,
		SQLLogLimit:         5000,
		Shortcuts:           map[string]string{},
		TimestampColumns:    []string{},
	}
}

// Normalize clamps values into valid ranges.
func (p Preferences) Normalize() Preferences {
	d := DefaultPreferences()
	switch p.Theme {
	case "system", "light", "dark":
	default:
		p.Theme = d.Theme
	}
	if p.EditorFontSize < 8 || p.EditorFontSize > 40 {
		p.EditorFontSize = d.EditorFontSize
	}
	if p.MaxRows < 10 {
		p.MaxRows = 10
	}
	if p.MaxRows > 50000 {
		p.MaxRows = 50000
	}
	if p.QueryTimeoutSeconds < 0 {
		p.QueryTimeoutSeconds = 0
	}
	if p.HistoryLimit < 10 {
		p.HistoryLimit = 10
	}
	if p.HistoryLimit > 100000 {
		p.HistoryLimit = 100000
	}
	if p.EditorTabSize < 1 || p.EditorTabSize > 8 {
		p.EditorTabSize = d.EditorTabSize
	}
	if p.GridMaxColumnWidth == 0 {
		p.GridMaxColumnWidth = d.GridMaxColumnWidth
	}
	p.GridMaxColumnWidth = min(max(p.GridMaxColumnWidth, 80), 2000)
	if p.SQLLogLimit == 0 {
		p.SQLLogLimit = d.SQLLogLimit
	}
	p.SQLLogLimit = min(max(p.SQLLogLimit, 100), 100000)
	clean := map[string]string{}
	for k, v := range p.Shortcuts {
		if len(k) <= 64 && len(v) <= 64 {
			clean[k] = v
		}
	}
	p.Shortcuts = clean
	seen := map[string]bool{}
	cols := []string{}
	for _, c := range p.TimestampColumns {
		if c != "" && len(c) <= 512 && !seen[c] && len(cols) < 2000 {
			seen[c] = true
			cols = append(cols, c)
		}
	}
	p.TimestampColumns = cols
	return p
}

// PreferencesStore persists preferences.
type PreferencesStore struct {
	mu   sync.RWMutex
	path string
	p    Preferences
}

// OpenPreferences loads preferences from dir. Load problems are returned
// together with a usable store holding defaults.
func OpenPreferences(dir string) (*PreferencesStore, error) {
	s := &PreferencesStore{path: filepath.Join(dir, "preferences.json"), p: DefaultPreferences()}
	p, ok, err := Load[Preferences](s.path, PreferencesVersion)
	if ok {
		s.p = p.Normalize()
	}
	return s, err
}

// Get returns a copy of the preferences.
func (s *PreferencesStore) Get() Preferences {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.p
}

// Set validates, stores and persists preferences.
func (s *PreferencesStore) Set(p Preferences) (Preferences, error) {
	p = p.Normalize()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := Save(s.path, PreferencesVersion, p); err != nil {
		return s.p, err
	}
	s.p = p
	return p, nil
}

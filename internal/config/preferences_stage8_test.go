package config

import "testing"

func TestPreferencesNewFieldsDefaults(t *testing.T) {
	// A file written before the options existed decodes with zero values.
	p := Preferences{Theme: "dark", EditorFontSize: 14, MaxRows: 500, HistoryLimit: 100}.Normalize()
	if p.EditorTabSize != 2 || p.GridMaxColumnWidth != 380 || p.SQLLogLimit != 5000 || p.Shortcuts == nil || p.TimestampColumns == nil {
		t.Fatalf("%+v", p)
	}
	p = Preferences{EditorTabSize: 99, GridMaxColumnWidth: 5, SQLLogLimit: 1 << 30, TimestampColumns: []string{"a", "a", ""}, Shortcuts: map[string]string{"newTab": "Ctrl+N"}}.Normalize()
	if p.EditorTabSize != 2 || p.GridMaxColumnWidth != 80 || p.SQLLogLimit != 100000 || len(p.TimestampColumns) != 1 || p.Shortcuts["newTab"] != "Ctrl+N" {
		t.Fatalf("%+v", p)
	}
}

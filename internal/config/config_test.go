package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPreferencesRoundTripAndNormalize(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenPreferences(dir)
	if err != nil || !reflect.DeepEqual(s.Get(), DefaultPreferences()) {
		t.Fatalf("defaults: %v %+v", err, s.Get())
	}
	p := s.Get()
	p.Theme, p.MaxRows, p.HistoryEnabled = "dark", 5, false
	got, err := s.Set(p)
	if err != nil || got.MaxRows != 10 || got.Theme != "dark" {
		t.Fatalf("normalize: %v %+v", err, got)
	}
	s2, _ := OpenPreferences(dir)
	if !reflect.DeepEqual(s2.Get(), got) {
		t.Fatal("not persisted")
	}
	p.Theme = "neon"
	got, _ = s2.Set(p)
	if got.Theme != "system" {
		t.Fatal("invalid theme accepted")
	}
}

func TestNewerVersionRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	if err := Save(path, 99, map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load[map[string]int](path, 1); !errors.Is(err, ErrNewerVersion) {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("file from newer version must not be touched")
	}
}

func TestMissingFile(t *testing.T) {
	v, ok, err := Load[int](filepath.Join(t.TempDir(), "missing.json"), 1)
	if v != 0 || ok || err != nil {
		t.Fatal("missing file should yield zero value without error")
	}
}

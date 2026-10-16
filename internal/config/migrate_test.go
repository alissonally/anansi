package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLegacyDir(t *testing.T) {
	base := t.TempDir()
	old, dir := filepath.Join(base, legacyDirName), filepath.Join(base, AppDirName)
	if err := os.MkdirAll(old, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "connections.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	migrateLegacyDir(old, dir)
	if _, err := os.Stat(filepath.Join(dir, "connections.json")); err != nil {
		t.Fatal("not moved:", err)
	}
	// An existing new directory is never replaced.
	_ = os.MkdirAll(old, 0o700)
	_ = os.WriteFile(filepath.Join(old, "x"), nil, 0o600)
	migrateLegacyDir(old, dir)
	if _, err := os.Stat(filepath.Join(old, "x")); err != nil {
		t.Fatal("old dir touched while new exists")
	}
}

// Package config handles local, versioned configuration files: locating the
// application directories and reading/writing JSON documents atomically.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// AppDirName is the directory name under the user's config dir.
const AppDirName = "dbstudio"

// Dir returns the configuration directory (respecting $XDG_CONFIG_HOME) and
// creates it with owner-only permissions. DBSTUDIO_CONFIG_DIR overrides it.
func Dir() (string, error) {
	dir := os.Getenv("DBSTUDIO_CONFIG_DIR")
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(base, AppDirName)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// Envelope wraps every persisted document with a schema version.
type Envelope[T any] struct {
	Version int `json:"version"`
	Data    T   `json:"data"`
}

// ErrNewerVersion is returned when a file was written by a newer version of
// the application.
var ErrNewerVersion = errors.New("arquivo de configuração criado por uma versão mais nova do aplicativo")

// Load reads path into an envelope. A missing file yields (zero, false, nil).
// A corrupt file is moved aside (path.corrupt-<timestamp>) so the user does
// not lose it, and an error describing the problem is returned together with
// the zero value.
func Load[T any](path string, currentVersion int) (T, bool, error) {
	var zero T
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return zero, false, nil
	}
	if err != nil {
		return zero, false, err
	}
	var env Envelope[T]
	if err := json.Unmarshal(b, &env); err != nil {
		backup := fmt.Sprintf("%s.corrupt-%s", path, time.Now().Format("20060102-150405"))
		_ = os.Rename(path, backup)
		return zero, false, fmt.Errorf("arquivo %s inválido (cópia salva em %s): %w", filepath.Base(path), filepath.Base(backup), err)
	}
	if env.Version > currentVersion {
		return zero, false, ErrNewerVersion
	}
	return env.Data, true, nil
}

// Save atomically writes data wrapped in an envelope with owner-only
// permissions.
func Save[T any](path string, version int, data T) error {
	b, err := json.MarshalIndent(Envelope[T]{Version: version, Data: data}, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, b, 0o600)
}

// WriteFileAtomic writes b to path through a temporary file, fsync and
// rename, so readers never see a partial file.
func WriteFileAtomic(path string, b []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

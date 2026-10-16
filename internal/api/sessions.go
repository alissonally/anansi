package api

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"anansi/internal/apperr"
	"anansi/internal/config"
)

// TrustSSHHost records the key last presented by an SSH server (after an
// ssh_host_unknown error) in the application's known_hosts.
func (a *App) TrustSSHHost(host, fingerprint string) error {
	return wrap(a.hostKeys.Trust(host, fingerprint))
}

// KnownSSHHosts lists the hosts trusted in the application.
func (a *App) KnownSSHHosts() []string { return a.hostKeys.Known() }

// SetSSHSecret provides the SSH password/passphrase asked at connect time.
func (a *App) SetSSHSecret(id, secret string, remember bool) error {
	if err := checkConnID(id); err != nil {
		return err
	}
	return wrap(a.conns.SetSSHSecret(id, secret, remember))
}

var connFilters = []runtime.FileFilter{{DisplayName: "Conexões (*.json)", Pattern: "*.json"}}

// ExportConnections writes the chosen profiles (all when ids is empty) to a
// JSON file chosen by the user. Passwords are never exported. Returns the
// path ("" when cancelled).
func (a *App) ExportConnections(ids []string) (string, error) {
	data, n, err := a.conns.Export(ids)
	if err != nil {
		return "", wrap(err)
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "Exportar conexões", DefaultFilename: "conexoes-anansi.json", Filters: connFilters})
	if err != nil || path == "" {
		return "", wrap(err)
	}
	if !strings.HasSuffix(strings.ToLower(path), ".json") {
		path += ".json"
	}
	if err := config.WriteFileAtomic(path, data, 0o600); err != nil {
		return "", wrap(err)
	}
	a.log.Info("connections exported", "count", n)
	return path, nil
}

// ImportResult reports an import.
type ImportResult struct {
	Imported int      `json:"imported"`
	Problems []string `json:"problems"`
}

// ImportConnections reads an exported file chosen by the user.
func (a *App) ImportConnections() (*ImportResult, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Importar conexões", Filters: connFilters})
	if err != nil || path == "" {
		return nil, wrap(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, wrap(err)
	}
	if st.Size() > 8<<20 {
		return nil, apperr.New(apperr.CodeValidation, "arquivo grande demais para conexões: "+filepath.Base(path))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, wrap(err)
	}
	n, problems, err := a.conns.Import(data)
	if err != nil {
		return nil, wrap(err)
	}
	return &ImportResult{Imported: n, Problems: problems}, nil
}

// PickKeyFile chooses an SSH private key ("" when cancelled).
func (a *App) PickKeyFile() (string, error) {
	start := ""
	if home, err := os.UserHomeDir(); err == nil {
		start = filepath.Join(home, ".ssh")
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Chave privada SSH", DefaultDirectory: start, ShowHiddenFiles: true})
	return path, wrap(err)
}

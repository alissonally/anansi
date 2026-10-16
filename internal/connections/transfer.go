package connections

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"dbstudio/internal/apperr"
)

// ExportFile is the format of exported connection settings. It never
// contains passwords or SSH secrets.
type ExportFile struct {
	App        string    `json:"app"`
	Version    int       `json:"version"`
	ExportedAt time.Time `json:"exportedAt"`
	Profiles   []Profile `json:"profiles"`
}

const exportApp = "dbstudio-connections"

// Export serializes the given profiles (all when ids is empty).
func (s *Service) Export(ids []string) ([]byte, int, error) {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var out []Profile
	for _, p := range s.Store.List() {
		if len(want) > 0 && !want[p.ID] {
			continue
		}
		p.Stats = ConnStats{}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, 0, apperr.New(apperr.CodeValidation, "nenhuma conexão para exportar")
	}
	b, err := json.MarshalIndent(ExportFile{App: exportApp, Version: ProfilesVersion, ExportedAt: time.Now().UTC(), Profiles: out}, "", "  ")
	return b, len(out), err
}

// Import adds the profiles of an exported file with new ids; names that
// already exist get a numeric suffix. Invalid profiles are reported and
// skipped.
func (s *Service) Import(data []byte) (int, []string, error) {
	var f ExportFile
	if err := json.Unmarshal(data, &f); err != nil {
		return 0, nil, apperr.New(apperr.CodeValidation, "arquivo de conexões inválido: "+err.Error())
	}
	if f.App != exportApp {
		return 0, nil, apperr.New(apperr.CodeValidation, "o arquivo não é uma exportação de conexões do Database Studio")
	}
	if f.Version > ProfilesVersion {
		return 0, nil, apperr.New(apperr.CodeValidation, "o arquivo foi gerado por uma versão mais nova do aplicativo")
	}
	names := map[string]bool{}
	for _, p := range s.Store.List() {
		names[strings.ToLower(p.Name)] = true
	}
	n := 0
	var problems []string
	for _, p := range f.Profiles {
		p.ID = ""
		base := strings.TrimSpace(p.Name)
		for i := 2; names[strings.ToLower(p.Name)]; i++ {
			p.Name = fmt.Sprintf("%s (%d)", base, i)
		}
		if _, err := s.Save(SaveInput{Profile: p}); err != nil {
			msg := err.Error()
			var ae *apperr.Error
			if errors.As(err, &ae) {
				msg = ae.Message
			}
			problems = append(problems, base+": "+msg)
			continue
		}
		names[strings.ToLower(p.Name)] = true
		n++
	}
	return n, problems, nil
}

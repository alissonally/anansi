package connections

import (
	"context"
	"errors"
	"strings"

	"anansi/internal/apperr"
	"anansi/internal/database"
	"anansi/internal/security"
)

// ProfileView is a profile plus credential status, safe to send to the UI.
type ProfileView struct {
	Profile
	HasStoredPassword  bool `json:"hasStoredPassword"`
	HasStoredSSHSecret bool `json:"hasStoredSSHSecret"`
}

// SaveInput is the payload of SaveProfile. Password and SSHSecret are nil
// when unchanged.
type SaveInput struct {
	Profile   Profile `json:"profile"`
	Password  *string `json:"password"`
	SSHSecret *string `json:"sshSecret"`
}

// SaveResult reports the saved profile and any non-fatal warning.
type SaveResult struct {
	Profile ProfileView `json:"profile"`
	Warning string      `json:"warning,omitempty"`
}

// Service coordinates profiles, credentials and sessions.
type Service struct {
	Store    *ProfileStore
	Secrets  security.SecretStore // persistent (keyring)
	Memory   *security.Memory     // session-only fallback, never written to disk
	Manager  *Manager
	Registry *database.Registry
}

func secretKey(id string) string { return "connection:" + id }

func sshKey(id string) string { return "ssh:" + id }

// List returns all profiles.
func (s *Service) List() []ProfileView {
	ps := s.Store.List()
	out := make([]ProfileView, len(ps))
	for i, p := range ps {
		out[i] = s.view(p)
	}
	return out
}

func (s *Service) view(p Profile) ProfileView {
	has := false
	if _, err := s.Memory.Get(secretKey(p.ID)); err == nil {
		has = true
	} else if p.SavePassword && s.Secrets.Available() {
		if _, err := s.Secrets.Get(secretKey(p.ID)); err == nil {
			has = true
		}
	}
	_, sshErr := s.sshSecret(p)
	return ProfileView{Profile: p, HasStoredPassword: has, HasStoredSSHSecret: p.SSH.Enabled && sshErr == nil}
}

// sshSecret resolves the SSH password/passphrase from memory or keyring.
func (s *Service) sshSecret(p Profile) (string, error) {
	if v, err := s.Memory.Get(sshKey(p.ID)); err == nil {
		return v, nil
	}
	if p.SSH.SaveSecret && s.Secrets.Available() {
		return s.Secrets.Get(sshKey(p.ID))
	}
	return "", security.ErrNotFound
}

// SetSSHSecret records the SSH password/passphrase asked at connect time.
func (s *Service) SetSSHSecret(id, secret string, remember bool) error {
	p, err := s.Store.Get(id)
	if err != nil {
		return err
	}
	_ = s.Memory.Set(sshKey(id), secret)
	if remember && p.SSH.SaveSecret && s.Secrets.Available() {
		if err := s.Secrets.Set(sshKey(id), secret); err != nil {
			return apperr.New(apperr.CodeKeyringUnavailable, "não foi possível salvar a senha SSH no keyring")
		}
	}
	return nil
}

// KeyringAvailable reports whether passwords can be persisted.
func (s *Service) KeyringAvailable() bool { return s.Secrets.Available() }

func (s *Service) validate(p Profile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	drv, err := s.Registry.Get(p.Kind)
	if err != nil {
		return apperr.New(apperr.CodeValidation, err.Error())
	}
	if err := drv.Validate(p.ConnectParams("")); err != nil {
		return apperr.Wrap(err)
	}
	return nil
}

// Save creates or updates a profile and its credential.
func (s *Service) Save(in SaveInput) (SaveResult, error) {
	p := in.Profile.Normalize()
	p.Stats = ConnStats{} // maintained by the store
	if p.ID == "" {
		p.ID = NewID()
	} else if !ValidID(p.ID) {
		return SaveResult{}, apperr.New(apperr.CodeValidation, "identificador de conexão inválido")
	} else if _, err := s.Store.Get(p.ID); err != nil {
		return SaveResult{}, err
	}
	if err := s.validate(p); err != nil {
		return SaveResult{}, err
	}
	warning := ""
	key := secretKey(p.ID)
	if p.SavePassword && !s.Secrets.Available() {
		p.SavePassword = false
		warning = "O keyring do sistema não está disponível. A senha não foi salva e será solicitada ao conectar (fica apenas na memória até o aplicativo ser fechado)."
	}
	saved, err := s.Store.Put(p)
	if err != nil {
		return SaveResult{}, apperr.Wrap(err)
	}
	if in.Password != nil {
		_ = s.Memory.Set(key, *in.Password)
	}
	if saved.SavePassword {
		if in.Password != nil {
			if err := s.Secrets.Set(key, *in.Password); err != nil {
				warning = "Não foi possível salvar a senha no keyring; ela será solicitada ao conectar."
			}
		}
	} else if err := s.Secrets.Delete(key); err != nil && !errors.Is(err, security.ErrUnavailable) {
		warning = "Não foi possível remover a senha antiga do keyring."
	}
	if in.SSHSecret != nil {
		_ = s.Memory.Set(sshKey(p.ID), *in.SSHSecret)
	}
	if saved.SSH.Enabled && saved.SSH.SaveSecret {
		if in.SSHSecret != nil && s.Secrets.Available() {
			if err := s.Secrets.Set(sshKey(p.ID), *in.SSHSecret); err != nil {
				warning = "Não foi possível salvar a senha SSH no keyring; ela será solicitada ao conectar."
			}
		}
	} else {
		_ = s.Secrets.Delete(sshKey(p.ID))
	}
	return SaveResult{Profile: s.view(saved), Warning: warning}, nil
}

// Duplicate copies a profile (and its stored password, if any).
func (s *Service) Duplicate(id string) (ProfileView, error) {
	p, err := s.Store.Get(id)
	if err != nil {
		return ProfileView{}, err
	}
	pw, pwErr := s.password(p)
	sshSecret, sshErr := s.sshSecret(p)
	p.ID = "" // Save assigns a new id
	p.Name = strings.TrimSpace(p.Name + " (cópia)")
	if r := []rune(p.Name); len(r) > 100 {
		p.Name = string(r[:100])
	}
	in := SaveInput{Profile: p}
	if pwErr == nil {
		in.Password = &pw
	}
	if sshErr == nil {
		in.SSHSecret = &sshSecret
	}
	res, err := s.Save(in)
	return res.Profile, err
}

// Delete disconnects and removes a profile and its secrets.
func (s *Service) Delete(id string) error {
	if _, err := s.Store.Get(id); err != nil {
		return err
	}
	_ = s.Manager.Disconnect(id)
	if err := s.Store.Delete(id); err != nil {
		return apperr.Wrap(err)
	}
	_ = s.Memory.Delete(secretKey(id))
	_ = s.Secrets.Delete(secretKey(id))
	_ = s.Memory.Delete(sshKey(id))
	_ = s.Secrets.Delete(sshKey(id))
	return nil
}

// password resolves the password for p from memory or the keyring.
func (s *Service) password(p Profile) (string, error) {
	if pw, err := s.Memory.Get(secretKey(p.ID)); err == nil {
		return pw, nil
	}
	if p.SavePassword {
		pw, err := s.Secrets.Get(secretKey(p.ID))
		if err == nil {
			return pw, nil
		}
		if errors.Is(err, security.ErrUnavailable) {
			return "", apperr.New(apperr.CodePasswordRequired, "O keyring do sistema não está disponível; informe a senha.")
		}
	}
	return "", apperr.New(apperr.CodePasswordRequired, "Informe a senha para conectar.")
}

// TestInput is the payload of Test: an unsaved or edited profile.
type TestInput struct {
	Profile   Profile `json:"profile"`
	Password  *string `json:"password"`
	SSHSecret *string `json:"sshSecret"`
}

// Test opens a temporary connection and reports the server info.
func (s *Service) Test(ctx context.Context, in TestInput) (database.ServerInfo, error) {
	p := in.Profile.Normalize()
	if p.Name == "" {
		p.Name = "teste"
	}
	if err := s.validate(p); err != nil {
		return database.ServerInfo{}, err
	}
	var pw string
	if in.Password != nil {
		pw = *in.Password
	} else if ValidID(p.ID) {
		stored, err := s.Store.Get(p.ID)
		if err != nil {
			return database.ServerInfo{}, err
		}
		if pw, err = s.password(stored); err != nil {
			return database.ServerInfo{}, err
		}
	}
	if p.SSH.Enabled {
		if in.SSHSecret != nil {
			ctx = WithSSHSecret(ctx, *in.SSHSecret)
		} else if ValidID(p.ID) {
			if stored, err := s.Store.Get(p.ID); err == nil {
				if v, err := s.sshSecret(stored); err == nil {
					ctx = WithSSHSecret(ctx, v)
				}
			}
		}
	}
	tmp := NewManager(s.Manager.open)
	p.ID = "test"
	sess, err := tmp.Connect(ctx, p, pw)
	if err != nil {
		return database.ServerInfo{}, err
	}
	info := sess.Server
	_ = tmp.CloseAll(context.Background())
	return info, nil
}

// Connect opens (or reuses) a session. password is optional; when nil the
// stored password is used. When remember is true and the profile allows it
// the password is saved to the keyring after a successful connection.
func (s *Service) Connect(ctx context.Context, id string, password *string, remember bool) (SessionState, error) {
	p, err := s.Store.Get(id)
	if err != nil {
		return SessionState{}, err
	}
	var pw string
	if password != nil {
		pw = *password
	} else if pw, err = s.password(p); err != nil {
		return SessionState{}, err
	}
	if p.SSH.Enabled {
		if v, err := s.sshSecret(p); err == nil {
			ctx = WithSSHSecret(ctx, v)
		}
	}
	wasOpen := s.Manager.isOpen(id)
	sess, err := s.Manager.Connect(ctx, p, pw)
	if !wasOpen {
		s.Store.RecordConnect(id, err == nil)
	}
	if err != nil {
		var ae *apperr.Error
		if errors.As(err, &ae) && ae.DB != nil && ae.DB.Category == "auth" {
			// A stale stored password must not be retried silently.
			_ = s.Memory.Delete(secretKey(id))
		}
		return SessionState{}, err
	}
	if password != nil {
		_ = s.Memory.Set(secretKey(id), pw)
		if remember && p.SavePassword {
			_ = s.Secrets.Set(secretKey(id), pw)
		}
	}
	return SessionState{ProfileID: id, Status: "connected", Server: sess.Server, OpenedAt: sess.OpenedAt}, nil
}

// PasswordChanged records a password changed on the server by the session's
// own account: it replaces the remembered password (memory and, when the
// profile saves it, the keyring). The open pool keeps its idle connections;
// reconnecting uses the new password.
func (s *Service) PasswordChanged(id, pw string) error {
	p, err := s.Store.Get(id)
	if err != nil {
		return err
	}
	_ = s.Memory.Set(secretKey(id), pw)
	if p.SavePassword && s.Secrets.Available() {
		if err := s.Secrets.Set(secretKey(id), pw); err != nil {
			return apperr.New(apperr.CodeKeyringUnavailable, "Senha alterada no servidor, mas o keyring não pôde ser atualizado: "+err.Error())
		}
	}
	return nil
}

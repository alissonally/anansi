// Package security provides credential storage and helpers that keep secrets
// out of logs, error messages and the frontend.
package security

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/zalando/go-keyring"
)

// ErrNotFound is returned when no secret exists for a key.
var ErrNotFound = errors.New("segredo não encontrado")

// ErrUnavailable is returned when the backing store cannot be used (for
// example, no Secret Service provider is running).
var ErrUnavailable = errors.New("o keyring do sistema não está disponível")

// SecretStore stores secrets keyed by an opaque identifier.
type SecretStore interface {
	Get(key string) (string, error)
	Set(key, secret string) error
	Delete(key string) error
	// Available reports whether the store can persist secrets.
	Available() bool
}

// Keyring stores secrets in the OS keyring (Secret Service on Linux, via
// D-Bus: GNOME Keyring or KWallet).
type Keyring struct {
	Service string
	// Legacy are previous service names; their secrets are read (and moved)
	// when the current service has none.
	Legacy []string

	once      sync.Once
	available bool
}

// NewKeyring creates a keyring-backed store.
func NewKeyring(service string, legacy ...string) *Keyring {
	return &Keyring{Service: service, Legacy: legacy}
}

// Available probes the keyring once.
func (k *Keyring) Available() bool {
	k.once.Do(func() {
		_, err := keyring.Get(k.Service, "__probe__")
		k.available = err == nil || errors.Is(err, keyring.ErrNotFound)
	})
	return k.available
}

// Get implements SecretStore.
func (k *Keyring) Get(key string) (string, error) {
	if !k.Available() {
		return "", ErrUnavailable
	}
	s, err := keyring.Get(k.Service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		// Secrets saved under a previous service name move to the current one.
		for _, old := range k.Legacy {
			if v, lerr := keyring.Get(old, key); lerr == nil {
				if keyring.Set(k.Service, key, v) == nil {
					_ = keyring.Delete(old, key)
				}
				return v, nil
			}
		}
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return s, nil
}

// Set implements SecretStore.
func (k *Keyring) Set(key, secret string) error {
	if !k.Available() {
		return ErrUnavailable
	}
	if err := keyring.Set(k.Service, key, secret); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

// Delete implements SecretStore. Deleting a missing key is not an error.
func (k *Keyring) Delete(key string) error {
	if !k.Available() {
		return nil
	}
	for _, old := range k.Legacy {
		_ = keyring.Delete(old, key)
	}
	err := keyring.Delete(k.Service, key)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

// Memory keeps secrets only in process memory. It is used as the session-only
// fallback when the keyring is unavailable (secrets are lost on exit, never
// written to disk) and as a test double.
type Memory struct {
	mu sync.Mutex
	m  map[string]string
}

// NewMemory creates an in-memory store.
func NewMemory() *Memory { return &Memory{m: map[string]string{}} }

// Available implements SecretStore. Memory storage is always usable but
// never persistent.
func (m *Memory) Available() bool { return true }

// Get implements SecretStore.
func (m *Memory) Get(key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.m[key]
	if !ok {
		return "", ErrNotFound
	}
	return s, nil
}

// Set implements SecretStore.
func (m *Memory) Set(key, secret string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[key] = secret
	return nil
}

// Delete implements SecretStore.
func (m *Memory) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.m, key)
	return nil
}

// Redact removes every occurrence of the given secrets from s. Empty and very
// short secrets are ignored to avoid mangling unrelated text.
func Redact(s string, secrets ...string) string {
	for _, sec := range secrets {
		if len(sec) < 3 {
			continue
		}
		s = strings.ReplaceAll(s, sec, "******")
	}
	return s
}

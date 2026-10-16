package security

import (
	"errors"
	"os"
	"testing"
)

func TestRedact(t *testing.T) {
	if got := Redact("Access denied for 'u' using s3cret!", "s3cret!"); got != "Access denied for 'u' using ******" {
		t.Fatal(got)
	}
	if got := Redact("abc", ""); got != "abc" {
		t.Fatal(got)
	}
}

func TestMemoryStore(t *testing.T) {
	m := NewMemory()
	if _, err := m.Get("k"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	_ = m.Set("k", "v")
	if v, _ := m.Get("k"); v != "v" {
		t.Fatal(v)
	}
	_ = m.Delete("k")
	if _, err := m.Get("k"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

// TestSystemKeyring talks to the real Secret Service (GNOME Keyring /
// KWallet). Opt-in: ANANSI_TEST_KEYRING=1 go test ./internal/security/
func TestSystemKeyring(t *testing.T) {
	if os.Getenv("ANANSI_TEST_KEYRING") != "1" {
		t.Skip("set ANANSI_TEST_KEYRING=1 to test the system keyring")
	}
	k := NewKeyring("anansi-test")
	if !k.Available() {
		t.Fatal("keyring not available")
	}
	key := "connection:test-roundtrip"
	if err := k.Set(key, "p@ss wörd"); err != nil {
		t.Fatal(err)
	}
	if v, err := k.Get(key); err != nil || v != "p@ss wörd" {
		t.Fatalf("%q %v", v, err)
	}
	if err := k.Delete(key); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Get(key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

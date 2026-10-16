package connections

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"dbstudio/internal/apperr"
	"dbstudio/internal/database"
	"dbstudio/internal/database/mysql"
	"dbstudio/internal/security"
)

// unavailableKeyring simulates a system without a Secret Service provider.
type unavailableKeyring struct{}

func (unavailableKeyring) Get(string) (string, error) { return "", security.ErrUnavailable }
func (unavailableKeyring) Set(string, string) error   { return security.ErrUnavailable }
func (unavailableKeyring) Delete(string) error        { return nil }
func (unavailableKeyring) Available() bool            { return false }

func newService(t *testing.T, secrets security.SecretStore, open Opener) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := OpenProfileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if open == nil {
		open = func(ctx context.Context, p Profile, pw string) (*Session, error) {
			return &Session{ID: p.ID, Profile: p, password: pw}, nil
		}
	}
	return &Service{Store: store, Secrets: secrets, Memory: security.NewMemory(), Manager: NewManager(open),
		Registry: database.NewRegistry(mysql.New())}, dir
}

func sample() Profile {
	return Profile{Name: "Local", Kind: database.MySQL, Host: "127.0.0.1", Port: 3306, User: "root", SavePassword: true}
}

func TestPasswordNeverWrittenToConfig(t *testing.T) {
	keyring := security.NewMemory()
	svc, dir := newService(t, keyring, nil)
	pw := "S3cr3t-P@ss"
	res, err := svc.Save(SaveInput{Profile: sample(), Password: &pw})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "connections.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), pw) {
		t.Fatal("password written to connections.json")
	}
	if st, _ := os.Stat(filepath.Join(dir, "connections.json")); st.Mode().Perm() != 0o600 {
		t.Fatalf("config permissions %v", st.Mode().Perm())
	}
	if got, _ := keyring.Get("connection:" + res.Profile.ID); got != pw {
		t.Fatal("password not stored in keyring")
	}
	if !res.Profile.HasStoredPassword {
		t.Fatal("HasStoredPassword should be true")
	}
	// Turning SavePassword off removes it from the keyring.
	p := res.Profile.Profile
	p.SavePassword = false
	if _, err := svc.Save(SaveInput{Profile: p}); err != nil {
		t.Fatal(err)
	}
	if _, err := keyring.Get("connection:" + p.ID); !errors.Is(err, security.ErrNotFound) {
		t.Fatal("password should be deleted from keyring")
	}
}

func TestKeyringUnavailableDoesNotFallBackToDisk(t *testing.T) {
	svc, dir := newService(t, unavailableKeyring{}, nil)
	pw := "Another-Secret"
	res, err := svc.Save(SaveInput{Profile: sample(), Password: &pw})
	if err != nil {
		t.Fatal(err)
	}
	if res.Warning == "" || res.Profile.SavePassword {
		t.Fatalf("expected warning and SavePassword=false: %+v", res)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "connections.json"))
	if strings.Contains(string(b), pw) {
		t.Fatal("password written to disk")
	}
	// The password is kept in memory for this session only.
	if _, err := svc.Connect(context.Background(), res.Profile.ID, nil, false); err != nil {
		t.Fatalf("connect with in-memory password: %v", err)
	}
	// A fresh service (= app restart) must ask for the password.
	store, _ := OpenProfileStore(dir)
	svc2 := &Service{Store: store, Secrets: unavailableKeyring{}, Memory: security.NewMemory(), Manager: svc.Manager, Registry: svc.Registry}
	_ = svc.Manager.Disconnect(res.Profile.ID)
	_, err = svc2.Connect(context.Background(), res.Profile.ID, nil, false)
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Code != apperr.CodePasswordRequired {
		t.Fatalf("expected password_required, got %v", err)
	}
}

func TestProfilePersistenceAndReload(t *testing.T) {
	svc, dir := newService(t, security.NewMemory(), nil)
	a, _ := svc.Save(SaveInput{Profile: sample()})
	pb := sample()
	pb.Name = "Prod"
	pb.Environment = EnvProduction
	b, _ := svc.Save(SaveInput{Profile: pb})
	if err := svc.Store.Reorder([]string{b.Profile.ID, a.Profile.ID}); err != nil {
		t.Fatal(err)
	}
	store, err := OpenProfileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	list := store.List()
	if len(list) != 2 || list[0].Name != "Prod" || list[0].Environment != EnvProduction || list[1].TLS.Mode != database.TLSPreferred {
		t.Fatalf("reloaded: %+v", list)
	}
}

func TestCorruptConfigIsPreserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "connections.json")
	_ = os.WriteFile(path, []byte("{not json"), 0o600)
	store, err := OpenProfileStore(dir)
	if err == nil || len(store.List()) != 0 {
		t.Fatal("expected error and empty store")
	}
	matches, _ := filepath.Glob(path + ".corrupt-*")
	if len(matches) != 1 {
		t.Fatal("corrupt file should be kept aside")
	}
}

func TestValidation(t *testing.T) {
	svc, _ := newService(t, security.NewMemory(), nil)
	bad := []Profile{
		{Name: "", Host: "h", User: "u"},
		{Name: "x", Host: "", User: "u"},
		{Name: "x", Host: "h", User: ""},
		{Name: "x", Host: "h", User: "u", Port: 70000},
		{Name: "x", Host: "h", User: "u", Params: map[string]string{"allowAllFiles": "true"}},
		{Name: "x", Host: "h", User: "u", Params: map[string]string{"multiStatements": "true"}},
		{Name: "x", Host: "h", User: "u", Params: map[string]string{"sql_mode": "x; DROP"}},
		{Name: "x", Host: "h", User: "u", AllowCleartextPasswords: true},
		{Name: "x", Host: "h", User: "u", TLS: database.TLSOptions{Mode: database.TLSVerifyCA}},
		{Name: "x", Host: "h", User: "u", Environment: "moon"},
	}
	for i, p := range bad {
		if _, err := svc.Save(SaveInput{Profile: p}); err == nil {
			t.Errorf("case %d accepted: %+v", i, p)
		}
	}
	if _, err := svc.Save(SaveInput{Profile: Profile{ID: "../../etc", Name: "x", Host: "h", User: "u"}}); err == nil {
		t.Error("invalid id accepted")
	}
}

func TestDuplicateAndDelete(t *testing.T) {
	keyring := security.NewMemory()
	svc, _ := newService(t, keyring, nil)
	pw := "pw-123"
	a, _ := svc.Save(SaveInput{Profile: sample(), Password: &pw})
	d, err := svc.Duplicate(a.Profile.ID)
	if err != nil || d.ID == a.Profile.ID || !strings.HasSuffix(d.Name, "(cópia)") {
		t.Fatalf("duplicate: %v %+v", err, d)
	}
	if got, _ := keyring.Get("connection:" + d.ID); got != pw {
		t.Fatal("duplicate should copy the stored password")
	}
	if _, err := svc.Connect(context.Background(), d.ID, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(d.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := keyring.Get("connection:" + d.ID); !errors.Is(err, security.ErrNotFound) {
		t.Fatal("password not removed on delete")
	}
	if len(svc.Manager.States()) != 0 {
		t.Fatal("session not closed on delete")
	}
}

func TestManagerConcurrency(t *testing.T) {
	var opens int32
	open := func(ctx context.Context, p Profile, pw string) (*Session, error) {
		atomic.AddInt32(&opens, 1)
		time.Sleep(20 * time.Millisecond) // simulated network latency
		return &Session{ID: p.ID, Profile: p}, nil
	}
	m := NewManager(open)
	var closed int32
	m.OnClose(func(string) { atomic.AddInt32(&closed, 1) })
	profiles := []Profile{{ID: NewID()}, {ID: NewID()}, {ID: NewID()}}
	var wg sync.WaitGroup
	for i := 0; i < 60; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p := profiles[i%3]
			if _, err := m.Connect(context.Background(), p, ""); err != nil {
				t.Error(err)
			}
			_ = m.States()
			_, _ = m.Get(p.ID)
		}(i)
	}
	wg.Wait()
	if opens != 3 {
		t.Fatalf("expected 3 opens (one per profile), got %d", opens)
	}
	if len(m.States()) != 3 {
		t.Fatal("expected 3 sessions")
	}
	// Disconnect concurrently; hooks run once per session.
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); _ = m.Disconnect(profiles[i%3].ID) }(i)
	}
	wg.Wait()
	if closed != 3 || len(m.States()) != 0 {
		t.Fatalf("closed=%d states=%d", closed, len(m.States()))
	}
	// After CloseAll no new sessions are accepted.
	_ = m.CloseAll(context.Background())
	if _, err := m.Connect(context.Background(), profiles[0], ""); err == nil {
		t.Fatal("connect after CloseAll must fail")
	}
}

func TestManagerConnectFailureIsNotCached(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	m := NewManager(func(ctx context.Context, p Profile, pw string) (*Session, error) {
		if fail.Load() {
			return nil, errors.New("refused")
		}
		return &Session{ID: p.ID}, nil
	})
	p := Profile{ID: NewID()}
	if _, err := m.Connect(context.Background(), p, ""); err == nil {
		t.Fatal("expected failure")
	}
	fail.Store(false)
	if _, err := m.Connect(context.Background(), p, ""); err != nil {
		t.Fatal(err)
	}
}

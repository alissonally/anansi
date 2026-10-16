package connections

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dbstudio/internal/apperr"
	"dbstudio/internal/security"
)

func TestExportImportWithoutSecrets(t *testing.T) {
	keyring := security.NewMemory()
	svc, dir := newService(t, keyring, nil)
	pw, sshPw := "db-S3cret", "ssh-S3cret"
	p := sample()
	p.Folder = " Clientes / ACME "
	p.Comment = "produção do cliente"
	p.SSH = SSHOptions{Enabled: true, Host: "bastion.example.com", User: "deploy", Auth: "password", SaveSecret: true}
	res, err := svc.Save(SaveInput{Profile: p, Password: &pw, SSHSecret: &sshPw})
	if err != nil {
		t.Fatal(err)
	}
	if res.Profile.Folder != "Clientes/ACME" || res.Profile.SSH.Port != 22 || !res.Profile.HasStoredSSHSecret {
		t.Fatalf("%+v", res.Profile)
	}
	if v, _ := keyring.Get(sshKey(res.Profile.ID)); v != sshPw {
		t.Fatal("ssh secret not in keyring")
	}
	cfg, _ := os.ReadFile(filepath.Join(dir, "connections.json"))
	data, n, err := svc.Export(nil)
	if err != nil || n != 1 {
		t.Fatal(n, err)
	}
	for _, b := range [][]byte{cfg, data} {
		if strings.Contains(string(b), pw) || strings.Contains(string(b), sshPw) {
			t.Fatal("secret written")
		}
	}
	got, problems, err := svc.Import(data)
	if err != nil || got != 1 || len(problems) != 0 {
		t.Fatal(got, problems, err)
	}
	list := svc.List()
	if len(list) != 2 || list[1].Name != "Local (2)" || list[1].ID == list[0].ID || list[1].SSH.Host != "bastion.example.com" || list[1].HasStoredPassword || list[1].HasStoredSSHSecret {
		t.Fatalf("%+v", list[1])
	}
	if _, _, err := svc.Import([]byte(`{"app":"x"}`)); err == nil {
		t.Fatal("foreign file accepted")
	}
	// Deleting removes the SSH secret too.
	if err := svc.Delete(res.Profile.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := keyring.Get(sshKey(res.Profile.ID)); err == nil {
		t.Fatal("ssh secret left in keyring")
	}
}

func TestProfileValidationSessions(t *testing.T) {
	svc, _ := newService(t, security.NewMemory(), nil)
	cases := []struct {
		mut  func(*Profile)
		want string
	}{
		{func(p *Profile) {
			p.Socket = "/run/mysqld.sock"
			p.SSH = SSHOptions{Enabled: true, Host: "h", User: "u"}
		}, "socket"},
		{func(p *Profile) { p.SSH = SSHOptions{Enabled: true, Host: "h"} }, "usuário SSH"},
		{func(p *Profile) {
			p.SSH = SSHOptions{Enabled: true, Host: "h", User: "u", Auth: "key", KeyFile: "/nao/existe"}
		}, "chave SSH"},
		{func(p *Profile) { p.Socket = "relativo.sock" }, "absoluto"},
		{func(p *Profile) { p.InitSQL = "SET NAMES utf8mb4; START TRANSACTION" }, "transações"},
		{func(p *Profile) { p.ReadOnly = true; p.InitSQL = "DELETE FROM t" }, "somente leitura"},
	}
	for _, c := range cases {
		p := sample()
		c.mut(&p)
		_, err := svc.Save(SaveInput{Profile: p})
		var ae *apperr.Error
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("want %q, got %v %v", c.want, err, ae)
		}
	}
	p := sample()
	p.ReadOnly = true
	p.InitSQL = "SET time_zone = '+00:00'; SELECT 1"
	p.Socket = "/run/mysqld/mysqld.sock"
	p.Host = ""
	if _, err := svc.Save(SaveInput{Profile: p}); err != nil {
		t.Fatal(err)
	}
}

func TestConnectStatistics(t *testing.T) {
	svc, _ := newService(t, security.NewMemory(), nil)
	pw := "x"
	res, err := svc.Save(SaveInput{Profile: sample(), Password: &pw})
	if err != nil {
		t.Fatal(err)
	}
	id := res.Profile.ID
	if _, err := svc.Connect(context.Background(), id, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Connect(context.Background(), id, nil, false); err != nil { // reuses the session
		t.Fatal(err)
	}
	p, _ := svc.Store.Get(id)
	if p.Stats.Connects != 1 || p.Stats.LastConnectedAt.IsZero() {
		t.Fatalf("%+v", p.Stats)
	}
	// Saving from the UI does not reset or forge statistics.
	p.Stats.Connects = 99
	if _, err := svc.Save(SaveInput{Profile: p}); err != nil {
		t.Fatal(err)
	}
	if q, _ := svc.Store.Get(id); q.Stats.Connects != 1 {
		t.Fatalf("%+v", q.Stats)
	}
}

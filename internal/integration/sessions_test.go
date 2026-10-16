package integration

import (
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"strconv"
	"testing"

	"dbstudio/internal/api"
	"dbstudio/internal/apperr"
	"dbstudio/internal/connections"
	"dbstudio/internal/database"
	"dbstudio/internal/security"
	"dbstudio/internal/sshtunnel"
	"dbstudio/internal/sshtunnel/sshtest"
)

func bareApp(t *testing.T) *api.App {
	t.Helper()
	t.Setenv("DBSTUDIO_CONFIG_DIR", t.TempDir())
	app, err := api.New(slog.New(slog.NewTextHandler(io.Discard, nil)), security.NewMemory())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Shutdown(t.Context()) })
	return app
}

func codeOf(err error) string {
	if ae := appErr(err); ae != nil {
		return ae.Code
	}
	return ""
}

func TestSSHTunnelConnection(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			srv, err := sshtest.Start("tunel", "ssh-pw")
			if err != nil {
				t.Fatal(err)
			}
			defer srv.Close()
			app := bareApp(t)
			host, portS, _ := net.SplitHostPort(tg.addr)
			port, _ := strconv.Atoi(portS)
			pw := password()
			p := connections.Profile{Name: "SSH " + tg.name, Kind: tg.kind, Host: host, Port: port, User: "root", Database: "studio_it",
				TLS: database.TLSOptions{Mode: database.TLSPreferred}, Environment: connections.EnvLocal, KeepAliveSeconds: 5,
				SSH: connections.SSHOptions{Enabled: true, Host: "127.0.0.1", Port: srv.Port(), User: "tunel", Auth: sshtunnel.AuthPassword}}
			res, err := app.SaveConnection(connections.SaveInput{Profile: p, Password: &pw})
			if err != nil {
				t.Fatal(err)
			}
			id := res.Profile.ID

			// No SSH secret yet.
			if _, err := app.Connect(id, nil, false); codeOf(err) != apperr.CodeSSHSecretRequired {
				t.Fatalf("want ssh secret required, got %v", err)
			}
			if err := app.SetSSHSecret(id, "ssh-pw", false); err != nil {
				t.Fatal(err)
			}
			// Unknown host key: the error carries the fingerprint to confirm.
			_, err = app.Connect(id, nil, false)
			ae := appErr(err)
			if ae == nil || ae.Code != apperr.CodeSSHHostUnknown {
				t.Fatalf("want unknown host, got %v", err)
			}
			uh, ok := ae.Details.(*sshtunnel.UnknownHostError)
			if !ok || uh.Fingerprint != sshtunnel.Fingerprint(srv.HostKey) {
				t.Fatalf("details %#v", ae.Details)
			}
			if err := app.TrustSSHHost(uh.Host, uh.Fingerprint); err != nil {
				t.Fatal(err)
			}
			if _, err := app.Connect(id, nil, false); err != nil {
				t.Fatal(err)
			}
			if v := scalar(t, app, id, "SELECT 40 + 2"); v != "42" {
				t.Fatalf("query through tunnel: %s", v)
			}
			if srv.Forwards.Load() == 0 {
				t.Fatal("the connection did not go through the tunnel")
			}
			// The tunnel closes with the session; reconnecting works.
			if err := app.Disconnect(id); err != nil {
				t.Fatal(err)
			}
			if _, err := app.Connect(id, nil, false); err != nil {
				t.Fatal(err)
			}
			if v := scalar(t, app, id, "SELECT 1"); v != "1" {
				t.Fatal(v)
			}
			// Test connection of an unsaved profile with the secret typed.
			sec := "ssh-pw"
			p.ID = ""
			if _, err := app.TestConnection(connections.TestInput{Profile: p, Password: &pw, SSHSecret: &sec}); err != nil {
				t.Fatal(err)
			}
			bad := "errada"
			if _, err := app.TestConnection(connections.TestInput{Profile: p, Password: &pw, SSHSecret: &bad}); err == nil {
				t.Fatal("wrong ssh password accepted")
			}
		})
	}
}

// unixProxy exposes a TCP server on a Unix socket.
func unixProxy(t *testing.T, tcp string) string {
	path := filepath.Join(t.TempDir(), "mysqld.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				s, err := net.Dial("tcp", tcp)
				if err != nil {
					return
				}
				defer s.Close()
				done := make(chan struct{}, 2)
				go func() { _, _ = io.Copy(s, c); done <- struct{}{} }()
				go func() { _, _ = io.Copy(c, s); done <- struct{}{} }()
				<-done
			}()
		}
	}()
	return path
}

func TestUnixSocketAndInitScript(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			sock := unixProxy(t, tg.addr)
			app, id := newApp(t, tg, func(p *connections.Profile) {
				p.Host, p.Socket = "", sock
				p.TLS.Mode = database.TLSDisabled
				p.InitSQL = "SET @dbstudio_init = 42; SET SESSION sql_mode = CONCAT(@@sql_mode, ',ANSI_QUOTES')"
			})
			if v := scalar(t, app, id, "SELECT @dbstudio_init"); v != "42" {
				t.Fatalf("init script: %s", v)
			}
			// ANSI_QUOTES from the script: "a" is an identifier now.
			if resp, err := app.Execute(queryReq(id, `SELECT "a"`)); err != nil || resp.Results[0].Error == nil {
				t.Fatalf("sql_mode from the init script not applied: %v", err)
			}
			// Every pooled connection runs the script: a second tab gets its own connection.
			resp, err := app.Execute(queryReq(id, "SELECT @dbstudio_init"))
			if err != nil || resp.Results[0].Error != nil || cell(resp.Results[0].Rows, 0, 0) != "42" {
				t.Fatalf("%v %+v", err, resp)
			}
			// A failing script fails the connection with its statement in the message.
			app2, err := api.New(slog.New(slog.NewTextHandler(io.Discard, nil)), security.NewMemory())
			if err != nil {
				t.Fatal(err)
			}
			defer app2.Shutdown(t.Context())
			pw := password()
			res, err := app2.SaveConnection(connections.SaveInput{Profile: connections.Profile{Name: "init ruim", Kind: tg.kind, Socket: sock, User: "root",
				TLS: database.TLSOptions{Mode: database.TLSDisabled}, InitSQL: "SET @x = (SELECT nada FROM tabela_que_nao_existe)", Environment: connections.EnvLocal}, Password: &pw})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := app2.Connect(res.Profile.ID, nil, false); err == nil {
				t.Fatal("broken init script accepted")
			}
		})
	}
}

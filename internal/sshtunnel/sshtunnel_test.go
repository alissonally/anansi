package sshtunnel

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"dbstudio/internal/sshtunnel/sshtest"

	"golang.org/x/crypto/ssh"
)

func echoServer(t *testing.T) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
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
			go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
		}
	}()
	return ln.Addr().String()
}

func roundTrip(t *testing.T, tun *Tunnel, addr string) {
	t.Helper()
	c, err := tun.DialContext(context.Background(), "tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Write([]byte("olá túnel")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len("olá túnel"))
	if _, err := io.ReadFull(c, buf); err != nil || string(buf) != "olá túnel" {
		t.Fatalf("%q %v", buf, err)
	}
}

func TestTunnelHostKeysAndAuth(t *testing.T) {
	srv, err := sshtest.Start("deploy", "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	echo := echoServer(t)
	dir := t.TempDir()
	keys := &HostKeys{AppFile: filepath.Join(dir, "known_hosts"), pending: map[string]ssh.PublicKey{}}
	cfg := Config{Host: "127.0.0.1", Port: srv.Port(), User: "deploy", Auth: AuthPassword, Secret: "s3cret", Timeout: 5 * time.Second}
	ctx := context.Background()

	// Unknown host: refused with the fingerprint.
	_, err = Open(ctx, cfg, keys)
	var uh *UnknownHostError
	if !errors.As(err, &uh) || uh.Fingerprint != Fingerprint(srv.HostKey) || uh.Host != "[127.0.0.1]:"+strconv.Itoa(srv.Port()) {
		t.Fatalf("want unknown host, got %v", err)
	}
	if err := keys.Trust(uh.Host, "SHA256:outro"); err == nil {
		t.Fatal("trusted a fingerprint the server did not present")
	}
	if err := keys.Trust(uh.Host, uh.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if len(keys.Known()) != 1 {
		t.Fatalf("known: %v", keys.Known())
	}
	tun, err := Open(ctx, cfg, keys)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip(t, tun, echo)
	roundTrip(t, tun, echo)
	if srv.Forwards.Load() != 2 {
		t.Fatalf("forwards %d", srv.Forwards.Load())
	}
	_ = tun.Close()
	_ = tun.Close()

	// Wrong password / missing password.
	bad := cfg
	bad.Secret = "x"
	if _, err := Open(ctx, bad, keys); err == nil || !strings.Contains(err.Error(), "recusou") {
		t.Fatalf("wrong password: %v", err)
	}
	bad.Secret = ""
	if _, err := Open(ctx, bad, keys); !errors.Is(err, ErrNeedSecret) {
		t.Fatalf("missing password: %v", err)
	}

	// Changed host key: hard failure.
	srv2, err := sshtest.Start("deploy", "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	defer srv2.Close()
	line := strings.Fields(keys.Known()[0])[0]
	data, _ := os.ReadFile(keys.AppFile)
	other := strings.Replace(string(data), line, "[127.0.0.1]:"+strconv.Itoa(srv2.Port()), 1)
	_ = os.WriteFile(keys.AppFile, []byte(other), 0o600)
	cfg2 := cfg
	cfg2.Port = srv2.Port()
	if _, err := Open(ctx, cfg2, keys); err == nil || !strings.Contains(err.Error(), "MUDOU") {
		t.Fatalf("changed key: %v", err)
	}
}

func TestTunnelPrivateKey(t *testing.T) {
	srv, err := sshtest.Start("deploy", "")
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	sshPub, _ := ssh.NewPublicKey(pub)
	srv.AuthorizedKey = sshPub
	dir := t.TempDir()
	block, err := ssh.MarshalPrivateKeyWithPassphrase(priv, "", []byte("frase"))
	if err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	keys := &HostKeys{AppFile: filepath.Join(dir, "known_hosts"), pending: map[string]ssh.PublicKey{}}
	host := "[127.0.0.1]:" + strconv.Itoa(srv.Port())
	keys.pending[host] = srv.HostKey
	if err := keys.Trust(host, Fingerprint(srv.HostKey)); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Host: "127.0.0.1", Port: srv.Port(), User: "deploy", Auth: AuthKey, KeyFile: keyFile}
	if _, err := Open(context.Background(), cfg, keys); !errors.Is(err, ErrNeedSecret) {
		t.Fatalf("passphrase: %v", err)
	}
	cfg.Secret = "frase"
	tun, err := Open(context.Background(), cfg, keys)
	if err != nil {
		t.Fatal(err)
	}
	defer tun.Close()
	roundTrip(t, tun, echoServer(t))
}

func TestDeadlineConn(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	c := &deadlineConn{Conn: a}
	_ = c.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	start := time.Now()
	_, err := c.Read(make([]byte, 1))
	if !errors.Is(err, os.ErrDeadlineExceeded) || time.Since(start) > 2*time.Second {
		t.Fatalf("%v after %v", err, time.Since(start))
	}
	c2 := &deadlineConn{Conn: b}
	_ = c2.SetWriteDeadline(time.Now().Add(time.Hour))
	_ = c2.SetWriteDeadline(time.Time{})
	if c2.timers[1] != nil {
		t.Fatal("deadline not cleared")
	}
}

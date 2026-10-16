package mysql

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	gomysql "github.com/go-sql-driver/mysql"

	"dbstudio/internal/database"
)

func base() database.ConnectParams {
	return database.ConnectParams{Kind: database.MySQL, Host: "db.example.com", Port: 3306, User: "app", Password: "p@ss:w/rd?#", Database: "shop",
		TLS: database.TLSOptions{Mode: database.TLSPreferred}}
}

func TestBuildConfigNoManualDSN(t *testing.T) {
	d := New()
	p := base()
	p.Params = map[string]string{"charset": "utf8mb4", "time_zone": "'+00:00'"}
	cfg, err := d.buildConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Passwd != p.Password || cfg.Addr != "db.example.com:3306" || cfg.DBName != "shop" {
		t.Fatalf("config: %+v", cfg)
	}
	if cfg.MultiStatements || cfg.AllowAllFiles || cfg.AllowOldPasswords {
		t.Fatal("unsafe options enabled")
	}
	if cfg.Params["time_zone"] != "'+00:00'" {
		t.Fatal("session variable not passed")
	}
	if !cfg.AllowFallbackToPlaintext || cfg.TLS == nil {
		t.Fatal("preferred TLS should try TLS and allow fallback")
	}
	// Special characters in the password survive DSN formatting/parsing.
	parsed, err := gomysql.ParseDSN(cfg.FormatDSN())
	if err != nil || parsed.Passwd != p.Password {
		t.Fatalf("password round trip: %v %q", err, parsed.Passwd)
	}
}

func TestIPv6Host(t *testing.T) {
	p := base()
	p.Host = "::1"
	cfg, err := New().buildConfig(p)
	if err != nil || cfg.Addr != "[::1]:3306" {
		t.Fatalf("%v %q", err, cfg.Addr)
	}
}

func TestTLSModes(t *testing.T) {
	d := New()
	p := base()
	p.TLS.Mode = database.TLSDisabled
	cfg, _ := d.buildConfig(p)
	if cfg.TLS != nil || cfg.AllowFallbackToPlaintext {
		t.Fatal("disabled")
	}
	p.TLS.Mode = database.TLSRequired
	cfg, _ = d.buildConfig(p)
	if cfg.TLS == nil || cfg.AllowFallbackToPlaintext || !cfg.TLS.InsecureSkipVerify {
		t.Fatal("required must not fall back to plaintext")
	}
	p.TLS.Mode = database.TLSVerifyFull
	cfg, _ = d.buildConfig(p)
	if cfg.TLS == nil || cfg.TLS.InsecureSkipVerify || cfg.TLS.ServerName != "db.example.com" || cfg.AllowFallbackToPlaintext {
		t.Fatal("verify-full must verify the host name")
	}
	p.TLS = database.TLSOptions{Mode: database.TLSVerifyCA, CAFile: "/nonexistent/ca.pem"}
	if _, err := d.buildConfig(p); err == nil {
		t.Fatal("missing CA file accepted")
	}
}

func TestBlockedParams(t *testing.T) {
	for _, k := range []string{"allowAllFiles", "multiStatements", "tls", "allowOldPasswords", "password", "allowFallbackToPlaintext"} {
		p := base()
		p.Params = map[string]string{k: "true"}
		if err := New().Validate(p); err == nil {
			t.Errorf("%s accepted", k)
		}
	}
}

func TestDescribeError(t *testing.T) {
	d := New()
	syntax := &gomysql.MySQLError{Number: 1064, Message: "You have an error in your SQL syntax; check the manual that corresponds to your MySQL server version for the right syntax to use near 'FROM\nWHERE' at line 2"}
	de := d.DescribeError(fmt.Errorf("wrapped: %w", syntax))
	if de.Category != "syntax" || de.Line != 2 || de.Near != "FROM\nWHERE" || de.Code != 1064 {
		t.Fatalf("%+v", de)
	}
	auth := d.DescribeError(&gomysql.MySQLError{Number: 1045, Message: "Access denied for user 'x'@'y' (using password: YES)"})
	if auth.Category != "auth" {
		t.Fatalf("%+v", auth)
	}
	if d.DescribeError(context.DeadlineExceeded).Category != "timeout" || d.DescribeError(context.Canceled).Category != "cancelled" {
		t.Fatal("context errors")
	}
	if d.DescribeError(gomysql.ErrInvalidConn).Category != "network" {
		t.Fatal("invalid conn")
	}
	if !strings.Contains(d.DescribeError(errors.New("x509: certificate signed by unknown authority")).Category, "tls") {
		t.Fatal("tls")
	}
}

func TestQuoteIdent(t *testing.T) {
	if QuoteIdent("a`b") != "`a``b`" {
		t.Fatal(QuoteIdent("a`b"))
	}
}

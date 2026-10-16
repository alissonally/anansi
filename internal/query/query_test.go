package query

import (
	"errors"
	"strings"
	"testing"

	"anansi/internal/apperr"
	"anansi/internal/connections"
	"anansi/internal/sqllex"
)

func TestConvertValue(t *testing.T) {
	cases := []struct {
		in   any
		col  Column
		want any
	}{
		{nil, Column{}, nil},
		{int64(42), Column{}, int64(42)},
		{int64(1 << 60), Column{}, "1152921504606846976"},
		{uint64(1<<63 + 5), Column{}, "9223372036854775813"},
		{[]byte("NULL"), Column{}, "NULL"},
		{[]byte("12.3400"), Column{Numeric: true}, "12.3400"},
		{[]byte{0x00, 0xff}, Column{Binary: true}, "0x00FF"},
		{[]byte{0xff, 0xfe}, Column{}, "0xFFFE"}, // invalid UTF-8
		{float64(1.5), Column{}, 1.5},
	}
	for _, c := range cases {
		got, trunc := convertValue(c.in, c.col)
		if got != c.want || trunc {
			t.Errorf("convertValue(%#v) = %#v (%v), want %#v", c.in, got, trunc, c.want)
		}
	}
	big := strings.Repeat("é", MaxCellBytes)
	got, trunc := convertValue([]byte(big), Column{})
	if !trunc || len(got.(string)) > MaxCellBytes || !strings.HasSuffix(got.(string), "é") {
		t.Fatal("large cell not truncated on a rune boundary")
	}
}

func TestPlan(t *testing.T) {
	sql := "SELECT 1;\nSELECT 2;\nSELECT 3"
	st, err := Plan(ExecuteRequest{SQL: sql, CursorOffset: 12, Mode: "statement"})
	if err != nil || len(st) != 1 || st[0].Text != "SELECT 2" {
		t.Fatalf("%v %+v", err, st)
	}
	st, _ = Plan(ExecuteRequest{SQL: sql, CursorOffset: -1, Mode: "script"})
	if len(st) != 3 {
		t.Fatal("script")
	}
	if _, err := Plan(ExecuteRequest{SQL: "  -- nothing\n", Mode: "statement"}); err == nil {
		t.Fatal("empty sql accepted")
	}
	if _, err := Plan(ExecuteRequest{SQL: "SELECT 1", Mode: "rm -rf"}); err == nil {
		t.Fatal("invalid mode accepted")
	}
}

func code(err error) string {
	var ae *apperr.Error
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ""
}

func TestCheckPolicy(t *testing.T) {
	dev := connections.Profile{Name: "dev", Environment: connections.EnvDevelopment}
	prod := connections.Profile{Name: "prod", Environment: connections.EnvProduction}
	ro := connections.Profile{Name: "ro", Environment: connections.EnvDevelopment, ReadOnly: true}
	opts := Options{ConfirmDestructive: true}
	split := sqllex.Split
	if err := CheckPolicy(dev, split("UPDATE t SET a=1 WHERE id=1"), opts, false); err != nil {
		t.Fatal("safe update on dev should run without confirmation")
	}
	if code(CheckPolicy(dev, split("DELETE FROM t"), opts, false)) != apperr.CodeConfirmationRequired {
		t.Fatal("destructive statement needs confirmation")
	}
	if CheckPolicy(dev, split("DELETE FROM t"), Options{ConfirmDestructive: false}, false) != nil {
		t.Fatal("confirmation disabled in preferences")
	}
	if code(CheckPolicy(prod, split("UPDATE t SET a=1 WHERE id=1"), Options{}, false)) != apperr.CodeConfirmationRequired {
		t.Fatal("writes on production need confirmation even if destructive confirmation is off")
	}
	if CheckPolicy(prod, split("SELECT 1; SHOW TABLES; USE x"), opts, false) != nil {
		t.Fatal("reads on production need no confirmation")
	}
	if CheckPolicy(prod, split("DROP TABLE t"), opts, true) != nil {
		t.Fatal("confirmed")
	}
	if code(CheckPolicy(ro, split("INSERT INTO t VALUES (1)"), opts, true)) != apperr.CodeReadOnly {
		t.Fatal("read-only profile must block writes even when confirmed")
	}
	if code(CheckPolicy(ro, split("SET GLOBAL max_connections = 1"), opts, true)) != apperr.CodeReadOnly {
		t.Fatal("read-only profile must block SET GLOBAL")
	}
	if CheckPolicy(ro, split("SELECT 1; BEGIN; ROLLBACK"), opts, false) != nil {
		t.Fatal("reads allowed on read-only profile")
	}
	err := CheckPolicy(prod, split("DELETE FROM t"), opts, false)
	var ae *apperr.Error
	errors.As(err, &ae)
	d := ae.Details.(ConfirmationDetails)
	if !d.Production || len(d.Statements) != 1 || d.Statements[0].Reason == "" {
		t.Fatalf("details: %+v", d)
	}
}

func TestValidTabID(t *testing.T) {
	for id, ok := range map[string]bool{"tab-1234abcd": true, "short": false, "../../../../x": false, strings.Repeat("a", 65): false} {
		if ValidTabID(id) != ok {
			t.Errorf("%q", id)
		}
	}
}

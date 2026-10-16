package sqlfile

import (
	"context"
	"errors"
	"strings"
	"testing"

	"dbstudio/internal/sqllex"
)

type nopProgress struct{ warns int }

func (*nopProgress) Set(int64, int64) {}
func (*nopProgress) Message(string)   {}
func (n *nopProgress) Warn(string)    { n.warns++ }

func TestRunStreamsAcrossChunks(t *testing.T) {
	// A script larger than one chunk with a multi-byte character, a string
	// containing semicolons and a DELIMITER block crossing chunk boundaries.
	var b strings.Builder
	b.WriteString("\xef\xbb\xbfSET NAMES utf8mb4;\n")
	for b.Len() < chunkSize+100 {
		b.WriteString("INSERT INTO t VALUES ('ação; teste');\n")
	}
	b.WriteString("DELIMITER //\nCREATE PROCEDURE p() BEGIN SELECT 1; END//\nDELIMITER ;\nSELECT 'último'")
	script := b.String()
	want := len(sqllex.Split(strings.TrimPrefix(script, "\xef\xbb\xbf")))
	var got []string
	res, err := Run(context.Background(), strings.NewReader(script), int64(len(script)), func(_ context.Context, st sqllex.Statement) error {
		got = append(got, st.Text)
		return nil
	}, &nopProgress{}, true)
	if err != nil || int(res.Statements) != want || len(got) != want {
		t.Fatalf("%v: %d/%d statements", err, len(got), want)
	}
	if got[0] != "SET NAMES utf8mb4" || got[len(got)-2] != "CREATE PROCEDURE p() BEGIN SELECT 1; END" || got[len(got)-1] != "SELECT 'último'" {
		t.Fatalf("%q … %q", got[0], got[len(got)-2:])
	}
	for _, s := range got[1 : len(got)-2] {
		if s != "INSERT INTO t VALUES ('ação; teste')" {
			t.Fatalf("broken statement %q", s)
		}
	}
}

func TestRunErrors(t *testing.T) {
	boom := errors.New("boom")
	exec := func(_ context.Context, st sqllex.Statement) error {
		if strings.Contains(st.Text, "ruim") {
			return boom
		}
		return nil
	}
	p := &nopProgress{}
	res, err := Run(context.Background(), strings.NewReader("SELECT 1; SELECT ruim; SELECT 3;"), 0, exec, p, false)
	if err != nil || res.Statements != 3 || res.Errors != 1 || p.warns != 1 || !strings.Contains(res.FirstError, "instrução 2") {
		t.Fatalf("%v %+v", err, res)
	}
	res, err = Run(context.Background(), strings.NewReader("SELECT 1; SELECT ruim; SELECT 3;"), 0, exec, p, true)
	if err == nil || res.Statements != 2 {
		t.Fatalf("stop on error: %v %+v", err, res)
	}
}

package sqllex

import (
	"strings"
	"testing"
)

func TestTokenizeRoundTrip(t *testing.T) {
	inputs := []string{
		"SELECT * FROM `my``table` WHERE a = 'it''s' AND b = \"x\\\"y\" -- c\n# d\n/* e */",
		"select 1.5e10, 0x1F, .5, @a, @@session.sql_mode, ?;",
		"SELECT 'unterminated",
		"SELECT `open",
		"/* open comment",
		"SELECT ção, naïve FROM tabela",
	}
	for _, in := range inputs {
		var b strings.Builder
		for _, tok := range Tokenize(in) {
			b.WriteString(tok.Text)
		}
		if b.String() != in {
			t.Errorf("round trip mismatch: %q != %q", b.String(), in)
		}
	}
}

func TestTokenizeKinds(t *testing.T) {
	toks := Tokenize("SELECT `a``b`, 'x;y' FROM t -- ;\n")
	var kinds []Kind
	for _, tok := range toks {
		if tok.Significant() {
			kinds = append(kinds, tok.Kind)
		}
	}
	want := []Kind{Word, QuotedIdent, Punct, String, Word, Word}
	if len(kinds) != len(want) {
		t.Fatalf("got %v want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("token %d: got %v want %v", i, kinds[i], want[i])
		}
	}
	if got := toks[2].Ident(); got != "a`b" {
		t.Errorf("Ident() = %q", got)
	}
}

func TestOpenTokens(t *testing.T) {
	toks := Tokenize("SELECT 'abc")
	last := toks[len(toks)-1]
	if last.Kind != String || !last.Open {
		t.Fatalf("expected open string, got %+v", last)
	}
}

func TestSplit(t *testing.T) {
	src := "SELECT 1;\n  -- comment only\nSELECT 'a;b' ; ;\nINSERT INTO `x;y` VALUES (1)"
	st := Split(src)
	if len(st) != 3 {
		t.Fatalf("expected 3 statements, got %d: %+v", len(st), st)
	}
	if st[0].Text != "SELECT 1" || st[1].Text != "SELECT 'a;b'" || st[2].Text != "INSERT INTO `x;y` VALUES (1)" {
		t.Errorf("unexpected texts: %q %q %q", st[0].Text, st[1].Text, st[2].Text)
	}
	if st[1].Line != 3 || st[2].Line != 4 {
		t.Errorf("unexpected lines: %d %d", st[1].Line, st[2].Line)
	}
	if src[st[2].Start:st[2].End] != st[2].Text {
		t.Error("offsets do not match text")
	}
}

func TestSplitDelimiter(t *testing.T) {
	src := "DELIMITER $$\nCREATE PROCEDURE p() BEGIN SELECT 1; SELECT 2; END$$\nDELIMITER ;\nCALL p();"
	st := Split(src)
	if len(st) != 2 {
		t.Fatalf("expected 2 statements, got %d: %+v", len(st), st)
	}
	if !strings.HasPrefix(st[0].Text, "CREATE PROCEDURE") || !strings.HasSuffix(st[0].Text, "END") {
		t.Errorf("procedure body split incorrectly: %q", st[0].Text)
	}
	if st[1].Text != "CALL p()" {
		t.Errorf("got %q", st[1].Text)
	}
}

func TestSplitDelimiterWordAsColumn(t *testing.T) {
	st := Split("SELECT\ndelimiter FROM t; SELECT 2")
	if len(st) != 2 || !strings.Contains(st[0].Text, "delimiter FROM t") {
		t.Fatalf("column named delimiter misparsed: %+v", st)
	}
}

func TestStatementAt(t *testing.T) {
	src := "SELECT 1;\nSELECT 2;\n\nSELECT 3"
	st := Split(src)
	cases := map[int]string{0: "SELECT 1", 12: "SELECT 2", len(src): "SELECT 3", 20: "SELECT 2"}
	for pos, want := range cases {
		got, ok := StatementAt(st, pos)
		if !ok || got.Text != want {
			t.Errorf("pos %d: got %q want %q", pos, got.Text, want)
		}
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		sql         string
		kind        Category
		destructive bool
	}{
		{"SELECT * FROM t", CatRead, false},
		{"  (SELECT 1) UNION (SELECT 2)", CatRead, false},
		{"WITH x AS (SELECT 1) SELECT * FROM x", CatRead, false},
		{"WITH x AS (SELECT 1) DELETE FROM t WHERE id IN (SELECT * FROM x)", CatWrite, false},
		{"show tables", CatRead, false},
		{"EXPLAIN SELECT 1", CatRead, false},
		{"INSERT INTO t VALUES (1)", CatWrite, false},
		{"UPDATE t SET a = 1 WHERE id = 2", CatWrite, false},
		{"UPDATE t SET a = (SELECT 1 FROM x WHERE y)", CatWrite, true},
		{"DELETE FROM t", CatWrite, true},
		{"DELETE FROM t WHERE id = 1", CatWrite, false},
		{"DROP TABLE t", CatDDL, true},
		{"TRUNCATE t", CatDDL, true},
		{"ALTER TABLE t ADD COLUMN c INT", CatDDL, false},
		{"ALTER TABLE t DROP COLUMN c", CatDDL, true},
		{"CREATE TABLE t (id INT)", CatDDL, false},
		{"BEGIN", CatTransaction, false},
		{"USE db", CatSession, false},
		{"SET @a = 1", CatSession, false},
		{"SELECT * INTO OUTFILE '/tmp/x' FROM t", CatWrite, false},
		{"-- comment\nSELECT 1", CatRead, false},
	}
	for _, c := range cases {
		k, d, _ := Classify(c.sql)
		if k != c.kind || d != c.destructive {
			t.Errorf("%q: got (%s,%v) want (%s,%v)", c.sql, k, d, c.kind, c.destructive)
		}
	}
}

func TestTransactionEffect(t *testing.T) {
	cases := map[string]int{
		"BEGIN": 1, "START TRANSACTION READ ONLY": 1, "COMMIT": -1, "ROLLBACK": -1,
		"ROLLBACK TO SAVEPOINT a": 0, "ROLLBACK WORK TO a": 0, "SELECT 1": 0, "CREATE TABLE x (a int)": -1,
	}
	for sql, want := range cases {
		if got := TransactionEffect(sql); got != want {
			t.Errorf("%q: got %d want %d", sql, got, want)
		}
	}
}

func TestSplitPartialMatchesSplit(t *testing.T) {
	script := "SET NAMES utf8mb4;\nINSERT INTO t VALUES ('a;b', \"c;d\", `e;f`); -- c;omentário\n" +
		"/* bloco; */ SELECT 1;\nDELIMITER $$\nCREATE PROCEDURE p() BEGIN SELECT 1; SELECT 2; END$$\nDELIMITER ;\n" +
		"INSERT INTO t VALUES ('linha\nquebrada; ainda string');\nSELECT 'fim'"
	want := Split(script)
	for _, size := range []int{1, 2, 3, 5, 7, 13, 64, 1000} {
		var got []Statement
		buf := ""
		delim := ";"
		for i := 0; i < len(script); i += size {
			end := i + size
			if end > len(script) {
				end = len(script)
			}
			buf += script[i:end]
			stmts, rest, d := SplitPartial(buf, delim)
			got = append(got, stmts...)
			buf, delim = buf[rest:], d
		}
		got = append(got, Split(map[bool]string{true: "DELIMITER " + delim + "\n", false: ""}[delim != ";"]+buf)...)
		if len(got) != len(want) {
			t.Fatalf("chunk %d: %d statements, want %d: %+v", size, len(got), len(want), got)
		}
		for i := range want {
			if got[i].Text != want[i].Text {
				t.Fatalf("chunk %d stmt %d: %q want %q", size, i, got[i].Text, want[i].Text)
			}
		}
	}
}

func TestConditionalComments(t *testing.T) {
	stmts := Split("/*!40101 SET NAMES utf8mb4 */;\n/* comentário */ SELECT 1;\n/*!50000 DROP TABLE t */;")
	if len(stmts) != 3 || stmts[0].Text != "/*!40101 SET NAMES utf8mb4 */" {
		t.Fatalf("%+v", stmts)
	}
	if stmts[0].Kind != CatSession || stmts[2].Kind != CatDDL || !stmts[2].Destructive {
		t.Fatalf("kinds %v %v %v", stmts[0].Kind, stmts[2].Kind, stmts[2].Destructive)
	}
	if ConditionalBody("/*M!100400 SET x = 1 */") != "SET x = 1" {
		t.Fatal(ConditionalBody("/*M!100400 SET x = 1 */"))
	}
}

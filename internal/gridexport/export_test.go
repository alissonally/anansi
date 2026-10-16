package gridexport

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

var cols = []Column{{Name: "id", Numeric: true, Key: true, AutoIncrement: true}, {Name: "nome"}, {Name: "foto", Binary: true}}
var rows = [][]any{{float64(1), `Ana "x"`, nil}, {"9007199254740993", "a|b\nc'd", "0xCAFE"}}

func run(t *testing.T, o Options) string {
	t.Helper()
	var buf bytes.Buffer
	w, err := New(&buf, o)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Begin(cols); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if err := w.Row(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.End(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestDelimited(t *testing.T) {
	got := run(t, Options{Format: FormatCSV, Header: true})
	want := "\"id\";\"nome\";\"foto\"\n\"1\";\"Ana \"\"x\"\"\";\\N\n\"9007199254740993\";\"a|b\nc'd\";\"0xCAFE\"\n"
	if got != want {
		t.Fatalf("csv:\n%q\n%q", got, want)
	}
	got = run(t, Options{Format: FormatTSV, NullText: "NULL", RemoveLineBreaks: true})
	if got != "1\tAna \"x\"\tNULL\n9007199254740993\ta|b c'd\t0xCAFE\n" {
		t.Fatalf("tsv %q", got)
	}
	got = run(t, Options{Format: FormatExcelCSV, Header: true})
	if !strings.HasPrefix(got, "\ufeff\"id\";") || !strings.Contains(got, "\r\n") || strings.Contains(got, `\N`) {
		t.Fatalf("excel %q", got)
	}
}

func TestJSON(t *testing.T) {
	got := run(t, Options{Format: FormatJSON, Table: "db.t", Query: "SELECT 1"})
	var doc struct {
		Table string           `json:"table"`
		Query string           `json:"query"`
		Rows  []map[string]any `json:"rows"`
	}
	dec := json.NewDecoder(strings.NewReader(got))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("%v\n%s", err, got)
	}
	if doc.Table != "db.t" || len(doc.Rows) != 2 || doc.Rows[1]["id"].(json.Number).String() != "9007199254740993" || doc.Rows[0]["foto"] != nil {
		t.Fatalf("%+v", doc)
	}
	lines := strings.Split(strings.TrimSpace(run(t, Options{Format: FormatJSONLines})), "\n")
	if len(lines) != 2 || !json.Valid([]byte(lines[1])) {
		t.Fatal(lines)
	}
}

func TestMarkupFormats(t *testing.T) {
	if got := run(t, Options{Format: FormatMarkdown}); !strings.Contains(got, "| ---: | --- | --- |") || !strings.Contains(got, `a\|b<br>c'd`) || !strings.Contains(got, "| NULL |") {
		t.Fatal(got)
	}
	if got := run(t, Options{Format: FormatHTML, Table: "t"}); !strings.Contains(got, "<td>Ana &#34;x&#34;</td>") || !strings.Contains(got, `<td class="null">NULL</td>`) || !strings.Contains(got, "a|b<br>c&#39;d") {
		t.Fatal(got)
	}
	if got := run(t, Options{Format: FormatXML}); !strings.Contains(got, `<field name="nome">Ana &quot;x&quot;</field>`) || !strings.Contains(got, `xsi:nil="true"`) {
		t.Fatal(got)
	}
	if got := run(t, Options{Format: FormatLaTeX}); !strings.Contains(got, `\begin{tabular}{|r|l|l|}`) {
		t.Fatal(got)
	}
	if got := run(t, Options{Format: FormatPHP, Table: "minha-tabela"}); !strings.Contains(got, "$minha_tabela = array(") || !strings.Contains(got, `'nome' => 'a|b`+"\n"+`c\'d'`) {
		t.Fatal(got)
	}
	if got := run(t, Options{Format: FormatJira}); !strings.HasPrefix(got, "|| id || nome || foto ||") {
		t.Fatal(got)
	}
}

func TestSQLFormats(t *testing.T) {
	got := run(t, Options{Format: FormatSQLInsert, Table: "loja.t"})
	if !strings.Contains(got, "INSERT INTO `loja`.`t` (`id`, `nome`, `foto`) VALUES (1, 'Ana \"x\"', NULL);") ||
		!strings.Contains(got, `(9007199254740993, 'a|b\nc\'d', _binary 0xCAFE);`) {
		t.Fatal(got)
	}
	got = run(t, Options{Format: FormatSQLReplace, Table: "t", SkipAutoIncrement: true, RowsPerInsert: 100})
	if strings.Count(got, "REPLACE INTO `t` (`nome`, `foto`) VALUES") != 1 || !strings.Contains(got, "),\n\t(") {
		t.Fatal(got)
	}
	got = run(t, Options{Format: FormatSQLDelIns, Table: "t"})
	if !strings.Contains(got, "DELETE FROM `t` WHERE `id`=1;\nINSERT INTO") {
		t.Fatal(got)
	}
	got = run(t, Options{Format: FormatSQLUpdate, Table: "t"})
	if !strings.Contains(got, "UPDATE `t` SET `nome`='Ana \"x\"', `foto`=NULL WHERE `id`=1;") {
		t.Fatal(got)
	}
	if QuoteString("a\x00\\'\x1a") != `'a\0\\\'\Z'` {
		t.Fatal(QuoteString("a\x00\\'\x1a"))
	}
}

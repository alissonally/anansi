package csvimport

import (
	"strings"
	"testing"
)

func vals(rec []*string) []string {
	out := make([]string, len(rec))
	for i, v := range rec {
		if v == nil {
			out[i] = "<NULL>"
		} else {
			out[i] = *v
		}
	}
	return out
}

func readAllT(t *testing.T, text string, f Format) [][]string {
	t.Helper()
	r, err := NewReader(strings.NewReader(text), f)
	if err != nil {
		t.Fatal(err)
	}
	var out [][]string
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		out = append(out, vals(rec))
	}
	return out
}

func TestReaderRFC4180AndMySQLStyle(t *testing.T) {
	got := readAllT(t, "\ufeffid;nome;obs\r\n1;\"Ana \"\"A\"\"\";\"linha\nnova\"\r\n2;;\\N\n3;\"\";x", Format{Separator: ";", Encloser: `"`, SkipLines: 1})
	want := [][]string{{"1", `Ana "A"`, "linha\nnova"}, {"2", "", "<NULL>"}, {"3", "", "x"}}
	if len(got) != 3 || strings.Join(got[0], "|") != strings.Join(want[0], "|") || strings.Join(got[1], "|") != strings.Join(want[1], "|") || strings.Join(got[2], "|") != strings.Join(want[2], "|") {
		t.Fatalf("%q", got)
	}
	got = readAllT(t, "1,\"a\\\"b\\nc\",\\N\n", Format{Separator: ",", Encloser: `"`, Escape: `\`})
	if strings.Join(got[0], "|") != "1|a\"b\nc|<NULL>" {
		t.Fatalf("%q", got)
	}
	got = readAllT(t, "a\t\tb\n", Format{Separator: `\t`, EmptyNull: true})
	if strings.Join(got[0], "|") != "a|<NULL>|b" {
		t.Fatalf("%q", got)
	}
	got = readAllT(t, "nome;cidade\nJosé;S\xe3o Paulo\n", Format{Separator: ";", Encoding: "latin1", SkipLines: 1})
	if got[0][1] != "São Paulo" {
		t.Fatalf("latin1 %q", got)
	}
	if _, err := (&Reader{}).nullMarkerOK(); err != nil {
		t.Fatal(err)
	}
}

func (r *Reader) nullMarkerOK() (string, error) { return r.nullMarker(), nil }

func TestDetect(t *testing.T) {
	d := Detect([]byte("Código;Nome do cliente;Valor;Data;Quando\n1;Ana;10,50;2024-01-02;2024-01-02 10:00:00\n200;Bia;;2024-02-03;2024-02-03 11:00\n"), 100)
	if d.Format.Separator != ";" || d.Format.SkipLines != 1 || strings.Join(d.Header, ",") != "Código,Nome_do_cliente,Valor,Data,Quando" {
		t.Fatalf("%+v", d)
	}
	types := []string{}
	for _, c := range d.Columns {
		types = append(types, c.Type+c.Length)
	}
	if strings.Join(types, ",") != "SMALLINT,VARCHAR10,DECIMAL4,2,DATE,DATETIME" || !d.Columns[2].Nullable || !d.Columns[0].Unsigned {
		t.Fatalf("%v %+v", types, d.Columns)
	}
	d = Detect([]byte("1\t2\n3\t4\n"), 10)
	if d.Format.Separator != "\t" || d.Format.SkipLines != 0 || d.Header[0] != "coluna1" {
		t.Fatalf("%+v", d)
	}
	if NormalizeNumber("1.234,56", ".", ",") != "1234.56" {
		t.Fatal("number")
	}
}

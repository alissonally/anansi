package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"CREATE USER 'bob'@'%' IDENTIFIED BY 'hunter2'": "CREATE USER '***'@'%' IDENTIFIED BY '***'",
		"ALTER USER bob IDENTIFIED BY \"pw\"":           "ALTER USER bob IDENTIFIED BY '***'",
		"SET PASSWORD FOR bob = 'x'":                    "SET PASSWORD FOR bob = '***'",
		"SELECT * FROM t WHERE name = 'ok'":             "SELECT * FROM t WHERE name = 'ok'",
		"SELECT AES_DECRYPT(col, 'key') FROM secrets":   "SELECT AES_DECRYPT(col, '***') FROM secrets",
	}
	for in, want := range cases {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStorePersistLimitAndClear(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, 3)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		s.Add(Entry{ConnectionID: map[bool]string{true: "a", false: "b"}[i%2 == 0], SQL: "SELECT " + string(rune('0'+i)), ExecutedAt: time.Now()})
	}
	s.Add(Entry{ConnectionID: "a", SQL: "CREATE USER x IDENTIFIED BY 'secret-pw'"})
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "history.json"))
	if strings.Contains(string(b), "secret-pw") {
		t.Fatal("secret written to history")
	}
	s2, _ := Open(dir, 3)
	all := s2.List(Query{})
	if len(all) != 3 || !strings.Contains(all[0].SQL, "CREATE USER") {
		t.Fatalf("reloaded: %+v", all)
	}
	if got := s2.List(Query{ConnectionID: "b"}); len(got) != 1 {
		t.Fatalf("filter: %+v", got)
	}
	if err := s2.Clear("a"); err != nil {
		t.Fatal(err)
	}
	if got := s2.List(Query{}); len(got) != 1 || got[0].ConnectionID != "b" {
		t.Fatalf("after clear: %+v", got)
	}
}

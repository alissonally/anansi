package sqllex

import "testing"

func TestSingleTableSource(t *testing.T) {
	ok := []struct {
		sql, db, table, alias string
		items                 []SelectItem
	}{
		{"SELECT * FROM t", "", "t", "", []SelectItem{{Star: true}}},
		{"select id, `nome` as nome, c.x from loja.clientes c where id in (select 1) order by 2 limit 5", "loja", "clientes", "c",
			[]SelectItem{{Column: "id"}, {Column: "nome"}, {Qualifier: "c", Column: "x"}}},
		{"SELECT t.*, COUNT(1) OVER () AS n, a + 1, a AS b FROM `db`.`t` AS t", "db", "t", "t",
			[]SelectItem{{Star: true, Qualifier: "t"}, {}, {}, {}}},
	}
	for _, c := range ok {
		src, good := SingleTableSource(c.sql)
		if !good || src.Database != c.db || src.Table != c.table || src.Alias != c.alias || len(src.Items) != len(c.items) {
			t.Fatalf("%q → %+v %v", c.sql, src, good)
		}
		for i := range c.items {
			if src.Items[i] != c.items[i] {
				t.Fatalf("%q item %d: %+v want %+v", c.sql, i, src.Items[i], c.items[i])
			}
		}
	}
	for _, bad := range []string{
		"SELECT * FROM a JOIN b ON a.id = b.id", "SELECT * FROM a, b", "SELECT DISTINCT x FROM t", "SELECT x FROM t GROUP BY x",
		"SELECT * FROM (SELECT 1) d", "SELECT 1", "SELECT * FROM t UNION SELECT * FROM u", "UPDATE t SET a = 1", "SELECT * FROM t FOR UPDATE",
	} {
		if _, good := SingleTableSource(bad); good {
			t.Fatalf("accepted %q", bad)
		}
	}
}

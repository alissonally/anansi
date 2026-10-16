package mysql

import (
	"strings"
	"testing"

	"dbstudio/internal/database"
)

func TestSplitColumnTypeAndEnums(t *testing.T) {
	typ, args, u, z := splitColumnType("int(10) unsigned zerofill")
	if typ != "INT" || args != "10" || !u || !z {
		t.Fatal(typ, args, u, z)
	}
	typ, args, _, _ = splitColumnType("enum('a)','b''c')")
	if typ != "ENUM" || args != "'a)','b''c'" {
		t.Fatal(typ, args)
	}
	vals, err := parseEnumArgs(`'a', 'b''c','d\'e'`)
	if err != nil || strings.Join(vals, "|") != "a|b'c|d'e" {
		t.Fatal(vals, err)
	}
	if _, err := parseEnumArgs("'aberta"); err == nil {
		t.Fatal("unterminated enum accepted")
	}
}

func TestMariaDefault(t *testing.T) {
	for in, want := range map[string][2]string{
		"NULL": {"null", ""}, "'it''s'": {"value", "it's"}, "12.5": {"value", "12.5"}, "current_timestamp()": {"expression", "current_timestamp()"},
	} {
		k, v := mariaDefault(in)
		if k != want[0] || v != want[1] {
			t.Fatalf("%s → %s %s", in, k, v)
		}
	}
}

func TestColumnDefSQL(t *testing.T) {
	s, err := ColumnDefSQL(database.ColumnDef{Name: "c", Type: "varchar", Length: "20", Collation: "utf8mb4_bin", DefaultKind: "value", Default: "x'y", Comment: "c"}, false)
	if err != nil || s != "`c` VARCHAR(20) COLLATE utf8mb4_bin NOT NULL DEFAULT 'x''y' COMMENT 'c'" {
		t.Fatal(s, err)
	}
	s, _ = ColumnDefSQL(database.ColumnDef{Name: "u", Type: "BINARY", Length: "16", DefaultKind: "expression", Default: "UUID_TO_BIN(UUID())"}, false)
	if s != "`u` BINARY(16) NOT NULL DEFAULT (UUID_TO_BIN(UUID()))" {
		t.Fatal(s)
	}
	s, _ = ColumnDefSQL(database.ColumnDef{Name: "t", Type: "TIMESTAMP", DefaultKind: "expression", Default: "CURRENT_TIMESTAMP", OnUpdate: "CURRENT_TIMESTAMP"}, false)
	if s != "`t` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP" {
		t.Fatal(s)
	}
	s, _ = ColumnDefSQL(database.ColumnDef{Name: "g", Type: "INT", Generated: "a + 1", GeneratedKind: "STORED", Nullable: true}, true)
	if s != "`g` INT AS (a + 1) PERSISTENT" {
		t.Fatal(s)
	}
	for _, bad := range []database.ColumnDef{
		{Name: "x", Type: "VARCHAR"},
		{Name: "x", Type: "TEXT", Unsigned: true},
		{Name: "x", Type: "INT", DefaultKind: "expression", Default: "1); DROP TABLE t; --"},
		{Name: "x", Type: "INT", Generated: "(a"},
		{Name: "x", Type: "ENUM", Length: "'a"},
		{Name: "x", Type: "NOPE"},
		{Name: "x", Type: "VARCHAR", Length: "10) NOT NULL, y INT, z INT(1"},
	} {
		if _, err := ColumnDefSQL(bad, false); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
}

func TestAlterTableMovesOnlyWhatChanged(t *testing.T) {
	d := New()
	col := func(n string) database.ColumnDef { return database.ColumnDef{OrigName: n, Name: n, Type: "INT"} }
	cur := database.TableDef{Database: "db", Name: "t", Columns: []database.ColumnDef{col("a"), col("b"), col("c")}}
	same, err := d.AlterTableSQL(cur, cur)
	if err != nil || len(same) != 0 {
		t.Fatal(same, err)
	}
	def := cur
	def.Columns = []database.ColumnDef{col("c"), col("a"), {Name: "n", Type: "INT"}, col("b")}
	stmts, err := d.AlterTableSQL(cur, def)
	if err != nil || len(stmts) != 1 {
		t.Fatal(stmts, err)
	}
	s := stmts[0]
	if !strings.Contains(s, "CHANGE COLUMN `c` `c` INT NOT NULL FIRST") || !strings.Contains(s, "CHANGE COLUMN `a` `a` INT NOT NULL AFTER `c`") ||
		!strings.Contains(s, "ADD COLUMN `n` INT NOT NULL AFTER `a`") || strings.Contains(s, "CHANGE COLUMN `b`") {
		t.Fatal(s)
	}
	// A column that no longer exists on the server is reported.
	def.Columns = append(def.Columns, database.ColumnDef{OrigName: "sumiu", Name: "sumiu", Type: "INT"})
	if _, err := d.AlterTableSQL(cur, def); err == nil {
		t.Fatal("stale column accepted")
	}
}

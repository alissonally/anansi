package mysql

import (
	"testing"

	"anansi/internal/database"
)

func TestObjectSQL(t *testing.T) {
	d := New()
	ok := func(got string, err error, want string) {
		t.Helper()
		if err != nil || got != want {
			t.Fatalf("got %q (%v), want %q", got, err, want)
		}
	}
	bad := func(_ string, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("expected error")
		}
	}
	s, err := d.DropSQL(database.ObjTable, "loja", "pe`didos")
	ok(s, err, "DROP TABLE `loja`.`pe``didos`")
	s, err = d.DropSQL(database.ObjDatabase, "", "loja")
	ok(s, err, "DROP DATABASE `loja`")
	s, err = d.DropSQL(database.ObjProcedure, "loja", "p")
	ok(s, err, "DROP PROCEDURE `loja`.`p`")
	bad(d.DropSQL("index", "loja", "x"))
	bad(d.DropSQL(database.ObjTable, "", "x"))
	s, err = d.TruncateSQL("loja", "t")
	ok(s, err, "TRUNCATE TABLE `loja`.`t`")
	s, err = d.RenameSQL(database.ObjTable, "a", "t", "b", "u")
	ok(s, err, "RENAME TABLE `a`.`t` TO `b`.`u`")
	s, err = d.RenameSQL(database.ObjEvent, "a", "e", "", "f")
	ok(s, err, "ALTER EVENT `a`.`e` RENAME TO `a`.`f`")
	bad(d.RenameSQL(database.ObjView, "a", "v", "b", "v"))
	bad(d.RenameSQL(database.ObjTrigger, "a", "x", "", "y"))
	s, err = d.SetVariableSQL(true, "max_connections", " 200 ", "auto")
	ok(s, err, "SET GLOBAL max_connections = 200")
	s, err = d.SetVariableSQL(false, "sql_mode", "STRICT_ALL_TABLES", "auto")
	ok(s, err, "SET SESSION sql_mode = 'STRICT_ALL_TABLES'")
	s, err = d.SetVariableSQL(true, "general_log", "on", "auto")
	ok(s, err, "SET GLOBAL general_log = ON")
	s, err = d.SetVariableSQL(true, "x", "", "default")
	ok(s, err, "SET GLOBAL x = DEFAULT")
	bad(d.SetVariableSQL(true, "x; DROP", "1", "auto"))
	bad(d.SetVariableSQL(true, "x", "1; DROP TABLE t", "raw"))
	s, err = d.KillSQL(42, true)
	ok(s, err, "KILL QUERY 42")
	bad(d.KillSQL(0, false))
	s, err = d.AlterDatabaseSQL("loja", "utf8mb4", "utf8mb4_bin")
	ok(s, err, "ALTER DATABASE `loja` CHARACTER SET utf8mb4 COLLATE utf8mb4_bin")
	bad(d.AlterDatabaseSQL("loja", "utf8mb4; DROP", ""))
}

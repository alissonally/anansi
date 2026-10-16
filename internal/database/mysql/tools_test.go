package mysql

import (
	"strings"
	"testing"

	"anansi/internal/database"
)

func TestMaintenanceSQL(t *testing.T) {
	d := New()
	s, err := d.MaintenanceSQL("check", []string{"quick", "QUICK", "for upgrade"}, "db", "t`x")
	if err != nil || s != "CHECK TABLE `db`.`t``x` QUICK FOR UPGRADE" {
		t.Fatalf("%q %v", s, err)
	}
	for _, bad := range [][]string{{"CHECKSUM", "QUICK", "EXTENDED"}, {"ANALYZE", "QUICK"}, {"DROP"}, {"REPAIR", "QUICK; DROP"}} {
		if _, err := d.MaintenanceSQL(bad[0], bad[1:], "db", "t"); err == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
}

func TestSearchCondition(t *testing.T) {
	d := New()
	cols := []database.ColumnInfo{{Name: "nome", DataType: "varchar"}, {Name: "n", DataType: "int"}, {Name: "g", DataType: "geometry"}, {Name: "b", DataType: "blob"}}
	cond, args, err := d.SearchCondition(cols, SearchSpec{Text: "50%_!"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(cond, "`g`") || strings.Contains(cond, "`n`") {
		t.Fatalf("spatial or non-numeric text on numeric column: %s", cond)
	}
	if args[0] != "%50!%!_!!%" || !strings.Contains(cond, "ESCAPE '!'") {
		t.Fatalf("%s %v", cond, args)
	}
	cond, args, _ = d.SearchCondition(cols, SearchSpec{Text: "42", Types: "numeric"}, false)
	if cond != "`n` = ?" || args[0] != "42" {
		t.Fatalf("%s %v", cond, args)
	}
	cond, _, _ = d.SearchCondition(cols[:1], SearchSpec{Text: "a.c", Match: "regex", CaseSensitive: true}, false)
	if !strings.Contains(cond, "REGEXP_LIKE(") || !strings.Contains(cond, "'c')") {
		t.Fatalf("mysql regex %s", cond)
	}
	_, args, _ = d.SearchCondition(cols[:1], SearchSpec{Text: "a.c", Match: "regex"}, true)
	if args[0] != "(?i)a.c" {
		t.Fatalf("mariadb regex %v", args)
	}
	if _, _, err := d.SearchCondition(cols, SearchSpec{Text: "(", Match: "regex"}, false); err == nil {
		t.Fatal("invalid regex accepted")
	}
}

func TestBulkAndFlushSQL(t *testing.T) {
	d := New()
	s, err := d.BulkSQL("a", "t", BulkSpec{Engine: "InnoDB", ConvertCharset: "utf8mb4", ConvertCollation: "utf8mb4_bin", ResetAutoInc: true, MoveTo: "b"})
	if err != nil || len(s) != 2 || s[0] != "ALTER TABLE `a`.`t` ENGINE=InnoDB, CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_bin, AUTO_INCREMENT=1" || s[1] != "RENAME TABLE `a`.`t` TO `b`.`t`" {
		t.Fatalf("%v %v", s, err)
	}
	if _, err := d.BulkSQL("a", "t", BulkSpec{Engine: "InnoDB; DROP"}); err == nil {
		t.Fatal("bad engine accepted")
	}
	if _, err := d.BulkSQL("a", "t", BulkSpec{}); err == nil {
		t.Fatal("empty bulk accepted")
	}
	if s, err := d.FlushSQL("privileges"); err != nil || s != "FLUSH PRIVILEGES" {
		t.Fatal(s, err)
	}
	if _, err := d.FlushSQL("TABLES WITH READ LOCK"); err == nil {
		t.Fatal("flush with lock accepted")
	}
}

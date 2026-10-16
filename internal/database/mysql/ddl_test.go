package mysql

import (
	"strings"
	"testing"

	"dbstudio/internal/database"
)

func TestCreateDatabaseSQL(t *testing.T) {
	d := New()
	got, err := d.CreateDatabaseSQL(database.CreateDatabaseSpec{Name: "loja`x", Charset: "utf8mb4", Collation: "utf8mb4_unicode_ci", IfNotExists: true})
	if err != nil {
		t.Fatal(err)
	}
	if got != "CREATE DATABASE IF NOT EXISTS `loja``x` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci" {
		t.Fatal(got)
	}
	for _, bad := range []database.CreateDatabaseSpec{
		{Name: ""}, {Name: "a/b"}, {Name: "a."}, {Name: "x "}, {Name: strings.Repeat("a", 65)},
		{Name: "ok", Charset: "utf8; DROP"}, {Name: "ok", Collation: "x y"},
	} {
		if _, err := d.CreateDatabaseSQL(bad); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

func TestCreateTableSQL(t *testing.T) {
	spec := database.CreateTableSpec{
		Database: "shop", Name: "orders", Engine: "InnoDB", Charset: "utf8mb4", Collation: "utf8mb4_0900_ai_ci", Comment: "pedidos d'ontem",
		Columns: []database.ColumnSpec{
			{Name: "id", Type: "bigint", Unsigned: true, PrimaryKey: true, AutoIncrement: true},
			{Name: "status", Type: "ENUM", Length: "novo, pago,can'celado", DefaultKind: "value", Default: "novo"},
			{Name: "total", Type: "DECIMAL", Length: "10, 2", DefaultKind: "value", Default: "0.00"},
			{Name: "email", Type: "VARCHAR", Length: "190", Nullable: true, Unique: true, DefaultKind: "null"},
			{Name: "created_at", Type: "DATETIME", Length: "3", DefaultKind: "current_timestamp", OnUpdateCurrentTimestamp: true},
			{Name: "data", Type: "JSON", Nullable: true, DefaultKind: "expression", Default: "JSON_OBJECT()", Comment: "extra"},
		},
		Indexes: []database.IndexSpec{{Columns: []string{"status", "created_at"}}, {Name: "uq_tot", Columns: []string{"total"}, Unique: true}},
	}
	got, err := New().CreateTableSQL(spec)
	if err != nil {
		t.Fatal(err)
	}
	want := "CREATE TABLE `shop`.`orders` (\n" +
		"  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,\n" +
		"  `status` ENUM('novo', 'pago', 'can''celado') NOT NULL DEFAULT 'novo',\n" +
		"  `total` DECIMAL(10,2) NOT NULL DEFAULT 0.00,\n" +
		"  `email` VARCHAR(190) NULL DEFAULT NULL UNIQUE,\n" +
		"  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),\n" +
		"  `data` JSON NULL DEFAULT (JSON_OBJECT()) COMMENT 'extra',\n" +
		"  PRIMARY KEY (`id`),\n" +
		"  KEY `idx_status_created_at` (`status`, `created_at`),\n" +
		"  UNIQUE KEY `uq_tot` (`total`)\n" +
		") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='pedidos d''ontem'"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestCreateTableCompositeKeyAndQuoting(t *testing.T) {
	got, err := New().CreateTableSQL(database.CreateTableSpec{Database: "d", Name: "order items", Columns: []database.ColumnSpec{
		{Name: "order", Type: "INT", PrimaryKey: true, Nullable: true},
		{Name: "line`no", Type: "INT", PrimaryKey: true},
		{Name: "note", Type: "VARCHAR", Length: "20", DefaultKind: "value", Default: `a\b'c`},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, frag := range []string{"`d`.`order items`", "`order` INT NOT NULL", "PRIMARY KEY (`order`, `line``no`)", `DEFAULT 'a\\b''c'`} {
		if !strings.Contains(got, frag) {
			t.Errorf("missing %q in:\n%s", frag, got)
		}
	}
}

func TestCreateTableValidation(t *testing.T) {
	col := func(c database.ColumnSpec) database.CreateTableSpec {
		return database.CreateTableSpec{Database: "d", Name: "t", Columns: []database.ColumnSpec{c}}
	}
	bad := map[string]database.CreateTableSpec{
		"no columns":          {Database: "d", Name: "t"},
		"no table name":       {Database: "d", Columns: []database.ColumnSpec{{Name: "a", Type: "INT"}}},
		"unknown type":        col(database.ColumnSpec{Name: "a", Type: "INT; DROP TABLE x"}),
		"varchar w/o length":  col(database.ColumnSpec{Name: "a", Type: "VARCHAR"}),
		"bad length":          col(database.ColumnSpec{Name: "a", Type: "VARCHAR", Length: "10)"}),
		"text with length":    col(database.ColumnSpec{Name: "a", Type: "TEXT", Length: "10"}),
		"unsigned varchar":    col(database.ColumnSpec{Name: "a", Type: "VARCHAR", Length: "5", Unsigned: true}),
		"ai on varchar":       col(database.ColumnSpec{Name: "a", Type: "VARCHAR", Length: "5", AutoIncrement: true, PrimaryKey: true}),
		"ai not indexed":      col(database.ColumnSpec{Name: "a", Type: "INT", AutoIncrement: true}),
		"null default notnul": col(database.ColumnSpec{Name: "a", Type: "INT", DefaultKind: "null"}),
		"ts default on int":   col(database.ColumnSpec{Name: "a", Type: "INT", DefaultKind: "current_timestamp"}),
		"empty enum":          col(database.ColumnSpec{Name: "a", Type: "ENUM"}),
		"datetime fsp 255":    col(database.ColumnSpec{Name: "a", Type: "DATETIME", Length: "255"}),
		"timestamp fsp 7":     col(database.ColumnSpec{Name: "a", Type: "TIMESTAMP", Length: "7"}),
		"bad engine":          {Database: "d", Name: "t", Engine: "Inno DB", Columns: []database.ColumnSpec{{Name: "a", Type: "INT"}}},
		"duplicate column":    {Database: "d", Name: "t", Columns: []database.ColumnSpec{{Name: "a", Type: "INT"}, {Name: "A", Type: "INT"}}},
		"index unknown col":   {Database: "d", Name: "t", Columns: []database.ColumnSpec{{Name: "a", Type: "INT"}}, Indexes: []database.IndexSpec{{Columns: []string{"b"}}}},
		"two ai":              {Database: "d", Name: "t", Columns: []database.ColumnSpec{{Name: "a", Type: "INT", AutoIncrement: true, PrimaryKey: true}, {Name: "b", Type: "INT", AutoIncrement: true, Unique: true}}},
	}
	for name, spec := range bad {
		if s, err := New().CreateTableSQL(spec); err == nil {
			t.Errorf("%s: accepted\n%s", name, s)
		}
	}
	// AUTO_INCREMENT covered by a secondary index is valid.
	if _, err := New().CreateTableSQL(database.CreateTableSpec{Database: "d", Name: "t", Columns: []database.ColumnSpec{{Name: "a", Type: "INT", AutoIncrement: true}},
		Indexes: []database.IndexSpec{{Columns: []string{"a"}}}}); err != nil {
		t.Fatal(err)
	}
}

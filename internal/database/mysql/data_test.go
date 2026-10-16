package mysql

import (
	"strings"
	"testing"

	"anansi/internal/database"
)

var dataCols = []database.ColumnInfo{
	{Name: "id", DataType: "int", ColumnType: "int", Nullable: false},
	{Name: "email", DataType: "varchar", ColumnType: "varchar(100)", Nullable: false},
	{Name: "bio", DataType: "text", ColumnType: "text", Nullable: true},
	{Name: "foto", DataType: "blob", ColumnType: "blob", Nullable: true},
	{Name: "nota", DataType: "float", ColumnType: "float", Nullable: true},
	{Name: "flags", DataType: "bit", ColumnType: "bit(4)", Nullable: true},
}

func TestFindKey(t *testing.T) {
	d := New()
	k := d.FindKey(dataCols, []database.IndexInfo{{Name: "uq", Columns: []string{"email"}, Unique: true}, {Name: "PRIMARY", Columns: []string{"id"}, Unique: true, Primary: true}})
	if k.Kind != "primary" || k.Columns[0] != "id" {
		t.Fatalf("%+v", k)
	}
	k = d.FindKey(dataCols, []database.IndexInfo{{Name: "uq_bio", Columns: []string{"bio"}, Unique: true}, {Name: "uq", Columns: []string{"email"}, Unique: true}})
	if k.Kind != "unique" || k.Name != "uq" {
		t.Fatalf("nullable unique index must be skipped: %+v", k)
	}
	if k = d.FindKey(dataCols, nil); k.Kind != "none" {
		t.Fatalf("%+v", k)
	}
}

func TestSelectSQL(t *testing.T) {
	d := New()
	key := database.TableKey{Kind: "primary", Columns: []string{"id"}}
	p, err := d.SelectSQL(database.BrowseSpec{Database: "db", Table: "t", Columns: []string{"bio", "foto"}, Filter: "WHERE id > 3;", Sort: []database.SortSpec{{Column: "email", Desc: true}, {Column: "id"}}, Offset: 20, Limit: 10, PartialLength: 100}, dataCols, key)
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT LEFT(`bio`, 100) AS `bio`, LEFT(`foto`, 50) AS `foto`, `id`, (CHAR_LENGTH(`bio`) > 100), (LENGTH(`foto`) > 50) FROM `db`.`t` WHERE id > 3 ORDER BY `email` DESC, `id` LIMIT 20, 10"
	if p.SQL != want || strings.Join(p.Columns, ",") != "bio,foto,id" || !p.Partial[0] || p.Partial[2] {
		t.Fatalf("got %s %v %v", p.SQL, p.Columns, p.Partial)
	}
	// Without a key nothing is truncated (every column identifies the row).
	p, _ = d.SelectSQL(database.BrowseSpec{Database: "db", Table: "t", PartialLength: 100}, dataCols, database.TableKey{Kind: "none"})
	if strings.Contains(p.SQL, "LEFT(") || !strings.HasSuffix(p.SQL, "LIMIT 1000") {
		t.Fatal(p.SQL)
	}
	if _, err := d.SelectSQL(database.BrowseSpec{Database: "db", Table: "t", Columns: []string{"nope"}}, dataCols, key); err == nil {
		t.Fatal("unknown column accepted")
	}
	if _, err := d.SelectSQL(database.BrowseSpec{Database: "db", Table: "t", Sort: []database.SortSpec{{Column: "x`; DROP"}}}, dataCols, key); err == nil {
		t.Fatal("unknown sort column accepted")
	}
}

func TestRowSQL(t *testing.T) {
	d := New()
	key := database.TableKey{Kind: "primary", Columns: []string{"id"}}
	v := func(s string) database.CellValue { return database.CellValue{Kind: database.ValueText, Text: s} }
	st, err := d.RowSQL("db", "t", dataCols, key, database.RowEdit{Op: database.EditUpdate,
		Where: []database.ColumnValue{{Column: "id", Value: v("7")}},
		Values: []database.ColumnValue{{Column: "email", Value: v("a'b")}, {Column: "bio", Value: database.CellValue{Kind: database.ValueNull}},
			{Column: "foto", Value: database.CellValue{Kind: database.ValueHex, Text: "0xCAFE"}}, {Column: "flags", Value: v("b'1010'")}}})
	if err != nil {
		t.Fatal(err)
	}
	if st.SQL != "UPDATE `db`.`t` SET `email` = ?, `bio` = NULL, `foto` = ?, `flags` = ? WHERE `id` = ?" || len(st.Args) != 4 || st.Args[2] != uint64(10) {
		t.Fatalf("%s %v", st.SQL, st.Args)
	}
	if st.Display != "UPDATE `db`.`t` SET `email` = 'a''b', `bio` = NULL, `foto` = 0xCAFE, `flags` = 10 WHERE `id` = '7'" {
		t.Fatal(st.Display)
	}
	// No key: every column compared, floats as text, LIMIT 1.
	st, err = d.RowSQL("db", "t", dataCols, database.TableKey{Kind: "none"}, database.RowEdit{Op: database.EditDelete,
		Where: []database.ColumnValue{{Column: "id", Value: v("1")}, {Column: "nota", Value: v("1.1")}, {Column: "bio", Value: database.CellValue{Kind: database.ValueNull}}}})
	if err != nil || st.SQL != "DELETE FROM `db`.`t` WHERE `id` = ? AND CAST(`nota` AS CHAR) = ? AND `bio` IS NULL LIMIT 1" {
		t.Fatalf("%v %s", err, st.SQL)
	}
	// Key not fully covered → LIMIT 1 as well.
	st, _ = d.RowSQL("db", "t", dataCols, key, database.RowEdit{Op: database.EditDelete, Where: []database.ColumnValue{{Column: "email", Value: v("x")}}})
	if !strings.HasSuffix(st.SQL, "LIMIT 1") {
		t.Fatal(st.SQL)
	}
	st, err = d.RowSQL("db", "t", dataCols, key, database.RowEdit{Op: database.EditInsert, Values: []database.ColumnValue{{Column: "email", Value: database.CellValue{Kind: database.ValueExpression, Text: "UUID()"}}, {Column: "nota", Value: database.CellValue{Kind: database.ValueDefault}}}})
	if err != nil || st.SQL != "INSERT INTO `db`.`t` (`email`, `nota`) VALUES ((UUID()), DEFAULT)" {
		t.Fatalf("%v %s", err, st.SQL)
	}
	for _, bad := range []database.RowEdit{
		{Op: database.EditUpdate, Values: []database.ColumnValue{{Column: "email", Value: v("x")}}},
		{Op: database.EditUpdate, Where: []database.ColumnValue{{Column: "id", Value: v("1")}}, Values: []database.ColumnValue{{Column: "email", Value: database.CellValue{Kind: database.ValueExpression, Text: "1; DROP TABLE t"}}}},
		{Op: database.EditUpdate, Where: []database.ColumnValue{{Column: "id", Value: v("1")}}, Values: []database.ColumnValue{{Column: "foto", Value: database.CellValue{Kind: database.ValueHex, Text: "XYZ"}}}},
		{Op: database.EditUpdate, Where: []database.ColumnValue{{Column: "id", Value: v("1")}}, Values: []database.ColumnValue{{Column: "flags", Value: v("abc")}}},
		{Op: database.EditDelete, Where: []database.ColumnValue{{Column: "id", Value: database.CellValue{Kind: database.ValueExpression, Text: "1 OR 1"}}}},
		{Op: "merge"},
	} {
		if _, err := d.RowSQL("db", "t", dataCols, key, bad); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
}

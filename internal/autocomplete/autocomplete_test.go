package autocomplete

import (
	"context"
	"errors"
	"strings"
	"testing"

	"dbstudio/internal/database"
)

type fakeCatalog struct {
	fail bool
}

var fakeTables = map[string]map[string][]string{
	"shop": {
		"users":       {"id", "name", "email", "created_at"},
		"orders":      {"id", "user_id", "total", "status"},
		"order items": {"order_id", "product_id", "qty"},
		"select":      {"id"},
	},
	"crm": {
		"contacts": {"id", "full_name"},
	},
}

func (f fakeCatalog) Databases(ctx context.Context) ([]database.DatabaseInfo, error) {
	if f.fail {
		return nil, errors.New("offline")
	}
	return []database.DatabaseInfo{{Name: "crm"}, {Name: "shop"}}, nil
}

func (f fakeCatalog) Tables(ctx context.Context, db string) ([]database.TableInfo, error) {
	if f.fail {
		return nil, errors.New("offline")
	}
	var out []database.TableInfo
	for name := range fakeTables[db] {
		out = append(out, database.TableInfo{Name: name, Type: "table"})
	}
	return out, nil
}

func (f fakeCatalog) Columns(ctx context.Context, db, table string) ([]database.ColumnInfo, error) {
	if f.fail {
		return nil, errors.New("offline")
	}
	var out []database.ColumnInfo
	for i, c := range fakeTables[db][table] {
		out = append(out, database.ColumnInfo{Name: c, Position: i + 1, ColumnType: "int", Nullable: true})
	}
	return out, nil
}

func (f fakeCatalog) Routines(ctx context.Context, db string) ([]database.RoutineInfo, error) {
	return []database.RoutineInfo{{Name: "refresh_stats", Type: "PROCEDURE"}, {Name: "fmt_money", Type: "FUNCTION"}}, nil
}

// complete runs the engine with the cursor at the "|" marker.
func complete(t *testing.T, sqlWithCursor string) Response {
	t.Helper()
	i := strings.Index(sqlWithCursor, "|")
	if i < 0 {
		t.Fatal("missing cursor marker")
	}
	sql := sqlWithCursor[:i] + sqlWithCursor[i+1:]
	return Engine{}.Complete(context.Background(), fakeCatalog{}, Request{SQL: sql, Offset: utf16Len(sqlWithCursor[:i]), Database: "shop"})
}

func labels(r Response, kind string) []string {
	var out []string
	for _, it := range r.Items {
		if kind == "" || it.Kind == kind {
			out = append(out, it.Label)
		}
	}
	return out
}

func has(list []string, want ...string) bool {
	set := map[string]bool{}
	for _, l := range list {
		set[l] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}

func hasNone(list []string, bad ...string) bool {
	for _, l := range list {
		for _, b := range bad {
			if l == b {
				return false
			}
		}
	}
	return true
}

func TestStatementStart(t *testing.T) {
	r := complete(t, "SEL|")
	if !has(labels(r, KindKeyword), "SELECT") || r.ReplaceLen != 3 {
		t.Fatalf("got %v replaceLen=%d", labels(r, ""), r.ReplaceLen)
	}
}

func TestTablesAfterFrom(t *testing.T) {
	r := complete(t, "SELECT * FROM |")
	if r.Clause != "from" || !has(labels(r, KindTable), "users", "orders") || !has(labels(r, KindDatabase), "crm") {
		t.Fatalf("clause=%s items=%v", r.Clause, labels(r, ""))
	}
	if !hasNone(labels(r, ""), "id", "COUNT") {
		t.Fatal("columns/functions must not be offered after FROM")
	}
}

func TestTablesAfterJoinWithPrefix(t *testing.T) {
	r := complete(t, "SELECT * FROM users u JOIN ord|")
	got := labels(r, KindTable)
	if !has(got, "orders") || !hasNone(got, "users") {
		t.Fatalf("got %v", got)
	}
}

func TestColumnsInSelectFromLaterFrom(t *testing.T) {
	// Cursor in the select list, FROM written afterwards.
	r := complete(t, "SELECT | FROM users")
	if r.Clause != "select" || !has(labels(r, KindColumn), "id", "name", "email") {
		t.Fatalf("clause=%s items=%v", r.Clause, labels(r, ""))
	}
}

func TestAliasQualifiedColumns(t *testing.T) {
	r := complete(t, "SELECT o.| FROM users AS u JOIN orders o ON o.user_id = u.id")
	cols := labels(r, KindColumn)
	if !has(cols, "user_id", "total") || !hasNone(cols, "email") {
		t.Fatalf("got %v", cols)
	}
	if !hasNone(labels(r, ""), "SELECT", "COUNT") {
		t.Fatal("only columns expected after alias dot")
	}
}

func TestTableQualifiedColumns(t *testing.T) {
	r := complete(t, "SELECT users.| FROM users")
	if !has(labels(r, KindColumn), "email") {
		t.Fatalf("got %v", labels(r, ""))
	}
}

func TestDatabaseQualifiedTables(t *testing.T) {
	r := complete(t, "SELECT * FROM crm.|")
	if got := labels(r, KindTable); !has(got, "contacts") || !hasNone(got, "users") {
		t.Fatalf("got %v", got)
	}
}

func TestDatabaseTableQualifiedColumns(t *testing.T) {
	r := complete(t, "SELECT crm.contacts.| FROM crm.contacts")
	if !has(labels(r, KindColumn), "full_name") {
		t.Fatalf("got %v", labels(r, ""))
	}
}

func TestCrossDatabaseAlias(t *testing.T) {
	r := complete(t, "SELECT c.| FROM crm.contacts c")
	if !has(labels(r, KindColumn), "full_name") {
		t.Fatalf("got %v", labels(r, ""))
	}
}

func TestWhereColumnsAndKeywords(t *testing.T) {
	r := complete(t, "SELECT * FROM orders WHERE st|")
	if !has(labels(r, KindColumn), "status") {
		t.Fatalf("got %v", labels(r, ""))
	}
	r = complete(t, "SELECT * FROM orders WHERE status = 1 |")
	if !has(labels(r, KindKeyword), "AND", "ORDER BY") {
		t.Fatalf("got %v", labels(r, KindKeyword))
	}
}

func TestKeywordsAfterTable(t *testing.T) {
	r := complete(t, "SELECT * FROM users |")
	if r.Clause != "afterTable" || !has(labels(r, KindKeyword), "WHERE", "LEFT JOIN", "ORDER BY") {
		t.Fatalf("clause=%s got %v", r.Clause, labels(r, ""))
	}
	r = complete(t, "SELECT * FROM users u WH|")
	if !has(labels(r, KindKeyword), "WHERE") {
		t.Fatalf("got %v", labels(r, ""))
	}
}

func TestOrderByAndGroupBy(t *testing.T) {
	r := complete(t, "SELECT status, COUNT(*) FROM orders GROUP BY |")
	if r.Clause != "groupby" || !has(labels(r, KindColumn), "status") {
		t.Fatalf("clause=%s got %v", r.Clause, labels(r, ""))
	}
	r = complete(t, "SELECT * FROM orders ORDER BY |")
	if r.Clause != "orderby" || !has(labels(r, KindColumn), "total") {
		t.Fatalf("clause=%s got %v", r.Clause, labels(r, ""))
	}
}

func TestJoinOnBothTables(t *testing.T) {
	r := complete(t, "SELECT * FROM users u JOIN orders o ON |")
	cols := labels(r, KindColumn)
	if r.Clause != "on" || !has(cols, "email", "user_id") || !has(labels(r, KindAlias), "u", "o") {
		t.Fatalf("clause=%s got %v", r.Clause, labels(r, ""))
	}
}

func TestSubqueryScopes(t *testing.T) {
	// Inside the subquery both its own table and the outer one are visible.
	r := complete(t, "SELECT * FROM users u WHERE EXISTS (SELECT 1 FROM orders o WHERE o.user_id = u.id AND |)")
	if !has(labels(r, KindAlias), "o", "u") {
		t.Fatalf("got %v", labels(r, KindAlias))
	}
	// The outer query does not see the subquery's tables.
	r = complete(t, "SELECT | FROM users u WHERE EXISTS (SELECT 1 FROM orders o)")
	if has(labels(r, KindAlias), "o") {
		t.Fatalf("subquery alias leaked: %v", labels(r, KindAlias))
	}
}

func TestDerivedTableAlias(t *testing.T) {
	r := complete(t, "SELECT | FROM (SELECT id FROM users) AS x")
	if !has(labels(r, KindAlias), "x") {
		t.Fatalf("got %v", labels(r, KindAlias))
	}
}

func TestQuotedIdentifiers(t *testing.T) {
	r := complete(t, "SELECT * FROM `ord|")
	var ins []string
	for _, it := range r.Items {
		if it.Kind == KindTable {
			ins = append(ins, it.InsertText)
		}
	}
	if !has(ins, "`orders`", "`order items`") || r.ReplaceLen != 4 {
		t.Fatalf("got %v replaceLen=%d", ins, r.ReplaceLen)
	}
	// Names that need quoting are quoted even without a backtick.
	r = complete(t, "SELECT * FROM |")
	for _, it := range r.Items {
		if it.Label == "order items" && it.InsertText != "`order items`" {
			t.Fatalf("got %q", it.InsertText)
		}
		if it.Label == "select" && it.InsertText != "`select`" {
			t.Fatalf("reserved word not quoted: %q", it.InsertText)
		}
	}
	// Quoted table names resolve in the FROM clause.
	r = complete(t, "SELECT x.| FROM `order items` x")
	if !has(labels(r, KindColumn), "product_id") {
		t.Fatalf("got %v", labels(r, ""))
	}
}

func TestInsertColumns(t *testing.T) {
	r := complete(t, "INSERT INTO orders (|")
	if r.Clause != "insertColumns" || !has(labels(r, KindColumn), "user_id", "total") {
		t.Fatalf("clause=%s got %v", r.Clause, labels(r, ""))
	}
}

func TestUpdateSet(t *testing.T) {
	r := complete(t, "UPDATE orders SET |")
	if r.Clause != "set" || !has(labels(r, KindColumn), "status") {
		t.Fatalf("clause=%s got %v", r.Clause, labels(r, ""))
	}
	r = complete(t, "UPDATE |")
	if !has(labels(r, KindTable), "orders") {
		t.Fatalf("got %v", labels(r, ""))
	}
}

func TestUseAndCall(t *testing.T) {
	r := complete(t, "USE |")
	if !has(labels(r, KindDatabase), "crm", "shop") || len(labels(r, KindTable)) != 0 {
		t.Fatalf("got %v", labels(r, ""))
	}
	r = complete(t, "CALL |")
	if !has(labels(r, KindRoutine), "refresh_stats") || has(labels(r, KindRoutine), "fmt_money") {
		t.Fatalf("got %v", labels(r, ""))
	}
}

func TestNoCompletionInStringsAndComments(t *testing.T) {
	for _, s := range []string{"SELECT 'abc|", "SELECT 1 -- note |", "SELECT /* x | */ 1"} {
		if r := complete(t, s); len(r.Items) != 0 {
			t.Errorf("%q: expected no items, got %v", s, labels(r, ""))
		}
	}
}

func TestMultipleStatementsUseCursorStatement(t *testing.T) {
	r := complete(t, "SELECT * FROM orders o; SELECT | FROM users u")
	if has(labels(r, KindAlias), "o") || !has(labels(r, KindAlias), "u") {
		t.Fatalf("got %v", labels(r, KindAlias))
	}
}

func TestShowColumnsAndDescribe(t *testing.T) {
	for _, s := range []string{"SHOW COLUMNS FROM |", "DESCRIBE |", "ALTER TABLE |", "TRUNCATE TABLE |", "DROP TABLE |"} {
		if r := complete(t, s); !has(labels(r, KindTable), "users") {
			t.Errorf("%q: got %v", s, labels(r, ""))
		}
	}
}

func TestMetadataFailureDegrades(t *testing.T) {
	r := Engine{}.Complete(context.Background(), fakeCatalog{fail: true}, Request{SQL: "SELECT * FROM ", Offset: 14, Database: "shop"})
	if r.MetadataNote == "" {
		t.Fatal("expected metadata note")
	}
	r = Engine{}.Complete(context.Background(), fakeCatalog{fail: true}, Request{SQL: "SEL", Offset: 3, Database: "shop"})
	if !has(labels(r, KindKeyword), "SELECT") {
		t.Fatal("keywords must still be offered")
	}
}

func TestUTF16Offsets(t *testing.T) {
	// "😀" is two UTF-16 units and four bytes.
	sql := "SELECT '😀', na FROM users"
	off := utf16Len("SELECT '😀', na")
	r := Engine{}.Complete(context.Background(), fakeCatalog{}, Request{SQL: sql, Offset: off, Database: "shop"})
	if !has(labels(r, KindColumn), "name") || r.ReplaceLen != 2 {
		t.Fatalf("got %v replaceLen=%d", labels(r, ""), r.ReplaceLen)
	}
}

package integration

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"testing"

	"anansi/internal/api"
	"anansi/internal/apperr"
	"anansi/internal/connections"
	"anansi/internal/database"
	"anansi/internal/query"
	"anansi/internal/security"
)

// newApp builds the real application facade on a temporary config dir.
func newApp(t *testing.T, tg target, mutate ...func(*connections.Profile)) (*api.App, string) {
	t.Helper()
	t.Setenv("ANANSI_CONFIG_DIR", t.TempDir())
	app, err := api.New(slog.New(slog.NewTextHandler(io.Discard, nil)), security.NewMemory())
	if err != nil {
		t.Fatal(err)
	}
	host, portS, _ := net.SplitHostPort(tg.addr)
	port, _ := strconv.Atoi(portS)
	pw := password()
	p := connections.Profile{Name: "IT " + tg.name, Kind: tg.kind, Host: host, Port: port, User: "root", Database: "studio_it",
		TLS: database.TLSOptions{Mode: database.TLSPreferred}, SavePassword: true, Environment: connections.EnvLocal}
	for _, m := range mutate {
		m(&p)
	}
	res, err := app.SaveConnection(connections.SaveInput{Profile: p, Password: &pw})
	if err != nil {
		t.Fatal(err)
	}
	id := res.Profile.ID
	if _, err := app.Connect(id, nil, false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Shutdown(t.Context()) })
	return app, id
}

func mustAdmin(t *testing.T, app *api.App, id string, stmts ...string) {
	t.Helper()
	for _, s := range stmts {
		resp, err := app.Execute(queryReq(id, s))
		if err != nil {
			t.Fatalf("%s: %v", s, err)
		}
		for _, r := range resp.Results {
			if r.Error != nil {
				t.Fatalf("%s: %s", s, r.Error.Message)
			}
		}
	}
}

func cell(rows [][]any, r, c int) string {
	if rows[r][c] == nil {
		return "<NULL>"
	}
	return fmt.Sprint(rows[r][c])
}

func colIndex(d *api.TableData, name string) int {
	for i, c := range d.Columns {
		if c.Name == name {
			return i
		}
	}
	return -1
}

func val(s string) database.CellValue { return database.CellValue{Kind: database.ValueText, Text: s} }

func TestTableDataBrowseAndEdit(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			app, id := newApp(t, tg)
			db := uniq("it_data")
			mustAdmin(t, app, id,
				"CREATE DATABASE `"+db+"`",
				"CREATE TABLE `"+db+"`.itens (id INT PRIMARY KEY AUTO_INCREMENT, nome VARCHAR(80) NOT NULL DEFAULT 'sem nome', nota TEXT, bin BLOB, preco FLOAT, quando DATETIME(3), flag BIT(3), doc JSON, tipo ENUM('a','b') DEFAULT 'a')",
				"INSERT INTO `"+db+"`.itens (nome, nota, bin, preco, quando, flag, doc) VALUES ('um', REPEAT('x', 3000), REPEAT(0x41, 2000), 1.1, '2024-01-02 03:04:05.678', b'101', '{\"a\": 1}'), ('dois', 'curta', 0x00FF, 2.5, NULL, b'1', NULL), ('três', NULL, NULL, NULL, NULL, NULL, NULL)",
				"CREATE TABLE `"+db+"`.semchave (a INT, b FLOAT, c DATETIME, d JSON)",
				"INSERT INTO `"+db+"`.semchave VALUES (1, 1.1, '2020-01-01 00:00:00', '[1,2]'), (1, 1.1, '2020-01-01 00:00:00', '[1,2]')",
			)
			t.Cleanup(func() { mustAdmin(t, app, id, "DROP DATABASE `"+db+"`") })

			// Browse with partial loading and multi-column sort.
			d, err := app.BrowseTable("grid-1", id, database.BrowseSpec{Database: db, Table: "itens", Sort: []database.SortSpec{{Column: "nome", Desc: true}}, Limit: 2, PartialLength: 1000})
			if err != nil {
				t.Fatal(err)
			}
			if d.Key.Kind != "primary" || len(d.Rows) != 2 || !d.HasMore || len(d.Columns) != 9 {
				t.Fatalf("browse: key=%+v rows=%d more=%v cols=%d sql=%s", d.Key, len(d.Rows), d.HasMore, len(d.Columns), d.SQL)
			}
			nota, bin := colIndex(d, "nota"), colIndex(d, "bin")
			if cell(d.Rows, 0, colIndex(d, "nome")) != "um" || len([]rune(cell(d.Rows, 0, nota))) != 1000 {
				t.Fatalf("order/partial: %q", cell(d.Rows, 0, colIndex(d, "nome")))
			}
			trunc := map[[2]int]bool{}
			for _, tc := range d.TruncatedCells {
				trunc[tc] = true
			}
			if !trunc[[2]int{0, nota}] || !trunc[[2]int{0, bin}] || trunc[[2]int{1, nota}] {
				t.Fatalf("truncated flags %v", d.TruncatedCells)
			}
			// Second page.
			d2, err := app.BrowseTable("grid-1", id, database.BrowseSpec{Database: db, Table: "itens", Sort: []database.SortSpec{{Column: "nome", Desc: true}}, Offset: 2, Limit: 2})
			if err != nil || len(d2.Rows) != 1 || d2.HasMore {
				t.Fatalf("page 2: %v %+v", err, d2)
			}
			// Filter + count.
			n, err := app.CountTableRows("grid-1", id, db, "itens", "WHERE preco > 2")
			if err != nil || n != 1 {
				t.Fatalf("count %d %v", n, err)
			}
			// Full value of a truncated cell.
			full, err := app.FetchRowValues(id, db, "itens", []database.ColumnValue{{Column: "id", Value: val("1")}}, []string{"nota", "bin"})
			if err != nil || len(fmt.Sprint(full[0])) != 3000 || len(fmt.Sprint(full[1])) != 2+4000 {
				t.Fatalf("full row: %v len=%d", err, len(fmt.Sprint(full[0])))
			}

			// Edits in one transaction, values bound as arguments. The
			// tricky string survives NO_BACKSLASH_ESCAPES.
			mustAdmin(t, app, id, "SET GLOBAL sql_mode = CONCAT(@@GLOBAL.sql_mode, ',NO_BACKSLASH_ESCAPES')")
			defer mustAdmin(t, app, id, "SET GLOBAL sql_mode = REPLACE(@@GLOBAL.sql_mode, 'NO_BACKSLASH_ESCAPES', '')")
			app.Disconnect(id)
			if _, err := app.Connect(id, nil, false); err != nil {
				t.Fatal(err)
			}
			tricky := `C:\pasta\'aspas' "duplas" ; -- não é comentário`
			res, err := app.ApplyRowEdits(id, db, "itens", []database.RowEdit{
				{Op: database.EditUpdate, Where: []database.ColumnValue{{Column: "id", Value: val("2")}}, Values: []database.ColumnValue{
					{Column: "nome", Value: val(tricky)},
					{Column: "nota", Value: database.CellValue{Kind: database.ValueNull}},
					{Column: "bin", Value: database.CellValue{Kind: database.ValueHex, Text: "DEADBEEF"}},
					{Column: "flag", Value: val("6")},
					{Column: "quando", Value: database.CellValue{Kind: database.ValueExpression, Text: "'2030-05-06 07:08:09.123'"}},
				}},
				{Op: database.EditInsert, Values: []database.ColumnValue{{Column: "nota", Value: val("nova")}, {Column: "tipo", Value: database.CellValue{Kind: database.ValueDefault}}}},
				{Op: database.EditDelete, Where: []database.ColumnValue{{Column: "id", Value: val("3")}}},
			}, true)
			if err != nil || !res.Applied {
				t.Fatalf("apply: %v %+v", err, res)
			}
			if res.Results[1].LastInsertID == nil || *res.Results[1].LastInsertID != 4 {
				t.Fatalf("last insert id %+v", res.Results[1])
			}
			if !strings.Contains(res.Results[0].SQL, "0xDEADBEEF") {
				t.Fatalf("display SQL %q", res.Results[0].SQL)
			}
			after, err := app.BrowseTable("grid-1", id, database.BrowseSpec{Database: db, Table: "itens", Sort: []database.SortSpec{{Column: "id"}}})
			if err != nil {
				t.Fatal(err)
			}
			if len(after.Rows) != 3 || cell(after.Rows, 1, colIndex(after, "nome")) != tricky || cell(after.Rows, 1, colIndex(after, "nota")) != "<NULL>" ||
				cell(after.Rows, 1, colIndex(after, "bin")) != "0xDEADBEEF" || cell(after.Rows, 1, colIndex(after, "flag")) != "0x06" ||
				!strings.HasPrefix(cell(after.Rows, 1, colIndex(after, "quando")), "2030-05-06 07:08:09.123") ||
				cell(after.Rows, 2, colIndex(after, "nome")) != "sem nome" || cell(after.Rows, 2, colIndex(after, "tipo")) != "a" {
				t.Fatalf("after edits: %v", after.Rows)
			}

			// A failure rolls the whole batch back.
			res, err = app.ApplyRowEdits(id, db, "itens", []database.RowEdit{
				{Op: database.EditUpdate, Where: []database.ColumnValue{{Column: "id", Value: val("1")}}, Values: []database.ColumnValue{{Column: "nome", Value: val("alterado")}}},
				{Op: database.EditInsert, Values: []database.ColumnValue{{Column: "id", Value: val("1")}}},
			}, true)
			if err != nil || res.Applied || res.Results[len(res.Results)-1].Error == nil {
				t.Fatalf("expected failure: %v %+v", err, res)
			}
			chk, _ := app.BrowseTable("grid-1", id, database.BrowseSpec{Database: db, Table: "itens", Filter: "id = 1"})
			if cell(chk.Rows, 0, colIndex(chk, "nome")) != "um" {
				t.Fatal("rollback did not undo the first update")
			}

			// Table without key: float, datetime and JSON are compared as
			// text and LIMIT 1 touches a single duplicate.
			nk, err := app.BrowseTable("grid-2", id, database.BrowseSpec{Database: db, Table: "semchave", PartialLength: 1000})
			if err != nil || nk.Key.Kind != "none" || len(nk.Partial) != 4 || nk.Partial[3] {
				t.Fatalf("no key browse %v %+v", err, nk)
			}
			var where []database.ColumnValue
			for i, c := range nk.Columns {
				where = append(where, database.ColumnValue{Column: c.Name, Value: val(cell(nk.Rows, 0, i))})
			}
			res, err = app.ApplyRowEdits(id, db, "semchave", []database.RowEdit{{Op: database.EditUpdate, Where: where, Values: []database.ColumnValue{{Column: "a", Value: val("9")}}}}, true)
			if err != nil || !res.Applied || *res.Results[0].RowsAffected != 1 {
				t.Fatalf("no-key update: %v %+v", err, res)
			}
			if n, _ := app.CountTableRows("g", id, db, "semchave", "a = 9"); n != 1 {
				t.Fatalf("no-key update touched %d rows", n)
			}

			// Quick filter helpers.
			dv, err := app.DistinctValues(id, db, "itens", "tipo", "")
			if err != nil || len(dv) != 1 || fmt.Sprint(dv[0].Value) != "a" || dv[0].Count != 3 {
				t.Fatalf("distinct %v %+v", err, dv)
			}
			lv, err := app.LookupValues(id, db, "itens", "id", "sem")
			if err != nil || len(lv) != 1 || fmt.Sprint(lv[0].Value) != "4" || lv[0].Label != "sem nome" {
				t.Fatalf("lookup %v %+v", err, lv)
			}
			// Filter with a syntax error reports the server error.
			if _, err := app.BrowseTable("grid-1", id, database.BrowseSpec{Database: db, Table: "itens", Filter: "nome = = 1"}); err == nil {
				t.Fatal("expected syntax error")
			}
			// Production profiles ask before writing; read-only refuses.
			ro, roID := newApp(t, tg, func(p *connections.Profile) { p.ReadOnly = true })
			_, err = ro.ApplyRowEdits(roID, db, "itens", []database.RowEdit{{Op: database.EditDelete, Where: []database.ColumnValue{{Column: "id", Value: val("1")}}}}, true)
			if ae := appErr(err); ae == nil || ae.Code != apperr.CodeReadOnly {
				t.Fatalf("read-only edit: %v", err)
			}
			prod, prodID := newApp(t, tg, func(p *connections.Profile) { p.Environment = connections.EnvProduction })
			_, err = prod.ApplyRowEdits(prodID, db, "itens", []database.RowEdit{{Op: database.EditDelete, Where: []database.ColumnValue{{Column: "id", Value: val("1")}}}}, false)
			if ae := appErr(err); ae == nil || ae.Code != apperr.CodeConfirmationRequired || !strings.Contains(fmt.Sprint(ae.Details), "WHERE `id` = '1'") {
				t.Fatalf("production edit: %v", err)
			}
		})
	}
}

func queryReq(id, sql string) query.ExecuteRequest {
	return query.ExecuteRequest{TabID: "it-admin-tab-0001", ConnectionID: id, SQL: sql, CursorOffset: -1, Mode: "script", Confirmed: true}
}

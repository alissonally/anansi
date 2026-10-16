package integration

import (
	"strings"
	"testing"

	"dbstudio/internal/query"
)

func TestQueryHelpers(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			app, id := newApp(t, tg)
			db := uniq("it_qh")
			mustAdmin(t, app, id, "CREATE DATABASE `"+db+"`", "CREATE TABLE `"+db+"`.t (id INT PRIMARY KEY, nome VARCHAR(5), segredo INT INVISIBLE)", "INSERT INTO `"+db+"`.t (id, nome) VALUES (1, 'a')")
			t.Cleanup(func() { mustAdmin(t, app, id, "DROP DATABASE `"+db+"`") })

			// Warnings are attached to the statement that raised them.
			resp, err := app.Execute(query.ExecuteRequest{TabID: "it-warn-tab-01", ConnectionID: id, SQL: "SELECT CAST('12abc' AS SIGNED) AS n; SELECT 1", CursorOffset: -1, Mode: "script", Database: db})
			if err != nil {
				t.Fatal(err)
			}
			if len(resp.Results) != 2 || len(resp.Results[0].Warnings) == 0 || resp.Results[0].Warnings[0].Code != 1292 || len(resp.Results[1].Warnings) != 0 {
				t.Fatalf("warnings %+v", resp.Results)
			}
			// A SHOW WARNINGS typed by the user still sees them.
			resp, err = app.Execute(query.ExecuteRequest{TabID: "it-warn-tab-01", ConnectionID: id, SQL: "SELECT CAST('7x' AS SIGNED); SHOW WARNINGS", CursorOffset: -1, Mode: "script", Database: db})
			if err != nil || len(resp.Results) != 2 || len(resp.Results[1].Rows) != 1 {
				t.Fatalf("user SHOW WARNINGS %v %+v", err, resp)
			}
			// Profiling appends the server profile.
			resp, err = app.Execute(query.ExecuteRequest{TabID: "it-prof-tab-01", ConnectionID: id, SQL: "SELECT COUNT(*) FROM t", CursorOffset: -1, Mode: "statement", Database: db, Profile: true})
			if err != nil {
				t.Fatal(err)
			}
			last := resp.Results[len(resp.Results)-1]
			if len(resp.Results) != 2 || !last.Profile || len(last.Rows) == 0 {
				t.Fatalf("profile %+v", resp.Results)
			}

			// Editable results.
			info, err := app.EditableResult(id, "SELECT * FROM t WHERE id > 0", db, []string{"id", "nome"})
			if err != nil || info == nil || info.Table != "t" || strings.Join(info.Columns, ",") != "id,nome" || info.Key.Kind != "primary" {
				t.Fatalf("editable * %v %+v", err, info)
			}
			info, _ = app.EditableResult(id, "SELECT nome, UPPER(nome) AS u, id FROM `"+db+"`.t x", "", []string{"nome", "u", "id"})
			if info == nil || strings.Join(info.Columns, ",") != "nome,,id" {
				t.Fatalf("editable mixed %+v", info)
			}
			for _, sql := range []string{"SELECT nome FROM t", "SELECT * FROM t a JOIN t b USING (id)", "SELECT id, COUNT(*) FROM t GROUP BY id"} {
				if info, _ := app.EditableResult(id, sql, db, []string{"nome"}); info != nil {
					t.Fatalf("%q should not be editable: %+v", sql, info)
				}
			}

			// Server help.
			h, err := app.SQLHelp(id, "contents")
			if err != nil || h.Kind != "list" || len(h.Items) < 5 {
				t.Fatalf("help contents %v %+v", err, h)
			}
			h, err = app.SQLHelp(id, "CONCAT")
			if err != nil || h.Kind != "topic" || !strings.Contains(strings.ToUpper(h.Description), "CONCAT") {
				t.Fatalf("help CONCAT %v %+v", err, h)
			}
			if h, _ := app.SQLHelp(id, "palavra_que_nao_existe_xyz"); h.Kind != "none" {
				t.Fatalf("help miss %+v", h)
			}

			// Snippets.
			if err := app.SaveSnippet("consulta útil", "SELECT 1;"); err != nil {
				t.Fatal(err)
			}
			if err := app.SaveSnippet("../fora", "x"); err == nil {
				t.Fatal("path traversal accepted")
			}
			if err := app.RenameSnippet("consulta útil", "renomeada"); err != nil {
				t.Fatal(err)
			}
			list, _ := app.ListSnippets()
			body, err := app.ReadSnippet("renomeada")
			if len(list) != 1 || list[0].Name != "renomeada" || err != nil || body != "SELECT 1;" {
				t.Fatalf("snippets %+v %q %v", list, body, err)
			}
			if err := app.DeleteSnippet("renomeada"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

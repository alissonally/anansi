package integration

import (
	"strings"
	"testing"

	"anansi/internal/api"
	"anansi/internal/database"
	"anansi/internal/query"
)

func noErrors(t *testing.T, res []query.AdminResult, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Error != nil {
			t.Fatalf("%s\n→ %s", r.SQL, r.Error.Message)
		}
	}
}

func findCol(def database.TableDef, name string) *database.ColumnDef {
	for i := range def.Columns {
		if def.Columns[i].Name == name {
			return &def.Columns[i]
		}
	}
	return nil
}

func TestTableEditorRoundTripAndAlter(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			app, id := newApp(t, tg)
			db := uniq("it_ed")
			mustAdmin(t, app, id, "CREATE DATABASE `"+db+"`", "CREATE TABLE `"+db+"`.pais (id INT PRIMARY KEY, nome VARCHAR(20))")
			t.Cleanup(func() { mustAdmin(t, app, id, "DROP DATABASE `"+db+"`") })
			maria := tg.kind == database.MariaDB

			def := database.TableDef{Database: db, Name: "itens", Options: database.TableOptions{Engine: "InnoDB", Collation: "utf8mb4_unicode_ci", Comment: "itens do pedido", RowFormat: "DYNAMIC", AutoIncrement: "100"},
				Columns: []database.ColumnDef{
					{Name: "id", Type: "BIGINT", Unsigned: true, AutoIncrement: true},
					{Name: "codigo", Type: "VARCHAR", Length: "40", Collation: "utf8mb4_bin", Comment: "código 'único'"},
					{Name: "status", Type: "ENUM", Length: "'novo','pago','it''s'", DefaultKind: "value", Default: "novo"},
					{Name: "qtd", Type: "INT", DefaultKind: "value", Default: "1"},
					{Name: "preco", Type: "DECIMAL", Length: "10,2", Nullable: true, DefaultKind: "null"},
					{Name: "total", Type: "DECIMAL", Length: "12,2", Nullable: true, Generated: "`qtd` * `preco`", GeneratedKind: "STORED"},
					{Name: "criado", Type: "DATETIME", Length: "3", DefaultKind: "expression", Default: "CURRENT_TIMESTAMP(3)", OnUpdate: "CURRENT_TIMESTAMP(3)"},
					{Name: "nota", Type: "TEXT", Nullable: true, DefaultKind: "null"},
					{Name: "segredo", Type: "VARCHAR", Length: "10", Nullable: true, DefaultKind: "null", Invisible: true},
					{Name: "pais_id", Type: "INT", Nullable: true, DefaultKind: "null"},
				},
				Indexes: []database.IndexDef{
					{Kind: "PRIMARY", Parts: []database.IndexPart{{Column: "id"}}},
					{Name: "uq_codigo", Kind: "UNIQUE", Parts: []database.IndexPart{{Column: "codigo"}}},
					{Name: "ix_nota", Kind: "KEY", Parts: []database.IndexPart{{Column: "nota", Length: 20}, {Column: "qtd", Desc: true}}, Comment: "parcial"},
					{Name: "ft_nota", Kind: "FULLTEXT", Parts: []database.IndexPart{{Column: "nota"}}},
				},
				ForeignKeys: []database.ForeignKeyDef{{Name: "fk_pais", Columns: []string{"pais_id"}, RefTable: "pais", RefColumns: []string{"id"}, OnDelete: "SET NULL", OnUpdate: "CASCADE"}},
				Checks:      []database.CheckDef{{Name: "chk_qtd", Clause: "`qtd` > 0"}},
			}
			res, err := app.SaveTableDef(id, def, true)
			noErrors(t, res, err)

			cur, err := app.LoadTableDef(id, db, "itens")
			if err != nil {
				t.Fatal(err)
			}
			if c := findCol(cur, "status"); c == nil || c.DefaultKind != "value" || c.Default != "novo" || !strings.Contains(c.Length, "'it''s'") {
				t.Fatalf("status %+v", c)
			}
			if c := findCol(cur, "criado"); c == nil || c.DefaultKind != "expression" || !strings.HasPrefix(strings.ToUpper(c.Default), "CURRENT_TIMESTAMP") || c.OnUpdate == "" {
				t.Fatalf("criado %+v", c)
			}
			if c := findCol(cur, "total"); c == nil || c.Generated == "" || c.GeneratedKind == "" {
				t.Fatalf("total %+v", c)
			}
			if c := findCol(cur, "segredo"); c == nil || !c.Invisible {
				t.Fatalf("segredo %+v", c)
			}
			if c := findCol(cur, "codigo"); c == nil || c.Collation != "utf8mb4_bin" || c.Comment != "código 'único'" {
				t.Fatalf("codigo %+v", c)
			}
			if cur.Options.Comment != "itens do pedido" || cur.Options.RowFormat != "DYNAMIC" || cur.Options.AutoIncrement != "100" || len(cur.ForeignKeys) != 1 || len(cur.Indexes) != 5 {
				t.Fatalf("options/fk/idx %+v %d %d", cur.Options, len(cur.ForeignKeys), len(cur.Indexes))
			}
			// Loading and saving without changes must generate nothing.
			stmts, err := app.PreviewTableDef(id, cur)
			if err != nil || len(stmts) != 0 {
				t.Fatalf("round trip should be a no-op, got %v %q", err, stmts)
			}

			// Edit: rename + move, change type, add, drop, index/FK/check
			// changes, options, partition-free rename of the table.
			ed := cur
			ed.Name = "itens_pedido"
			var cols []database.ColumnDef
			for _, c := range cur.Columns {
				switch c.Name {
				case "nota":
					continue // dropped
				case "qtd":
					c.Name = "quantidade"
					c.Type = "SMALLINT"
					c.Length = "" // the editor resets the length when the type changes
					c.Unsigned = true
				}
				cols = append(cols, c)
			}
			// Move "preco" first after id and add a new column at the end.
			var reordered []database.ColumnDef
			for _, c := range cols {
				if c.Name == "preco" {
					continue
				}
				reordered = append(reordered, c)
				if c.Name == "id" {
					for _, p := range cols {
						if p.Name == "preco" {
							reordered = append(reordered, p)
						}
					}
				}
			}
			reordered = append(reordered, database.ColumnDef{Name: "ativo", Type: "TINYINT", Length: "1", DefaultKind: "value", Default: "1"})
			ed.Columns = reordered
			// Generated column referenced qtd: update its expression.
			for i := range ed.Columns {
				if ed.Columns[i].Name == "total" {
					ed.Columns[i].Generated = "`quantidade` * `preco`"
				}
			}
			ed.Indexes = []database.IndexDef{cur.Indexes[0], cur.Indexes[1], {Name: "ix_ativo", Kind: "KEY", Parts: []database.IndexPart{{Column: "ativo"}, {Column: "quantidade"}}, Invisible: true}}
			ed.ForeignKeys = []database.ForeignKeyDef{cur.ForeignKeys[0]}
			ed.ForeignKeys[0].OnDelete = "CASCADE"
			ed.Checks = []database.CheckDef{{Name: "chk_qtd2", Clause: "`quantidade` < 1000"}}
			ed.Options.Comment = "renomeada"
			ed.Options.Collation = "utf8mb4_general_ci"
			stmts, err = app.PreviewTableDef(id, ed)
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(stmts, ";\n")
			for _, want := range []string{"DROP FOREIGN KEY `fk_pais`", "CHANGE COLUMN `qtd` `quantidade` SMALLINT UNSIGNED", "AFTER `id`", "DROP COLUMN `nota`",
				"ADD COLUMN `ativo` TINYINT(1)", "DROP INDEX `ix_nota`", "ADD CONSTRAINT `fk_pais`", "chk_qtd2", "RENAME TABLE"} {
				if !strings.Contains(joined, want) {
					t.Fatalf("missing %q in:\n%s", want, joined)
				}
			}
			res, err = app.SaveTableDef(id, ed, true)
			noErrors(t, res, err)
			after, err := app.LoadTableDef(id, db, "itens_pedido")
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, c := range after.Columns {
				names = append(names, c.Name)
			}
			if strings.Join(names, ",") != "id,preco,codigo,status,quantidade,total,criado,segredo,pais_id,ativo" {
				t.Fatalf("column order %v", names)
			}
			if c := findCol(after, "quantidade"); c.Type != "SMALLINT" || !c.Unsigned {
				t.Fatalf("quantidade %+v", c)
			}
			if after.ForeignKeys[0].OnDelete != "CASCADE" || len(after.Checks) != 1 || after.Checks[0].Name != "chk_qtd2" || after.Options.Comment != "renomeada" || after.Options.Collation != "utf8mb4_general_ci" {
				t.Fatalf("after %+v %+v %+v", after.ForeignKeys, after.Checks, after.Options)
			}
			inv := false
			for _, ix := range after.Indexes {
				if ix.Name == "ix_ativo" && ix.Invisible {
					inv = true
				}
			}
			if !inv {
				t.Fatalf("invisible index not created: %+v", after.Indexes)
			}
			// And the edited table round-trips too.
			if stmts, err := app.PreviewTableDef(id, after); err != nil || len(stmts) != 0 {
				t.Fatalf("second round trip: %v %q", err, stmts)
			}

			// Partitioning (only on a table without foreign keys).
			pdef := database.TableDef{Database: db, Name: "logs", Columns: []database.ColumnDef{{Name: "id", Type: "INT"}, {Name: "ano", Type: "INT"}},
				Indexes: []database.IndexDef{{Kind: "PRIMARY", Parts: []database.IndexPart{{Column: "id"}, {Column: "ano"}}}},
				Options: database.TableOptions{Partition: "PARTITION BY RANGE (`ano`) (PARTITION p1 VALUES LESS THAN (2020), PARTITION pmax VALUES LESS THAN MAXVALUE)"}}
			res, err = app.SaveTableDef(id, pdef, true)
			noErrors(t, res, err)
			pcur, err := app.LoadTableDef(id, db, "logs")
			if err != nil || !strings.HasPrefix(pcur.Options.Partition, "PARTITION BY RANGE") {
				t.Fatalf("partition %v %q", err, pcur.Options.Partition)
			}
			if stmts, _ := app.PreviewTableDef(id, pcur); len(stmts) != 0 {
				t.Fatalf("partitioned round trip %q", stmts)
			}
			pcur.Options.Partition = ""
			res, err = app.SaveTableDef(id, pcur, true)
			noErrors(t, res, err)
			if again, _ := app.LoadTableDef(id, db, "logs"); again.Options.Partition != "" {
				t.Fatal("REMOVE PARTITIONING did not apply")
			}

			// Without confirmation, dropping a column asks first.
			x := after
			x.Columns = nil
			for _, c := range after.Columns {
				if c.Name != "segredo" {
					x.Columns = append(x.Columns, c)
				}
			}
			if _, err := app.SaveTableDef(id, x, false); appErr(err) == nil || appErr(err).Code != "confirmation_required" {
				t.Fatalf("expected confirmation for DROP COLUMN, got %v", err)
			}
			_ = maria
		})
	}
}

func TestRoutineTriggerEventViewEditors(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			app, id := newApp(t, tg)
			db := uniq("it_ob")
			mustAdmin(t, app, id, "CREATE DATABASE `"+db+"`", "CREATE TABLE `"+db+"`.t (id INT PRIMARY KEY, v INT)")
			t.Cleanup(func() { mustAdmin(t, app, id, "DROP DATABASE `"+db+"`") })

			// Routine: create, replace with a new body, failed replace keeps the old one.
			r := database.RoutineDef{Database: db, Name: "dobro", Type: "FUNCTION", Params: []database.RoutineParam{{Name: "x", Type: "INT"}}, Returns: "INT", Deterministic: true, DataAccess: "NO SQL", Security: "DEFINER", Comment: "multiplica", Body: "RETURN x * 2"}
			res, err := app.SaveRoutineDef(id, r, false)
			noErrors(t, res, err)
			cur, err := app.LoadRoutineDef(id, db, "FUNCTION", "dobro")
			if err != nil || cur.Body != "RETURN x * 2" || len(cur.Params) != 1 || cur.Comment != "multiplica" || !cur.Deterministic {
				t.Fatalf("load routine %v %+v", err, cur)
			}
			cur.Body = "BEGIN\n  DECLARE y INT;\n  SET y = x * 3;\n  RETURN y;\nEND"
			res, err = app.SaveRoutineDef(id, cur, false)
			noErrors(t, res, err)
			if out := scalar(t, app, id, "SELECT `"+db+"`.dobro(5)"); out != "15" {
				t.Fatalf("replaced function returned %s", out)
			}
			bad := cur
			bad.Body = "RETURN coluna_que_nao_existe +"
			res, err = app.SaveRoutineDef(id, bad, false)
			if err != nil || len(res) == 0 || res[0].Error == nil {
				t.Fatalf("expected the temporary create to fail: %v %+v", err, res)
			}
			if out := scalar(t, app, id, "SELECT `"+db+"`.dobro(5)"); out != "15" {
				t.Fatalf("original lost after failed replace: %s", out)
			}
			// Procedure with OUT parameter; rename keeps working.
			p := database.RoutineDef{Database: db, Name: "conta", Type: "PROCEDURE", Params: []database.RoutineParam{{Mode: "OUT", Name: "n", Type: "INT"}}, DataAccess: "READS SQL DATA", Body: "SELECT COUNT(*) INTO n FROM t"}
			res, err = app.SaveRoutineDef(id, p, false)
			noErrors(t, res, err)
			pc, _ := app.LoadRoutineDef(id, db, "PROCEDURE", "conta")
			pc.Name = "contar"
			res, err = app.SaveRoutineDef(id, pc, false)
			noErrors(t, res, err)
			if _, err := app.LoadRoutineDef(id, db, "PROCEDURE", "conta"); err == nil {
				t.Fatal("old procedure name still exists")
			}

			// Trigger replace.
			tr := database.TriggerDef{Database: db, Name: "tg", Table: "t", Timing: "BEFORE", Event: "INSERT", Body: "SET NEW.v = 1"}
			res, err = app.SaveTriggerDef(id, tr, false)
			noErrors(t, res, err)
			tc, err := app.LoadTriggerDef(id, db, "tg")
			if err != nil || tc.Body != "SET NEW.v = 1" {
				t.Fatalf("trigger %v %+v", err, tc)
			}
			tc.Body = "BEGIN\n  SET NEW.v = NEW.id * 10;\nEND"
			res, err = app.SaveTriggerDef(id, tc, false)
			noErrors(t, res, err)
			mustAdmin(t, app, id, "INSERT INTO `"+db+"`.t (id) VALUES (4)")
			if out := scalar(t, app, id, "SELECT v FROM `"+db+"`.t WHERE id = 4"); out != "40" {
				t.Fatalf("trigger result %s", out)
			}

			// Event: create, alter with rename and schedule change.
			ev := database.EventDef{Database: db, Name: "ev", Recurring: true, IntervalValue: "1", IntervalField: "DAY", Starts: "2030-01-01 00:00:00", Preserve: true, Status: "DISABLE", Comment: "teste", Body: "DELETE FROM t WHERE id < 0"}
			res, err = app.SaveEventDef(id, ev, false)
			noErrors(t, res, err)
			ec, err := app.LoadEventDef(id, db, "ev")
			if err != nil || !ec.Recurring || ec.IntervalField != "DAY" || ec.Status != "DISABLE" || !ec.Preserve {
				t.Fatalf("event %v %+v", err, ec)
			}
			ec.Name = "ev2"
			ec.IntervalValue = "2"
			ec.IntervalField = "HOUR"
			ec.Comment = "alterado"
			res, err = app.SaveEventDef(id, ec, false)
			noErrors(t, res, err)
			e2, err := app.LoadEventDef(id, db, "ev2")
			if err != nil || e2.IntervalValue != "2" || e2.IntervalField != "HOUR" || e2.Comment != "alterado" {
				t.Fatalf("event after alter %v %+v", err, e2)
			}

			// View: create, alter with rename.
			v := database.ViewDef{Database: db, Name: "vt", Algorithm: "MERGE", Security: "INVOKER", CheckOption: "CASCADED", Select: "SELECT id, v FROM `" + db + "`.t WHERE v > 0"}
			res, err = app.SaveViewDef(id, v, false)
			noErrors(t, res, err)
			vc, err := app.LoadViewDef(id, db, "vt")
			if err != nil || vc.Algorithm != "MERGE" || vc.Security != "INVOKER" || vc.CheckOption != "CASCADED" {
				t.Fatalf("view %v %+v", err, vc)
			}
			vc.Name = "vt2"
			vc.CheckOption = ""
			res, err = app.SaveViewDef(id, vc, false)
			noErrors(t, res, err)
			if _, err := app.LoadViewDef(id, db, "vt2"); err != nil {
				t.Fatal(err)
			}
			defs, err := app.Definers(id)
			if err != nil || len(defs) == 0 {
				t.Fatalf("definers %v %v", err, defs)
			}
		})
	}
}

func scalar(t *testing.T, app *api.App, id, sql string) string {
	t.Helper()
	resp, err := app.Execute(query.ExecuteRequest{TabID: "it-scalar-tab-01", ConnectionID: id, SQL: sql, CursorOffset: -1, Mode: "statement"})
	if err != nil {
		t.Fatal(err)
	}
	r := resp.Results[0]
	if r.Error != nil {
		t.Fatalf("%s: %s", sql, r.Error.Message)
	}
	if len(r.Rows) == 0 {
		return ""
	}
	return cell(r.Rows, 0, 0)
}

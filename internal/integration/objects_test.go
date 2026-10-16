package integration

import (
	"context"
	"database/sql/driver"
	"strings"
	"sync"
	"testing"
	"time"

	"anansi/internal/apperr"
	"anansi/internal/connections"
	"anansi/internal/database"
	"anansi/internal/database/mysql"
	"anansi/internal/database/sqllog"
	"anansi/internal/query"
)

// objectsFixture creates a database with a table hierarchy, a view, routines,
// a trigger and an event.
func objectsFixture(t *testing.T, e *env) string {
	t.Helper()
	ctx := context.Background()
	db := uniq("it_obj")
	stmts := []string{
		"CREATE DATABASE `" + db + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci",
		"CREATE TABLE `" + db + "`.clientes (id INT PRIMARY KEY AUTO_INCREMENT, nome VARCHAR(50) NOT NULL, idade INT, CONSTRAINT chk_idade CHECK (idade >= 0)) COMMENT 'cadastro'",
		"CREATE TABLE `" + db + "`.pedidos (id INT PRIMARY KEY, cliente_id INT NOT NULL, loja INT, total DECIMAL(10,2), " +
			"CONSTRAINT fk_cli FOREIGN KEY (cliente_id) REFERENCES `" + db + "`.clientes(id) ON DELETE CASCADE ON UPDATE RESTRICT)",
		"CREATE VIEW `" + db + "`.v_ped AS SELECT id, total FROM `" + db + "`.pedidos",
		"CREATE PROCEDURE `" + db + "`.p_soma(IN a INT, INOUT b VARCHAR(20), OUT c INT) BEGIN SET c = a + 1; END",
		"CREATE FUNCTION `" + db + "`.f_dobro(x INT) RETURNS INT DETERMINISTIC RETURN x * 2",
		"CREATE TRIGGER `" + db + "`.trg_ped BEFORE INSERT ON `" + db + "`.pedidos FOR EACH ROW SET NEW.total = COALESCE(NEW.total, 0)",
		"CREATE EVENT `" + db + "`.ev_limpa ON SCHEDULE EVERY 1 DAY DISABLE COMMENT 'limpeza' DO DELETE FROM `" + db + "`.pedidos WHERE total = 0",
		"INSERT INTO `" + db + "`.clientes (nome, idade) VALUES ('Ana', 30), ('Bia', 40)",
	}
	res, err := e.queries.RunAdmin(ctx, query.AdminRequest{ConnectionID: e.profile.ID, Statements: stmts, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Error != nil {
			t.Fatalf("%s: %s", r.SQL, r.Error.Message)
		}
	}
	if len(res) != len(stmts) {
		t.Fatalf("ran %d of %d statements", len(res), len(stmts))
	}
	t.Cleanup(func() {
		_, _ = e.queries.RunAdmin(context.Background(), query.AdminRequest{ConnectionID: e.profile.ID, Statements: []string{"DROP DATABASE `" + db + "`"}, Confirmed: true})
	})
	return db
}

func TestObjectBrowser(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			e := setup(t, tg)
			e.connect(t)
			db := objectsFixture(t, e)
			ctx := context.Background()
			sess, _ := e.mgr.Get(e.profile.ID)
			b := sess.Driver.Inspector(sess.DB).(database.ObjectBrowser)

			sums, err := b.DatabaseSummaries(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var mine *database.DatabaseSummary
			for i := range sums {
				if sums[i].Name == db {
					mine = &sums[i]
				}
			}
			if mine == nil || mine.Tables != 2 || mine.Views != 1 || mine.Collation != "utf8mb4_unicode_ci" || mine.Size <= 0 {
				t.Fatalf("summary: %+v", mine)
			}

			st, err := b.TableStatus(ctx, db)
			if err != nil {
				t.Fatal(err)
			}
			byName := map[string]database.TableStatus{}
			for _, s := range st {
				byName[s.Name] = s
			}
			cli := byName["clientes"]
			if cli.Type != "table" || cli.Engine != "InnoDB" || cli.Comment != "cadastro" || cli.AutoIncrement != "3" || cli.DataLength == nil {
				t.Fatalf("clientes status: %+v", cli)
			}
			if byName["v_ped"].Type != "view" || byName["v_ped"].Comment != "" {
				t.Fatalf("view status: %+v", byName["v_ped"])
			}

			fks, err := b.ForeignKeys(ctx, db, "pedidos")
			if err != nil || len(fks) != 1 {
				t.Fatalf("fks %v %+v", err, fks)
			}
			fk := fks[0]
			if fk.Name != "fk_cli" || fk.RefTable != "clientes" || fk.Columns[0] != "cliente_id" || fk.RefColumns[0] != "id" || fk.OnDelete != "CASCADE" || fk.OnUpdate != "RESTRICT" {
				t.Fatalf("fk %+v", fk)
			}
			refs, err := b.ReferencingKeys(ctx, db, "clientes")
			if err != nil || len(refs) != 1 || refs[0].Table != "pedidos" {
				t.Fatalf("referencing %v %+v", err, refs)
			}

			checks, err := b.CheckConstraints(ctx, db, "clientes")
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, c := range checks {
				if c.Name == "chk_idade" && strings.Contains(c.Clause, "idade") {
					found = true
				}
			}
			if !found {
				t.Fatalf("checks %+v", checks)
			}

			trg, err := b.Triggers(ctx, db)
			if err != nil || len(trg) != 1 || trg[0].Table != "pedidos" || trg[0].Timing != "BEFORE" || trg[0].Event != "INSERT" || !strings.Contains(trg[0].Statement, "COALESCE") {
				t.Fatalf("triggers %v %+v", err, trg)
			}

			evs, err := b.Events(ctx, db)
			if err != nil || len(evs) != 1 || evs[0].Type != "RECURRING" || evs[0].IntervalField != "DAY" || evs[0].Status != "DISABLED" || evs[0].Comment != "limpeza" {
				t.Fatalf("events %v %+v", err, evs)
			}

			rts, err := b.RoutineDetails(ctx, db)
			if err != nil || len(rts) != 2 {
				t.Fatalf("routines %v %+v", err, rts)
			}
			for _, r := range rts {
				switch r.Name {
				case "f_dobro":
					if r.Type != "FUNCTION" || !strings.HasPrefix(strings.ToLower(r.Returns), "int") || !r.Deterministic || len(r.Params) != 1 || r.Params[0].Name != "x" {
						t.Fatalf("function %+v", r)
					}
				case "p_soma":
					if r.Type != "PROCEDURE" || len(r.Params) != 3 || r.Params[1].Mode != "INOUT" || !strings.HasPrefix(r.Params[1].Type, "varchar(20)") || !strings.Contains(r.Body, "SET c") {
						t.Fatalf("procedure %+v", r)
					}
				}
			}

			for kind, name := range map[database.ObjectKind]string{
				database.ObjTable: "pedidos", database.ObjView: "v_ped", database.ObjProcedure: "p_soma",
				database.ObjFunction: "f_dobro", database.ObjTrigger: "trg_ped", database.ObjEvent: "ev_limpa", database.ObjDatabase: db,
			} {
				ddl, err := b.CreateStatement(ctx, db, kind, name)
				if err != nil || !strings.HasPrefix(ddl, "CREATE") || !strings.Contains(ddl, name) {
					t.Fatalf("show create %s %s: %v %q", kind, name, err, ddl)
				}
			}
			if _, err := b.CreateStatement(ctx, db, database.ObjTable, "nao_existe"); err == nil {
				t.Fatal("expected error for missing table")
			}
		})
	}
}

func TestServerAdmin(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			e := setup(t, tg)
			e.connect(t)
			ctx := context.Background()
			sess, _ := e.mgr.Get(e.profile.ID)
			a := sess.Driver.Inspector(sess.DB).(database.ServerAdmin)

			vars, err := a.Variables(ctx)
			if err != nil || len(vars) < 100 {
				t.Fatalf("variables %v %d", err, len(vars))
			}
			sawBoth := false
			for _, v := range vars {
				if v.Name == "max_connections" && (v.Global == nil || *v.Global == "") {
					t.Fatalf("max_connections %+v", v)
				}
				if v.Name == "sql_mode" && v.Session != nil && v.Global != nil {
					sawBoth = true
				}
			}
			if !sawBoth {
				t.Fatal("sql_mode should have session and global values")
			}

			status, err := a.Status(ctx)
			if err != nil || len(status) < 50 {
				t.Fatalf("status %v %d", err, len(status))
			}
			uptime := ""
			for _, s := range status {
				if s.Name == "Uptime" {
					uptime = s.Value
				}
			}
			if uptime == "" {
				t.Fatal("Uptime missing")
			}

			// A sleeping statement on a tab connection must show up and be
			// killable with KILL QUERY.
			done := make(chan *query.ExecuteResponse, 1)
			go func() {
				resp, _ := e.queries.Execute(context.Background(), query.ExecuteRequest{TabID: "tab-kill-0001", ConnectionID: e.profile.ID, SQL: "SELECT SLEEP(30) AS dormindo", CursorOffset: -1, Mode: "statement"})
				done <- resp
			}()
			var target int64
			for i := 0; i < 50 && target == 0; i++ {
				time.Sleep(100 * time.Millisecond)
				procs, err := a.Processes(ctx)
				if err != nil {
					t.Fatal(err)
				}
				for _, p := range procs {
					if strings.Contains(p.Info, "SLEEP(30)") {
						target = p.ID
					}
				}
			}
			if target == 0 {
				t.Fatal("sleeping statement not found in the process list")
			}
			g := sess.Driver.(database.ObjectSQL)
			kill, _ := g.KillSQL(target, true)
			start := time.Now()
			res, err := e.queries.RunAdmin(ctx, query.AdminRequest{ConnectionID: e.profile.ID, Statements: []string{kill}, Confirmed: true})
			if err != nil || res[0].Error != nil {
				t.Fatalf("kill: %v %+v", err, res)
			}
			select {
			case <-done:
				if time.Since(start) > 5*time.Second {
					t.Fatal("kill took too long")
				}
			case <-time.After(10 * time.Second):
				t.Fatal("statement was not interrupted")
			}

			// SET GLOBAL through the generator (and back).
			var orig string
			if err := sess.DB.QueryRow("SELECT @@GLOBAL.max_connections").Scan(&orig); err != nil {
				t.Fatal(err)
			}
			set, _ := g.SetVariableSQL(true, "max_connections", "321", "auto")
			if _, err := e.queries.RunAdmin(ctx, query.AdminRequest{ConnectionID: e.profile.ID, Statements: []string{set}, Confirmed: true}); err != nil {
				t.Fatal(err)
			}
			var now string
			_ = sess.DB.QueryRow("SELECT @@GLOBAL.max_connections").Scan(&now)
			back, _ := g.SetVariableSQL(true, "max_connections", orig, "auto")
			_, _ = e.queries.RunAdmin(ctx, query.AdminRequest{ConnectionID: e.profile.ID, Statements: []string{back}, Confirmed: true})
			if now != "321" {
				t.Fatalf("max_connections = %s", now)
			}
		})
	}
}

func TestObjectActionsAndPolicies(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			e := setup(t, tg)
			e.connect(t)
			db := objectsFixture(t, e)
			ctx := context.Background()
			sess, _ := e.mgr.Get(e.profile.ID)
			g := sess.Driver.(database.ObjectSQL)
			run := func(stmts ...string) []query.AdminResult {
				t.Helper()
				res, err := e.queries.RunAdmin(ctx, query.AdminRequest{ConnectionID: e.profile.ID, Statements: stmts, Confirmed: true, ContinueOnError: true})
				if err != nil {
					t.Fatal(err)
				}
				return res
			}

			ren, _ := g.RenameSQL(database.ObjTable, db, "pedidos", "", "encomendas")
			if r := run(ren); r[0].Error != nil {
				t.Fatal(r[0].Error.Message)
			}
			evRen, _ := g.RenameSQL(database.ObjEvent, db, "ev_limpa", "", "ev_limpeza")
			if r := run(evRen); r[0].Error != nil {
				t.Fatal(r[0].Error.Message)
			}
			if _, err := g.RenameSQL(database.ObjProcedure, db, "p_soma", "", "x"); err == nil {
				t.Fatal("renaming procedures should be refused")
			}
			trunc, _ := g.TruncateSQL(db, "encomendas")
			if r := run(trunc); r[0].Error != nil {
				t.Fatal(r[0].Error.Message)
			}
			alt, _ := g.AlterDatabaseSQL(db, "utf8mb4", "utf8mb4_bin")
			if r := run(alt); r[0].Error != nil {
				t.Fatal(r[0].Error.Message)
			}
			// Dropping several objects reports each failure and keeps going.
			var drops []string
			for _, o := range []struct {
				k database.ObjectKind
				n string
			}{{database.ObjView, "v_ped"}, {database.ObjTable, "nao_existe"}, {database.ObjProcedure, "p_soma"}, {database.ObjFunction, "f_dobro"}, {database.ObjTrigger, "trg_ped"}, {database.ObjEvent, "ev_limpeza"}} {
				s, err := g.DropSQL(o.k, db, o.n)
				if err != nil {
					t.Fatal(err)
				}
				drops = append(drops, s)
			}
			res := run(drops...)
			if len(res) != 6 || res[1].Error == nil || res[0].Error != nil || res[5].Error != nil {
				t.Fatalf("drops %+v", res)
			}

			// Without confirmation the destructive statement is refused.
			d, _ := g.DropSQL(database.ObjTable, db, "encomendas")
			_, err := e.queries.RunAdmin(ctx, query.AdminRequest{ConnectionID: e.profile.ID, Statements: []string{d}})
			if ae := appErr(err); ae == nil || ae.Code != apperr.CodeConfirmationRequired {
				t.Fatalf("expected confirmation_required, got %v", err)
			}
			// Read-only profiles refuse every change.
			ro := setup(t, tg, func(p *connections.Profile) { p.ReadOnly = true })
			ro.connect(t)
			_, err = ro.queries.RunAdmin(ctx, query.AdminRequest{ConnectionID: ro.profile.ID, Statements: []string{d}, Confirmed: true})
			if ae := appErr(err); ae == nil || ae.Code != apperr.CodeReadOnly {
				t.Fatalf("expected read_only, got %v", err)
			}
			kill, _ := g.KillSQL(1, true)
			_, err = ro.queries.RunAdmin(ctx, query.AdminRequest{ConnectionID: ro.profile.ID, Statements: []string{kill}, Confirmed: true})
			if ae := appErr(err); ae == nil || ae.Code != apperr.CodeReadOnly {
				t.Fatalf("KILL on read-only: %v", err)
			}
		})
	}
}

func TestSQLLogSeesEveryStatement(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			var mu sync.Mutex
			var logged []string
			e := setup(t, tg)
			// Replace the manager with one that logs.
			reg := database.NewRegistry(mysql.New())
			e.mgr = connections.NewManager(connections.RegistryOpener(reg, func(id string) sqllog.Func {
				return func(q string, args []driver.NamedValue, d time.Duration, err error) {
					mu.Lock()
					logged = append(logged, q)
					mu.Unlock()
				}
			}))
			e.svc.Manager = e.mgr
			e.queries = query.NewService(e.mgr, func() query.Options { return e.opts }, query.Hooks{})
			e.connect(t)
			defer func() { _ = e.mgr.CloseAll(context.Background()) }()
			e.exec(t, "tab-log-0001", "SELECT 41 + 1 AS resposta", "statement")
			sess, _ := e.mgr.Get(e.profile.ID)
			if _, err := sess.Driver.Inspector(sess.DB).Tables(context.Background(), "studio_it"); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			defer mu.Unlock()
			joined := strings.Join(logged, "\n")
			for _, want := range []string{"SELECT VERSION()", "SELECT 41 + 1 AS resposta", "information_schema.TABLES", "CONNECTION_ID()"} {
				if !strings.Contains(joined, want) {
					t.Fatalf("log misses %q:\n%s", want, joined)
				}
			}
		})
	}
}

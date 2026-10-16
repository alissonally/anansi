package integration

import (
	"strings"
	"testing"
	"time"

	"dbstudio/internal/api"
	"dbstudio/internal/database/mysql"
	"dbstudio/internal/tasks"
)

func waitTask(t *testing.T, app *api.App, id string) tasks.Task {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		for _, tk := range app.ListTasks() {
			if tk.ID == id && tk.Status != tasks.Running {
				if tk.Status != tasks.Done {
					t.Fatalf("task %s: %s %s", tk.Title, tk.Status, tk.Error)
				}
				return tk
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("task timeout")
	return tasks.Task{}
}

func TestTableTools(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			app, id := newApp(t, tg)
			db := uniq("it_tools")
			db2 := db + "_b"
			mustAdmin(t, app, id,
				"CREATE DATABASE `"+db+"`", "CREATE DATABASE `"+db2+"`",
				"CREATE TABLE `"+db+"`.pai (id INT PRIMARY KEY AUTO_INCREMENT, nome VARCHAR(40) NOT NULL, UNIQUE KEY uk_nome (nome))",
				"CREATE TABLE `"+db+"`.filho (id INT PRIMARY KEY AUTO_INCREMENT, pai INT, obs TEXT, valor DECIMAL(8,2), quando DATETIME, cor ENUM('azul','verde'), dobro DECIMAL(10,2) AS (valor*2) STORED, "+
					"KEY ix_pai (pai), CONSTRAINT fk_pai FOREIGN KEY (pai) REFERENCES pai(id), CONSTRAINT ck_valor CHECK (valor >= 0))",
				"INSERT INTO `"+db+"`.pai (nome) VALUES ('Ana'), ('Bia 100%'), ('Carlos_x')",
				"INSERT INTO `"+db+"`.filho (pai, obs, valor, quando, cor) VALUES (1, 'contém agulha aqui', 1.5, '2024-01-01', 'azul'), (2, 'nada', 42, NULL, 'verde'), (3, 'AGULHA maiúscula', 0, NULL, NULL)",
			)
			t.Cleanup(func() { mustAdmin(t, app, id, "DROP DATABASE IF EXISTS `"+db2+"`", "DROP DATABASE IF EXISTS `"+db+"`") })
			pai := api.TableRef{Database: db, Table: "pai"}
			filho := api.TableRef{Database: db, Table: "filho"}

			// Maintenance.
			for _, op := range []struct {
				op   string
				opts []string
			}{{"CHECK", []string{"QUICK"}}, {"ANALYZE", nil}, {"CHECKSUM", []string{"EXTENDED"}}, {"OPTIMIZE", nil}} {
				tid, err := app.StartMaintenance(id, op.op, op.opts, []api.TableRef{pai, filho}, true)
				if err != nil {
					t.Fatal(err)
				}
				rows, _ := waitTask(t, app, tid).Data.([]api.MaintenanceRow)
				if len(rows) < 2 {
					t.Fatalf("%s: %+v", op.op, rows)
				}
				for _, r := range rows {
					if strings.EqualFold(r.MsgType, "error") {
						t.Fatalf("%s: %+v", op.op, r)
					}
				}
			}
			if _, err := app.StartMaintenance(id, "CHECKSUM", []string{"QUICK", "EXTENDED"}, []api.TableRef{pai}, true); err == nil {
				t.Fatal("QUICK+EXTENDED accepted")
			}

			// Text search.
			search := func(spec mysql.SearchSpec) map[string]api.SearchHit {
				t.Helper()
				tid, err := app.StartSearch(id, []string{db}, nil, spec)
				if err != nil {
					t.Fatal(err)
				}
				hits, _ := waitTask(t, app, tid).Data.([]api.SearchHit)
				out := map[string]api.SearchHit{}
				for _, h := range hits {
					out[h.Table] = h
				}
				return out
			}
			if h := search(mysql.SearchSpec{Text: "agulha"}); h["filho"].Rows != 2 || len(h) != 1 {
				t.Fatalf("contains: %+v", h)
			} else if n := scalar(t, app, id, "SELECT COUNT(*) FROM `"+db+"`.filho WHERE "+h["filho"].Filter); n != "2" {
				t.Fatalf("filter %q gives %s", h["filho"].Filter, n)
			}
			if h := search(mysql.SearchSpec{Text: "agulha", CaseSensitive: true}); h["filho"].Rows != 1 {
				t.Fatalf("case: %+v", h)
			}
			if h := search(mysql.SearchSpec{Text: "100%"}); h["pai"].Rows != 1 {
				t.Fatalf("percent literal: %+v", h)
			}
			if h := search(mysql.SearchSpec{Text: "s_x"}); h["pai"].Rows != 1 {
				t.Fatalf("underscore literal: %+v", h)
			}
			if h := search(mysql.SearchSpec{Text: "a_x"}); len(h) != 0 {
				t.Fatalf("underscore must not be a wildcard: %+v", h)
			}
			if h := search(mysql.SearchSpec{Text: "42", Types: "numeric"}); h["filho"].Rows != 1 || len(h) != 1 {
				t.Fatalf("numeric: %+v", h)
			}
			if h := search(mysql.SearchSpec{Text: "^(ana|bia)", Match: "regex"}); h["pai"].Rows != 2 {
				t.Fatalf("regex: %+v", h)
			}
			if h := search(mysql.SearchSpec{Text: "ana", Match: "exact"}); h["pai"].Rows != 1 {
				t.Fatalf("exact: %+v", h)
			}

			// Copy table: structure, indexes, FKs, checks and filtered data.
			res, err := app.CopyTable(id, api.CopyTableSpec{Database: db, Table: "filho", TargetDatabase: db, TargetTable: "filho_copia",
				Indexes: true, ForeignKeys: true, Checks: true, Data: true, Filter: "valor > 1"}, true)
			noErrors(t, res, err)
			if n := scalar(t, app, id, "SELECT COUNT(*) FROM `"+db+"`.filho_copia"); n != "2" {
				t.Fatalf("copied rows %s", n)
			}
			if n := scalar(t, app, id, "SELECT dobro FROM `"+db+"`.filho_copia WHERE id = 2"); n != "84.00" {
				t.Fatalf("generated column %s", n)
			}
			if n := scalar(t, app, id, "SELECT COUNT(*) FROM information_schema.REFERENTIAL_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = '"+db+"' AND TABLE_NAME = 'filho_copia'"); n != "1" {
				t.Fatalf("fks %s", n)
			}
			// Into another database, only some columns, no data; FK points back.
			res, err = app.CopyTable(id, api.CopyTableSpec{Database: db, Table: "filho", TargetDatabase: db2, TargetTable: "filho",
				Columns: []string{"id", "pai", "obs"}, Indexes: true, ForeignKeys: true}, true)
			noErrors(t, res, err)
			if n := scalar(t, app, id, "SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = '"+db2+"' AND TABLE_NAME = 'filho'"); n != "3" {
				t.Fatalf("columns %s", n)
			}
			if n := scalar(t, app, id, "SELECT REFERENCED_TABLE_SCHEMA FROM information_schema.KEY_COLUMN_USAGE WHERE TABLE_SCHEMA = '"+db2+"' AND REFERENCED_TABLE_NAME IS NOT NULL"); n != db {
				t.Fatalf("fk target %s", n)
			}
			if _, err := app.CopyTable(id, api.CopyTableSpec{Database: db, Table: "filho", TargetTable: "filho"}, true); err == nil {
				t.Fatal("copy over itself accepted")
			}
			res, err = app.CopyTable(id, api.CopyTableSpec{Database: db, Table: "pai", TargetDatabase: db2, TargetTable: "filho", DropExisting: true, Data: true}, true)
			noErrors(t, res, err)
			if n := scalar(t, app, id, "SELECT COUNT(*) FROM `"+db2+"`.filho"); n != "3" {
				t.Fatalf("drop existing %s", n)
			}

			// Generate data.
			tid, err := app.StartGenerateData(id, api.GenerateSpec{Database: db, Table: "filho", Rows: 1200, NullPercent: 20}, true)
			if err != nil {
				t.Fatal(err)
			}
			waitTask(t, app, tid)
			if n := scalar(t, app, id, "SELECT COUNT(*) FROM `"+db+"`.filho"); n != "1203" {
				t.Fatalf("generated rows %s", n)
			}
			if n := scalar(t, app, id, "SELECT COUNT(*) FROM `"+db+"`.filho WHERE obs IS NULL"); n == "0" {
				t.Fatal("no NULLs generated")
			}

			// Bulk editor: convert, reset auto increment, move.
			stmts, err := app.PreviewBulkEdit(id, []api.TableRef{pai, {Database: db, Table: "filho_copia"}}, mysql.BulkSpec{ConvertCharset: "latin1", MoveTo: db2})
			if err != nil || len(stmts) != 4 {
				t.Fatalf("%v %v", stmts, err)
			}
			res, err = app.ApplyBulkEdit(id, []api.TableRef{{Database: db, Table: "filho_copia"}}, mysql.BulkSpec{ConvertCharset: "latin1", ConvertCollation: "latin1_swedish_ci", MoveTo: db2}, true)
			noErrors(t, res, err)
			if n := scalar(t, app, id, "SELECT TABLE_COLLATION FROM information_schema.TABLES WHERE TABLE_SCHEMA = '"+db2+"' AND TABLE_NAME = 'filho_copia'"); n != "latin1_swedish_ci" {
				t.Fatalf("collation %s", n)
			}

			if err := app.Flush(id, "TABLES", true); err != nil {
				t.Fatal(err)
			}
			if err := app.Flush(id, "TABLES; DROP", true); err == nil {
				t.Fatal("bad flush accepted")
			}
		})
	}
}

func TestUserManager(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			app, id := newApp(t, tg)
			db := uniq("it_users")
			user := uniq("u")
			role := uniq("r")
			mustAdmin(t, app, id, "CREATE DATABASE `"+db+"`", "CREATE TABLE `"+db+"`.t (a INT, b INT)",
				"CREATE PROCEDURE `"+db+"`.p() SELECT 1")
			t.Cleanup(func() {
				mustAdmin(t, app, id, "DROP DATABASE IF EXISTS `"+db+"`", "DROP USER IF EXISTS '"+user+"'@'%'", "DROP USER IF EXISTS '"+user+"2'@'localhost'")
				if tg.name == "mariadb" {
					mustAdmin(t, app, id, "DROP ROLE IF EXISTS "+role)
				} else {
					mustAdmin(t, app, id, "DROP ROLE IF EXISTS '"+role+"'@'%'")
				}
			})
			cat, err := app.PrivilegeCatalog(id)
			if err != nil || len(cat.Global) == 0 || len(cat.Table) == 0 || !cat.Roles {
				t.Fatalf("%+v %v", cat, err)
			}

			// Role.
			r := mysql.UserDef{User: role, Host: "%", IsRole: true, Grants: []mysql.Grant{{Level: "database", Database: db, Privileges: []string{"SELECT"}}}}
			res, err := app.SaveAccount(id, r, true)
			noErrors(t, res, err)

			u := mysql.UserDef{User: user, Host: "%", Password: "S3nha!forte", MaxQueries: 100, Require: "SSL",
				PasswordExpire: "INTERVAL", ExpireDays: 30,
				Grants: []mysql.Grant{
					{Level: "global", Privileges: []string{"PROCESS"}},
					{Level: "database", Database: db, Privileges: []string{"SELECT", "INSERT"}, GrantOption: true},
					{Level: "table", Database: db, Name: "t", Privileges: []string{"DELETE"}, Columns: map[string][]string{"UPDATE": {"a"}}},
					{Level: "routine", Database: db, Name: "p", RoutineType: "PROCEDURE", Privileges: []string{"EXECUTE"}},
				},
				Roles: []mysql.RoleGrant{{Role: role}},
			}
			if tg.name == "mariadb" {
				u.DefaultRoles = []string{role}
			} else {
				u.DefaultRoles = []string{role + "@%"}
			}
			prev, err := app.PreviewAccount(id, u)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(strings.Join(prev, "\n"), "S3nha") {
				t.Fatalf("password in preview: %v", prev)
			}
			res, err = app.SaveAccount(id, u, true)
			noErrors(t, res, err)

			accts, err := app.ListAccounts(id)
			if err != nil {
				t.Fatal(err)
			}
			found := map[string]mysql.Account{}
			for _, a := range accts {
				found[a.User] = a
			}
			if !found[role].IsRole || found[user].IsRole || found[user].User == "" {
				t.Fatalf("accounts: %+v %+v", found[role], found[user])
			}

			got, err := app.LoadAccount(id, user, "%", false)
			if err != nil {
				t.Fatal(err)
			}
			if got.MaxQueries != 100 || got.Require != "SSL" || got.PasswordExpire != "INTERVAL" || got.ExpireDays != 30 || len(got.Grants) != 4 || len(got.Roles) != 1 || len(got.DefaultRoles) != 1 {
				t.Fatalf("loaded: %+v", got)
			}
			// Round trip: saving what was loaded changes nothing.
			if stmts, err := app.PreviewAccount(id, got); err != nil || len(stmts) != 0 {
				t.Fatalf("round trip: %v %v", stmts, err)
			}

			// Change: rename + host, drop a grant, revoke grant option, lock.
			got.User, got.Host = user+"2", "localhost"
			got.Locked = true
			got.Require = "NONE"
			got.Grants = got.Grants[:0]
			got.Grants = append(got.Grants, mysql.Grant{Level: "database", Database: db, Privileges: []string{"SELECT"}})
			res, err = app.SaveAccount(id, got, true)
			noErrors(t, res, err)
			again, err := app.LoadAccount(id, user+"2", "localhost", false)
			if err != nil {
				t.Fatal(err)
			}
			if !again.Locked || again.Require != "NONE" || len(again.Grants) != 1 || len(again.Grants[0].Privileges) != 1 || again.Grants[0].GrantOption {
				t.Fatalf("after change: %+v", again)
			}
			if stmts, err := app.PreviewAccount(id, again); err != nil || len(stmts) != 0 {
				t.Fatalf("round trip 2: %v %v", stmts, err)
			}

			if err := app.DropAccount(id, user+"2", "localhost", false, true); err != nil {
				t.Fatal(err)
			}
			if err := app.DropAccount(id, role, "%", true, true); err != nil {
				t.Fatal(err)
			}
			if _, err := app.LoadAccount(id, user+"2", "localhost", false); err == nil {
				t.Fatal("dropped user still loads")
			}
		})
	}
}

func TestCompareSchemas(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			app, id := newApp(t, tg)
			src := uniq("it_cmp_a")
			dst := src + "_b"
			mustAdmin(t, app, id, "CREATE DATABASE `"+src+"`", "CREATE DATABASE `"+dst+"`",
				"CREATE TABLE `"+src+"`.cli (id INT PRIMARY KEY AUTO_INCREMENT, nome VARCHAR(60) NOT NULL, email VARCHAR(120), KEY ix_email (email))",
				"CREATE TABLE `"+src+"`.ped (id INT PRIMARY KEY, cli INT, total DECIMAL(10,2) DEFAULT 0, CONSTRAINT fk_cli FOREIGN KEY (cli) REFERENCES cli(id), CONSTRAINT ck_total CHECK (total >= 0))",
				"CREATE VIEW `"+src+"`.v_cli AS SELECT id, nome FROM `"+src+"`.cli",
				"CREATE FUNCTION `"+src+"`.f2(x INT) RETURNS INT DETERMINISTIC RETURN x * 2",
				"CREATE PROCEDURE `"+src+"`.p1() SELECT COUNT(*) FROM cli",
				"CREATE TRIGGER `"+src+"`.tg BEFORE INSERT ON `"+src+"`.ped FOR EACH ROW SET NEW.total = COALESCE(NEW.total, 0)",
				"CREATE EVENT `"+src+"`.ev ON SCHEDULE EVERY 1 DAY DISABLE DO DELETE FROM ped WHERE id < 0",
				// Target: cli differs (missing column, other type, extra index), an extra table, a different function.
				"CREATE TABLE `"+dst+"`.cli (id INT PRIMARY KEY AUTO_INCREMENT, nome VARCHAR(30), KEY ix_nome (nome))",
				"CREATE TABLE `"+dst+"`.sobra (x INT)",
				"CREATE FUNCTION `"+dst+"`.f2(x INT) RETURNS INT DETERMINISTIC RETURN x * 3",
				"INSERT INTO `"+dst+"`.cli (nome) VALUES ('mantido')",
			)
			t.Cleanup(func() {
				mustAdmin(t, app, id, "DROP DATABASE IF EXISTS `"+dst+"`", "DROP DATABASE IF EXISTS `"+src+"`")
			})

			diff, err := app.CompareSchemas(id, src, id, dst)
			if err != nil {
				t.Fatal(err)
			}
			if len(diff.Warnings) > 0 {
				t.Fatalf("warnings: %v", diff.Warnings)
			}
			status := map[string]string{}
			var all []string
			for _, it := range diff.Items {
				status[it.Kind+" "+it.Name] = it.Status
				all = append(all, it.Statements...)
			}
			want := map[string]string{"table cli": "alter", "table ped": "create", "table sobra": "drop", "view v_cli": "create",
				"function f2": "alter", "procedure p1": "create", "trigger tg": "create", "event ev": "create"}
			for k, v := range want {
				if status[k] != v {
					t.Fatalf("%s: %q (want %q); all: %v", k, status[k], v, status)
				}
			}
			for _, s := range all {
				if strings.Contains(s, "`"+src+"`") {
					t.Fatalf("statement points at the source: %s", s)
				}
			}
			res, err := app.ApplySchemaDiff(id, dst, all, false, true)
			noErrors(t, res, err)
			if n := scalar(t, app, id, "SELECT nome FROM `"+dst+"`.cli"); n != "mantido" {
				t.Fatalf("data lost: %s", n)
			}
			again, err := app.CompareSchemas(id, src, id, dst)
			if err != nil {
				t.Fatal(err)
			}
			for _, it := range again.Items {
				if it.Status != "same" {
					t.Fatalf("after apply %s %s: %s %v\n%s\n%s", it.Kind, it.Name, it.Status, it.Statements, it.SourceSQL, it.TargetSQL)
				}
			}
			if n := scalar(t, app, id, "SELECT `"+dst+"`.f2(5)"); n != "10" {
				t.Fatalf("function %s", n)
			}
		})
	}
}

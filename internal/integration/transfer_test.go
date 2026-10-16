package integration

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"anansi/internal/connections"
	"anansi/internal/csvimport"
	"anansi/internal/database"
	"anansi/internal/dump"
	"anansi/internal/query"
	"anansi/internal/sqlfile"
	"anansi/internal/sqllex"
)

type quietProgress struct{ warns []string }

func (*quietProgress) Set(int64, int64)  {}
func (*quietProgress) Add(int64)         {}
func (*quietProgress) Message(string)    {}
func (q *quietProgress) Warn(msg string) { q.warns = append(q.warns, msg) }

func checksum(t *testing.T, conn *sql.Conn, db, table string) string {
	t.Helper()
	var name string
	var sum sql.NullInt64
	if err := conn.QueryRowContext(context.Background(), "CHECKSUM TABLE `"+db+"`.`"+table+"`").Scan(&name, &sum); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprint(sum.Int64)
}

func TestDumpRestoreAndCopy(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			e := setup(t, tg)
			e.connect(t)
			ctx := context.Background()
			src := uniq("it_dsrc")
			run := func(stmts ...string) {
				t.Helper()
				res, err := e.queries.RunAdmin(ctx, query.AdminRequest{ConnectionID: e.profile.ID, Statements: stmts, Confirmed: true})
				if err != nil {
					t.Fatal(err)
				}
				for _, r := range res {
					if r.Error != nil {
						t.Fatalf("%s: %s", r.SQL, r.Error.Message)
					}
				}
			}
			run("CREATE DATABASE `"+src+"` CHARACTER SET utf8mb4",
				"CREATE TABLE `"+src+"`.pais (id INT PRIMARY KEY AUTO_INCREMENT, nome VARCHAR(40))",
				"CREATE TABLE `"+src+"`.filho (id INT PRIMARY KEY, pai INT, txt MEDIUMTEXT, bin BLOB, flag BIT(4), dec_ DECIMAL(12,4), quando DATETIME(6), ts TIMESTAMP NULL, doc JSON, g INT AS (id * 2) STORED, "+
					"FOREIGN KEY (pai) REFERENCES pais(id))",
				"INSERT INTO `"+src+"`.pais (nome) VALUES ('A'), ('O''Brien \\\\ barra'), ('emoji 🎉 e ação')",
				"INSERT INTO `"+src+"`.filho (id, pai, txt, bin, flag, dec_, quando, ts, doc) VALUES "+
					"(1, 1, 'linha1\\nlinha2\\ttab', 0x00FF10, b'1010', 12345678.1234, '2024-01-02 03:04:05.123456', '2024-06-01 12:00:00', '{\"a\": [1, 2]}'), "+
					"(2, 2, NULL, NULL, NULL, NULL, NULL, NULL, NULL), (3, 3, REPEAT('x', 70000), REPEAT(0xAB, 3000), b'1', -0.5, '1999-12-31 23:59:59', NULL, 'null')",
			)
			// v_a (dumped first, alphabetically) depends on v_z: the dump
			// must not rely on the order of views.
			run("CREATE VIEW `"+src+"`.v_z AS SELECT id, nome FROM `"+src+"`.pais",
				"CREATE VIEW `"+src+"`.v_a AS SELECT * FROM `"+src+"`.v_z WHERE id > 1",
				"CREATE PROCEDURE `"+src+"`.p_conta(OUT n INT) BEGIN SELECT COUNT(*) INTO n FROM filho; END",
				"CREATE FUNCTION `"+src+"`.f_dobro(x INT) RETURNS INT DETERMINISTIC RETURN x * 2",
				"CREATE TRIGGER `"+src+"`.tg_filho BEFORE INSERT ON `"+src+"`.filho FOR EACH ROW SET NEW.txt = COALESCE(NEW.txt, 'trigger')",
				"CREATE EVENT `"+src+"`.ev ON SCHEDULE EVERY 1 DAY DISABLE DO DELETE FROM filho WHERE id < 0",
			)
			t.Cleanup(func() { run("DROP DATABASE IF EXISTS `" + src + "`") })
			sess, _ := e.mgr.Get(e.profile.ID)
			browser := sess.Driver.Inspector(sess.DB).(database.ObjectBrowser)

			opts := dump.Options{Selections: []dump.Selection{{Database: src, AllObjects: true}}, CreateDatabase: true, DropObjects: true, CreateObjects: true,
				Data: dump.DataInsert, MaxInsertKB: 64, Comments: true, WrapTransactions: true}

			// 1. Dump to SQL text.
			var buf bytes.Buffer
			err := e.queries.WithConn(ctx, e.profile.ID, func(conn *sql.Conn, _ *connections.Session) error {
				d := &dump.Dumper{Conn: conn, Browser: browser, Opts: opts, Sink: dump.NewWriterSink(&buf), Progress: &quietProgress{}}
				return d.Run(ctx)
			})
			if err != nil {
				t.Fatal(err)
			}
			text := buf.String()
			for _, want := range []string{"FOREIGN_KEY_CHECKS=0 */", "DELIMITER ;;", "CREATE TABLE IF NOT EXISTS `v_a`", "_binary 0x00FF10", "'linha1\\nlinha2\ttab'"} {
				if !strings.Contains(text, want) {
					t.Fatalf("dump misses %q", want)
				}
			}
			if strings.Contains(text, "`g`") && strings.Contains(text, "INSERT INTO `filho` (`id`, `pai`, `txt`, `bin`, `flag`, `dec_`, `quando`, `ts`, `doc`, `g`)") {
				t.Fatal("generated column should not be inserted")
			}

			// 2. Restore it under another name with the streaming runner.
			dst := uniq("it_ddst")
			restored := strings.ReplaceAll(text, "`"+src+"`", "`"+dst+"`")
			t.Cleanup(func() { run("DROP DATABASE IF EXISTS `" + dst + "`") })
			err = e.queries.WithConn(ctx, e.profile.ID, func(conn *sql.Conn, _ *connections.Session) error {
				_, err := sqlfile.Run(ctx, strings.NewReader(restored), int64(len(restored)), func(ctx context.Context, st sqllex.Statement) error {
					rows, err := conn.QueryContext(ctx, st.Text)
					if err != nil {
						return err
					}
					for rows.Next() {
					}
					rows.Close()
					return rows.Err()
				}, &quietProgress{}, true)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}

			// 3. Copy directly into a third database.
			cp := uniq("it_dcp")
			t.Cleanup(func() { run("DROP DATABASE IF EXISTS `" + cp + "`") })
			copyOpts := opts
			copyOpts.TargetDatabase = cp
			err = e.queries.WithConn(ctx, e.profile.ID, func(conn *sql.Conn, _ *connections.Session) error {
				target, err := sess.DB.Conn(ctx)
				if err != nil {
					return err
				}
				defer target.Close()
				d := &dump.Dumper{Conn: conn, Browser: browser, Opts: copyOpts, Sink: &dump.ExecSink{Ctx: ctx, Conn: target}, Progress: &quietProgress{}}
				return d.Run(ctx)
			})
			if err != nil {
				t.Fatal(err)
			}

			conn, _ := sess.DB.Conn(ctx)
			defer conn.Close()
			for _, db := range []string{dst, cp} {
				for _, tbl := range []string{"pais", "filho"} {
					if a, b := checksum(t, conn, src, tbl), checksum(t, conn, db, tbl); a != b {
						t.Fatalf("%s.%s checksum %s != %s", db, tbl, b, a)
					}
				}
				var n int
				if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+db+"`.v_a").Scan(&n); err != nil || n != 2 {
					t.Fatalf("%s.v_a: %v %d", db, err, n)
				}
				var kinds string
				_ = conn.QueryRowContext(ctx, `SELECT CONCAT((SELECT COUNT(*) FROM information_schema.ROUTINES WHERE ROUTINE_SCHEMA = ?), '/',
				  (SELECT COUNT(*) FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA = ?), '/', (SELECT COUNT(*) FROM information_schema.EVENTS WHERE EVENT_SCHEMA = ?))`, db, db, db).Scan(&kinds)
				if kinds != "2/1/1" {
					t.Fatalf("%s routines/triggers/events = %s", db, kinds)
				}
				var ai string
				_ = conn.QueryRowContext(ctx, "SELECT AUTO_INCREMENT FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = 'pais'", db).Scan(&ai)
				if ai == "" {
					t.Fatalf("%s: AUTO_INCREMENT lost", db)
				}
			}

			// 4. CSV import into the copy.
			run("CREATE TABLE `" + cp + "`.imp (id INT PRIMARY KEY, nome VARCHAR(40), valor DECIMAL(10,2), obs TEXT)")
			csv := "id;nome;valor;obs\n1;\"Ana; \"\"A\"\"\";1.234,50;\\N\n2;Bia;10;\"multi\nlinha\"\n"
			r, err := csvimport.NewReader(strings.NewReader(csv), csvimport.Format{Separator: ";", Encloser: `"`, SkipLines: 1})
			if err != nil {
				t.Fatal(err)
			}
			stats, err := csvimport.Import(ctx, conn, r, csvimport.Target{Database: cp, Table: "imp", Columns: []string{"id", "nome", "valor", "obs"},
				Numeric: []bool{true, false, true, false}, Thousands: ".", Decimal: ",", Atomic: true, BatchRows: 1}, int64(len(csv)), &quietProgress{})
			if err != nil || stats.Rows != 2 || stats.Inserted != 2 {
				t.Fatalf("import %v %+v", err, stats)
			}
			var nome, valor string
			var obs sql.NullString
			_ = conn.QueryRowContext(ctx, "SELECT nome, valor, obs FROM `"+cp+"`.imp WHERE id = 1").Scan(&nome, &valor, &obs)
			if nome != `Ana; "A"` || valor != "1234.50" || obs.Valid {
				t.Fatalf("imported row 1: %q %q %v", nome, valor, obs)
			}
			// A failing row rolls back an atomic import.
			bad := "3;x;1\n1;dup;2\n"
			r, _ = csvimport.NewReader(strings.NewReader(bad), csvimport.Format{Separator: ";"})
			if _, err := csvimport.Import(ctx, conn, r, csvimport.Target{Database: cp, Table: "imp", Columns: []string{"id", "nome", "valor"}, Atomic: true, BatchRows: 1}, 0, &quietProgress{}); err == nil {
				t.Fatal("expected duplicate key error")
			}
			var count int
			_ = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+cp+"`.imp").Scan(&count)
			if count != 2 {
				t.Fatalf("atomic import left %d rows", count)
			}
		})
	}
}

package integration

import (
	"context"
	"testing"

	"dbstudio/internal/apperr"
	"dbstudio/internal/connections"
	"dbstudio/internal/database"
	"dbstudio/internal/query"
)

func TestCreateDatabaseAndTable(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			e := setup(t, tg)
			e.connect(t)
			ctx := context.Background()
			sess, _ := e.mgr.Get(e.profile.ID)
			b := sess.Driver.(database.DDLBuilder)

			opts, err := b.DDLOptions(ctx, sess.DB)
			if err != nil {
				t.Fatal(err)
			}
			var utf8mb4 *database.CharsetInfo
			for i := range opts.Charsets {
				if opts.Charsets[i].Name == "utf8mb4" {
					utf8mb4 = &opts.Charsets[i]
				}
			}
			if utf8mb4 == nil || len(utf8mb4.Collations) < 5 || opts.DefaultEngine != "InnoDB" || opts.DefaultCharset == "" {
				t.Fatalf("options: engine=%q charset=%q utf8mb4=%+v", opts.DefaultEngine, opts.DefaultCharset, utf8mb4)
			}
			t.Logf("default %s/%s, %d charsets, utf8mb4 has %d collations", opts.DefaultCharset, opts.DefaultCollation, len(opts.Charsets), len(utf8mb4.Collations))

			dbName := uniq("it_db")
			stmt, err := b.CreateDatabaseSQL(database.CreateDatabaseSpec{Name: dbName, Charset: "utf8mb4", Collation: "utf8mb4_unicode_ci"})
			if err != nil {
				t.Fatal(err)
			}
			if err := e.queries.RunDDL(ctx, e.profile.ID, stmt, "", false); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = e.queries.RunDDL(ctx, e.profile.ID, "DROP DATABASE `"+dbName+"`", "", true) }()
			var coll string
			if err := sess.DB.QueryRow("SELECT DEFAULT_COLLATION_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?", dbName).Scan(&coll); err != nil || coll != "utf8mb4_unicode_ci" {
				t.Fatalf("database collation %q %v", coll, err)
			}

			spec := database.CreateTableSpec{Database: dbName, Name: "pedidos", Engine: "InnoDB", Comment: "teste",
				Columns: []database.ColumnSpec{
					{Name: "id", Type: "BIGINT", Unsigned: true, PrimaryKey: true, AutoIncrement: true},
					{Name: "status", Type: "ENUM", Values: []string{"novo", "pago"}, DefaultKind: "value", Default: "novo"},
					{Name: "total", Type: "DECIMAL", Length: "10,2", DefaultKind: "value", Default: "0"},
					{Name: "email", Type: "VARCHAR", Length: "190", Nullable: true, Unique: true},
					{Name: "criado_em", Type: "TIMESTAMP", DefaultKind: "current_timestamp", OnUpdateCurrentTimestamp: true},
				},
				Indexes: []database.IndexSpec{{Columns: []string{"status", "criado_em"}}},
			}
			stmt, err = b.CreateTableSQL(spec)
			if err != nil {
				t.Fatal(err)
			}
			if err := e.queries.RunDDL(ctx, e.profile.ID, stmt, dbName, false); err != nil {
				t.Fatalf("%v\n%s", err, stmt)
			}
			e.meta.Invalidate(e.profile.ID, dbName)
			cols, err := e.meta.Columns(ctx, e.profile.ID, dbName, "pedidos")
			if err != nil || len(cols) != 5 || cols[0].Extra != "auto_increment" || cols[3].Key != "UNI" || !cols[3].Nullable {
				t.Fatalf("columns %v %+v", err, cols)
			}
			idx, _ := e.meta.Indexes(ctx, e.profile.ID, dbName, "pedidos")
			if len(idx) != 3 {
				t.Fatalf("indexes %+v", idx)
			}
			// The table works: defaults and auto increment.
			e.exec(t, "tab-ddl-00001", "INSERT INTO `"+dbName+"`.pedidos (email) VALUES ('a@b.c')", "statement")
			r := e.exec(t, "tab-ddl-00001", "SELECT id, status, total FROM `"+dbName+"`.pedidos", "statement")
			if row := r.Results[0].Rows[0]; row[1] != "novo" || row[2] != "0.00" {
				t.Fatalf("row %v", row)
			}
			// Server errors come back as database errors.
			err = e.queries.RunDDL(ctx, e.profile.ID, stmt, dbName, false)
			if ae := appErr(err); ae == nil || ae.DB == nil || ae.DB.Code != 1050 {
				t.Fatalf("expected 'table exists' error, got %v", err)
			}
		})
	}
}

func TestCreateTablePolicies(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			prod := setup(t, tg, func(p *connections.Profile) { p.Environment = connections.EnvProduction })
			prod.connect(t)
			stmt := "CREATE TABLE `studio_it`.`" + uniq("prod") + "` (`a` INT NOT NULL)"
			err := prod.queries.RunDDL(context.Background(), prod.profile.ID, stmt, "studio_it", false)
			if ae := appErr(err); ae == nil || ae.Code != apperr.CodeConfirmationRequired {
				t.Fatalf("production must require confirmation, got %v", err)
			}
			if _, ok := appErr(err).Details.(query.ConfirmationDetails); !ok {
				t.Fatal("missing confirmation details")
			}
			ro := setup(t, tg, func(p *connections.Profile) { p.ReadOnly = true })
			ro.connect(t)
			err = ro.queries.RunDDL(context.Background(), ro.profile.ID, "CREATE DATABASE x_ro_blocked", "", true)
			if ae := appErr(err); ae == nil || ae.Code != apperr.CodeReadOnly {
				t.Fatalf("read-only must block, got %v", err)
			}
		})
	}
}

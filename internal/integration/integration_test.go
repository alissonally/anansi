// Package integration runs end-to-end tests against real MySQL and MariaDB
// servers. They are skipped unless the servers are configured:
//
//	eval "$(scripts/integration-db.sh env)"; go test ./internal/integration/ -v
package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"anansi/internal/apperr"
	"anansi/internal/autocomplete"
	"anansi/internal/connections"
	"anansi/internal/database"
	"anansi/internal/database/mysql"
	"anansi/internal/metadata"
	"anansi/internal/query"
	"anansi/internal/security"
)

type target struct {
	name string
	kind database.Kind
	addr string
}

func targets(t *testing.T) []target {
	var out []target
	if a := os.Getenv("ANANSI_IT_MYSQL"); a != "" {
		out = append(out, target{"mysql", database.MySQL, a})
	}
	if a := os.Getenv("ANANSI_IT_MARIADB"); a != "" {
		out = append(out, target{"mariadb", database.MariaDB, a})
	}
	if len(out) == 0 {
		t.Skip("servidores de integração não configurados (veja scripts/integration-db.sh)")
	}
	return out
}

func password() string { return os.Getenv("ANANSI_IT_PASSWORD") }

type env struct {
	svc     *connections.Service
	mgr     *connections.Manager
	queries *query.Service
	meta    *metadata.Cache
	profile connections.Profile
	opts    query.Options
	events  []query.StatementEvent
	evMu    sync.Mutex
}

func setup(t *testing.T, tg target, mutate ...func(*connections.Profile)) *env {
	t.Helper()
	host, portS, _ := net.SplitHostPort(tg.addr)
	port, _ := strconv.Atoi(portS)
	store, err := connections.OpenProfileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reg := database.NewRegistry(mysql.New())
	e := &env{opts: query.Options{PageSize: 500, ConfirmDestructive: true}}
	e.mgr = connections.NewManager(connections.RegistryOpener(reg, nil))
	e.svc = &connections.Service{Store: store, Secrets: security.NewMemory(), Memory: security.NewMemory(), Manager: e.mgr, Registry: reg}
	p := connections.Profile{Name: "IT " + tg.name, Kind: tg.kind, Host: host, Port: port, User: "root", Database: "studio_it",
		TLS: database.TLSOptions{Mode: database.TLSPreferred}, SavePassword: true, Environment: connections.EnvLocal}
	for _, m := range mutate {
		m(&p)
	}
	pw := password()
	res, err := e.svc.Save(connections.SaveInput{Profile: p, Password: &pw})
	if err != nil {
		t.Fatal(err)
	}
	e.profile = res.Profile.Profile
	e.meta = metadata.NewCache(e.mgr)
	e.queries = query.NewService(e.mgr, func() query.Options { return e.opts }, query.Hooks{AfterStatement: func(ev query.StatementEvent) {
		e.evMu.Lock()
		e.events = append(e.events, ev)
		e.evMu.Unlock()
	}})
	t.Cleanup(func() {
		e.queries.Close()
		_ = e.mgr.CloseAll(context.Background())
	})
	return e
}

func (e *env) connect(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := e.svc.Connect(ctx, e.profile.ID, nil, false); err != nil {
		t.Fatalf("connect: %v", err)
	}
}

func (e *env) exec(t *testing.T, tab, sql string, mode string) *query.ExecuteResponse {
	t.Helper()
	resp, err := e.queries.Execute(context.Background(), query.ExecuteRequest{TabID: tab, ConnectionID: e.profile.ID, SQL: sql, CursorOffset: -1, Mode: mode, Confirmed: true})
	if err != nil {
		t.Fatalf("execute %q: %v", sql, err)
	}
	for _, r := range resp.Results {
		if r.Error != nil {
			t.Fatalf("statement %q failed: %s", r.SQL, r.Error.Message)
		}
	}
	return resp
}

func appErr(err error) *apperr.Error {
	var ae *apperr.Error
	if errors.As(err, &ae) {
		return ae
	}
	return nil
}

func uniq(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano()%1_000_000_000)
}

func TestConnectServerInfoAndClose(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			e := setup(t, tg)
			e.connect(t)
			s, err := e.mgr.Get(e.profile.ID)
			if err != nil {
				t.Fatal(err)
			}
			wantFlavor := map[database.Kind]string{database.MySQL: "MySQL", database.MariaDB: "MariaDB"}[tg.kind]
			if s.Server.Flavor != wantFlavor {
				t.Errorf("flavor %q, want %q (version %s)", s.Server.Flavor, wantFlavor, s.Server.Version)
			}
			t.Logf("server %s %s tls=%v cipher=%s", s.Server.Flavor, s.Server.Version, s.Server.TLSActive, s.Server.TLSCipher)
			if len(e.mgr.States()) != 1 {
				t.Fatal("expected one session")
			}
			if err := e.mgr.Disconnect(e.profile.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := e.mgr.Get(e.profile.ID); appErr(err) == nil || appErr(err).Code != apperr.CodeNotConnected {
				t.Fatalf("expected not_connected, got %v", err)
			}
			if err := s.DB.Ping(); err == nil {
				t.Fatal("pool should be closed")
			}
		})
	}
}

func TestTestConnection(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			e := setup(t, tg)
			pw := password()
			info, err := e.svc.Test(context.Background(), connections.TestInput{Profile: e.profile, Password: &pw})
			if err != nil || info.Version == "" {
				t.Fatalf("test failed: %v", err)
			}
			if len(e.mgr.States()) != 0 {
				t.Fatal("test connection must not leave a session open")
			}
		})
	}
}

func TestAuthenticationErrorDoesNotLeakPassword(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			e := setup(t, tg)
			wrong := "Wr0ng-Secret-Value"
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			_, err := e.svc.Connect(ctx, e.profile.ID, &wrong, false)
			ae := appErr(err)
			if ae == nil || ae.DB == nil || ae.DB.Category != "auth" {
				t.Fatalf("expected auth error, got %v", err)
			}
			if strings.Contains(err.Error(), wrong) {
				t.Fatal("password leaked in error")
			}
		})
	}
}

func TestConnectTimeout(t *testing.T) {
	targets(t)
	reg := database.NewRegistry(mysql.New())
	open := connections.RegistryOpener(reg, nil)
	// 10.255.255.1 is non-routable: the dial hangs until the timeout.
	p := connections.Profile{ID: connections.NewID(), Name: "x", Kind: database.MySQL, Host: "10.255.255.1", Port: 3306, User: "u",
		TLS: database.TLSOptions{Mode: database.TLSDisabled}, ConnectTimeoutSeconds: 1}.Normalize()
	p.ConnectTimeoutSeconds = 1
	start := time.Now()
	_, err := open(context.Background(), p, "secret")
	if err == nil {
		t.Fatal("expected error")
	}
	if time.Since(start) > 8*time.Second {
		t.Fatalf("timeout not respected: %v", time.Since(start))
	}
	ae := appErr(err)
	if ae == nil || ae.DB == nil || (ae.DB.Category != "timeout" && ae.DB.Category != "network") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConcurrentConnectSharesSession(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			e := setup(t, tg)
			var wg sync.WaitGroup
			sessions := make([]*connections.Session, 16)
			errs := make([]error, 16)
			p, _ := e.svc.Store.Get(e.profile.ID)
			for i := range sessions {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					sessions[i], errs[i] = e.mgr.Connect(context.Background(), p, password())
				}(i)
			}
			wg.Wait()
			for i := range sessions {
				if errs[i] != nil {
					t.Fatal(errs[i])
				}
				if sessions[i] != sessions[0] {
					t.Fatal("concurrent connects created different sessions")
				}
			}
			// Concurrent disconnect + states must be race-free.
			for i := 0; i < 8; i++ {
				wg.Add(2)
				go func() { defer wg.Done(); _ = e.mgr.Disconnect(p.ID) }()
				go func() { defer wg.Done(); _ = e.mgr.States() }()
			}
			wg.Wait()
			if len(e.mgr.States()) != 0 {
				t.Fatal("session should be closed")
			}
		})
	}
}

func TestQueryResultsNullsDuplicatesAndTypes(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			e := setup(t, tg)
			e.connect(t)
			tbl := uniq("types")
			e.exec(t, "tab-types-0001", fmt.Sprintf(`CREATE TABLE %s (id BIGINT UNSIGNED PRIMARY KEY, name VARCHAR(20) NULL, price DECIMAL(20,6), raw VARBINARY(8), created DATETIME(3), big BIGINT)`, tbl), "script")
			e.exec(t, "tab-types-0001", fmt.Sprintf(`INSERT INTO %s VALUES (1, 'NULL', 12.345678, 0x00FF10, '2024-02-29 13:14:15.123', 9007199254740993), (2, NULL, NULL, NULL, NULL, -5)`, tbl), "script")
			resp := e.exec(t, "tab-types-0001", fmt.Sprintf("SELECT id, name, price, raw, created, big, id, name AS id FROM %s ORDER BY 1", tbl), "statement")
			r := resp.Results[0]
			if len(r.Columns) != 8 || r.Columns[0].Name != "id" || r.Columns[6].Name != "id" || r.Columns[7].Name != "id" {
				t.Fatalf("duplicate column names not preserved: %+v", r.Columns)
			}
			if len(r.Rows) != 2 {
				t.Fatalf("rows: %v", r.Rows)
			}
			row1, row2 := r.Rows[0], r.Rows[1]
			if row1[1] != "NULL" || row2[1] != nil {
				t.Errorf("string 'NULL' vs SQL NULL confused: %#v / %#v", row1[1], row2[1])
			}
			if fmt.Sprint(row1[2]) != "12.345678" {
				t.Errorf("decimal lost precision: %#v", row1[2])
			}
			if row1[3] != "0x00FF10" || !r.Columns[3].Binary {
				t.Errorf("binary: %#v", row1[3])
			}
			if row1[4] != "2024-02-29 13:14:15.123" {
				t.Errorf("datetime: %#v", row1[4])
			}
			if fmt.Sprint(row1[5]) != "9007199254740993" {
				t.Errorf("bigint lost precision: %#v", row1[5])
			}
			if _, isString := row1[5].(string); !isString {
				t.Errorf("unsafe integer must be a string, got %T", row1[5])
			}
			// JSON round trip as the frontend receives it.
			b, _ := json.Marshal(r.Rows)
			if !strings.Contains(string(b), `null`) || !strings.Contains(string(b), `"9007199254740993"`) {
				t.Errorf("json: %s", b)
			}
			// Statements without result sets.
			resp = e.exec(t, "tab-types-0001", fmt.Sprintf("UPDATE %s SET name = 'x' WHERE id = 2", tbl), "statement")
			if resp.Results[0].HasRows || resp.Results[0].RowsAffected == nil || *resp.Results[0].RowsAffected != 1 {
				t.Errorf("update result: %+v", resp.Results[0])
			}
			e.exec(t, "tab-types-0001", "DROP TABLE "+tbl, "statement")
		})
	}
}

func TestPagingWithCursor(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			e := setup(t, tg)
			e.connect(t)
			sql := "WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 1203) SELECT i FROM n"
			if tg.kind == database.MySQL {
				e.exec(t, "tab-paging-01", "SET SESSION cte_max_recursion_depth = 5000", "statement")
			} else {
				// MariaDB stops recursive CTEs at 1000 iterations by default.
				e.exec(t, "tab-paging-01", "SET SESSION max_recursive_iterations = 5000", "statement")
			}
			resp := e.exec(t, "tab-paging-01", sql, "statement")
			r := resp.Results[0]
			if len(r.Rows) != 500 || !r.HasMore || r.CursorID == "" {
				t.Fatalf("first page: rows=%d hasMore=%v cursor=%q", len(r.Rows), r.HasMore, r.CursorID)
			}
			total := len(r.Rows)
			for {
				page, err := e.queries.FetchMore(query.FetchRequest{TabID: "tab-paging-01", CursorID: r.CursorID})
				if err != nil {
					t.Fatal(err)
				}
				if page.Offset != total {
					t.Fatalf("offset %d, want %d", page.Offset, total)
				}
				total += len(page.Rows)
				if !page.HasMore {
					break
				}
			}
			if total != 1203 {
				t.Fatalf("total rows %d", total)
			}
			// Cursor is gone; the tab connection is still usable.
			if _, err := e.queries.FetchMore(query.FetchRequest{TabID: "tab-paging-01", CursorID: r.CursorID}); appErr(err) == nil || appErr(err).Code != apperr.CodeNotFound {
				t.Fatalf("expected not_found, got %v", err)
			}
			// Abandoning a cursor midway must be fast and keep the tab usable.
			resp = e.exec(t, "tab-paging-01", "WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 4000) SELECT i, REPEAT('x', 200) FROM n", "statement")
			if !resp.Results[0].HasMore {
				t.Fatal("expected more rows")
			}
			start := time.Now()
			resp = e.exec(t, "tab-paging-01", "SELECT 42", "statement")
			if time.Since(start) > 3*time.Second || fmt.Sprint(resp.Results[0].Rows[0][0]) != "42" {
				t.Fatalf("replacing cursor: %v %v", time.Since(start), resp.Results[0].Rows)
			}
		})
	}
}

func TestCancelLongQuery(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			e := setup(t, tg)
			e.connect(t)
			tab := "tab-cancel-01"
			e.exec(t, tab, "SET @marker = 7", "statement")
			go func() {
				time.Sleep(500 * time.Millisecond)
				_ = e.queries.Cancel(tab)
			}()
			start := time.Now()
			resp, err := e.queries.Execute(context.Background(), query.ExecuteRequest{TabID: tab, ConnectionID: e.profile.ID, SQL: "SELECT SLEEP(30); SELECT 'not reached'", CursorOffset: -1, Mode: "script"})
			if err != nil {
				t.Fatal(err)
			}
			if el := time.Since(start); el > 6*time.Second {
				t.Fatalf("cancel took %v", el)
			}
			if !resp.Cancelled || len(resp.Results) != 1 {
				t.Fatalf("expected cancelled after first statement: %+v", resp)
			}
			// KILL QUERY keeps the connection: session variables survive.
			resp = e.exec(t, tab, "SELECT @marker", "statement")
			if fmt.Sprint(resp.Results[0].Rows[0][0]) != "7" {
				t.Fatalf("session state lost after cancel: %v", resp.Results[0].Rows)
			}
		})
	}
}

func TestQueryTimeout(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			e := setup(t, tg)
			e.connect(t)
			e.opts.Timeout = 700 * time.Millisecond
			start := time.Now()
			resp, err := e.queries.Execute(context.Background(), query.ExecuteRequest{TabID: "tab-timeout-01", ConnectionID: e.profile.ID, SQL: "SELECT SLEEP(20)", CursorOffset: -1})
			if err != nil || time.Since(start) > 6*time.Second || !resp.Cancelled {
				t.Fatalf("timeout not applied: %v %v %+v", err, time.Since(start), resp)
			}
			// MySQL returns 1 from an interrupted SLEEP; MariaDB raises 1317.
			if r := resp.Results[0]; r.Error != nil && r.Error.Category != "cancelled" {
				t.Fatalf("unexpected error: %+v", r.Error)
			}
		})
	}
}

func TestTransactionsAndTabIndependence(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			e := setup(t, tg)
			e.connect(t)
			tbl := uniq("tx")
			e.exec(t, "tab-a-000001", fmt.Sprintf("CREATE TABLE %s (id INT PRIMARY KEY) ENGINE=InnoDB", tbl), "statement")
			r := e.exec(t, "tab-a-000001", "BEGIN", "statement")
			if !r.InTransaction {
				t.Fatal("transaction not tracked")
			}
			e.exec(t, "tab-a-000001", fmt.Sprintf("INSERT INTO %s VALUES (1)", tbl), "statement")
			// Same tab sees its uncommitted row; another tab does not.
			a := e.exec(t, "tab-a-000001", "SELECT COUNT(*) FROM "+tbl, "statement")
			b := e.exec(t, "tab-b-000001", "SELECT COUNT(*) FROM "+tbl, "statement")
			if fmt.Sprint(a.Results[0].Rows[0][0]) != "1" || fmt.Sprint(b.Results[0].Rows[0][0]) != "0" {
				t.Fatalf("isolation: a=%v b=%v", a.Results[0].Rows, b.Results[0].Rows)
			}
			// Releasing a tab with an open transaction requires confirmation.
			if err := e.queries.ReleaseTab("tab-a-000001", false); appErr(err) == nil || appErr(err).Code != apperr.CodeConfirmationRequired {
				t.Fatalf("expected confirmation, got %v", err)
			}
			r = e.exec(t, "tab-a-000001", "ROLLBACK", "statement")
			if r.InTransaction {
				t.Fatal("transaction still flagged")
			}
			// Different databases per tab.
			e.exec(t, "tab-b-000001", "USE information_schema", "statement")
			ra := e.exec(t, "tab-a-000001", "SELECT DATABASE()", "statement")
			rb := e.exec(t, "tab-b-000001", "SELECT DATABASE()", "statement")
			if ra.Database != "studio_it" || rb.Database != "information_schema" {
				t.Fatalf("databases: %q %q", ra.Database, rb.Database)
			}
			// Database requested by the tab is applied before running.
			resp, err := e.queries.Execute(context.Background(), query.ExecuteRequest{TabID: "tab-c-000001", ConnectionID: e.profile.ID, SQL: "SELECT DATABASE()", CursorOffset: -1, Database: "mysql"})
			if err != nil || fmt.Sprint(resp.Results[0].Rows[0][0]) != "mysql" {
				t.Fatalf("database switch: %v %+v", err, resp)
			}
			// Concurrent executions on different tabs.
			var wg sync.WaitGroup
			for i := 0; i < 6; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					tab := fmt.Sprintf("tab-par-%06d", i)
					resp, err := e.queries.Execute(context.Background(), query.ExecuteRequest{TabID: tab, ConnectionID: e.profile.ID, SQL: fmt.Sprintf("SELECT %d, SLEEP(0.2)", i), CursorOffset: -1})
					if err != nil || fmt.Sprint(resp.Results[0].Rows[0][0]) != strconv.Itoa(i) {
						t.Errorf("tab %d got %v %v", i, err, resp)
					}
				}(i)
			}
			wg.Wait()
			// The same tab rejects a concurrent execution.
			done := make(chan struct{})
			go func() {
				defer close(done)
				_, _ = e.queries.Execute(context.Background(), query.ExecuteRequest{TabID: "tab-busy-0001", ConnectionID: e.profile.ID, SQL: "SELECT SLEEP(1)", CursorOffset: -1})
			}()
			time.Sleep(300 * time.Millisecond)
			_, err = e.queries.Execute(context.Background(), query.ExecuteRequest{TabID: "tab-busy-0001", ConnectionID: e.profile.ID, SQL: "SELECT 1", CursorOffset: -1})
			if appErr(err) == nil || appErr(err).Code != apperr.CodeBusy {
				t.Fatalf("expected busy, got %v", err)
			}
			<-done
			e.exec(t, "tab-a-000001", "DROP TABLE "+tbl, "statement")
		})
	}
}

func TestScriptStopsOnErrorWithLine(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			e := setup(t, tg)
			e.connect(t)
			resp, err := e.queries.Execute(context.Background(), query.ExecuteRequest{TabID: "tab-script-01", ConnectionID: e.profile.ID,
				SQL: "SELECT 1;\nSELECT 2;\nSELECT *\nFROM\nWHERE;\nSELECT 4", CursorOffset: -1, Mode: "script"})
			if err != nil {
				t.Fatal(err)
			}
			if len(resp.Results) != 3 {
				t.Fatalf("expected stop at 3rd statement, got %d", len(resp.Results))
			}
			r := resp.Results[2]
			if r.Error == nil || r.Error.Category != "syntax" || r.Error.Code != 1064 || r.Line != 3 || r.Error.Line == 0 {
				t.Fatalf("syntax error info: %+v / %+v", r, r.Error)
			}
			// Statement under the cursor only.
			resp, err = e.queries.Execute(context.Background(), query.ExecuteRequest{TabID: "tab-script-01", ConnectionID: e.profile.ID,
				SQL: "SELECT 1;\nSELECT 2;", CursorOffset: 12, Mode: "statement"})
			if err != nil || len(resp.Results) != 1 || resp.Results[0].SQL != "SELECT 2" {
				t.Fatalf("statement at cursor: %v %+v", err, resp)
			}
		})
	}
}

func TestPolicies(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			e := setup(t, tg)
			e.connect(t)
			tbl := uniq("pol")
			e.exec(t, "tab-pol-0001", fmt.Sprintf("CREATE TABLE %s (id INT)", tbl), "statement")
			_, err := e.queries.Execute(context.Background(), query.ExecuteRequest{TabID: "tab-pol-0001", ConnectionID: e.profile.ID, SQL: "DELETE FROM " + tbl, CursorOffset: -1})
			ae := appErr(err)
			if ae == nil || ae.Code != apperr.CodeConfirmationRequired {
				t.Fatalf("expected confirmation_required, got %v", err)
			}
			e.exec(t, "tab-pol-0001", "DROP TABLE "+tbl, "statement")

			ro := setup(t, tg, func(p *connections.Profile) { p.ReadOnly = true; p.Environment = connections.EnvProduction })
			ro.connect(t)
			ro.exec(t, "tab-ro-00001", "SELECT 1", "statement")
			_, err = ro.queries.Execute(context.Background(), query.ExecuteRequest{TabID: "tab-ro-00001", ConnectionID: ro.profile.ID, SQL: "CREATE TABLE x (a int)", CursorOffset: -1, Confirmed: true})
			if appErr(err) == nil || appErr(err).Code != apperr.CodeReadOnly {
				t.Fatalf("expected read_only, got %v", err)
			}
		})
	}
}

func TestMetadataAndAutocomplete(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			e := setup(t, tg)
			e.connect(t)
			tbl := uniq("meta")
			e.exec(t, "tab-meta-001", fmt.Sprintf("CREATE TABLE %s (id INT PRIMARY KEY, email VARCHAR(100) NOT NULL, team_id INT, UNIQUE KEY uq_email (email), KEY ix_team (team_id))", tbl), "statement")
			defer e.exec(t, "tab-meta-001", "DROP TABLE "+tbl, "statement")
			ctx := context.Background()
			dbs, err := e.meta.Databases(ctx, e.profile.ID)
			if err != nil || !containsDB(dbs, "studio_it") {
				t.Fatalf("databases: %v %v", err, dbs)
			}
			tables, err := e.meta.Tables(ctx, e.profile.ID, "studio_it")
			if err != nil || !containsTable(tables, tbl) {
				t.Fatalf("tables: %v %v", err, tables)
			}
			cols, err := e.meta.Columns(ctx, e.profile.ID, "studio_it", tbl)
			if err != nil || len(cols) != 3 || cols[0].Name != "id" || cols[0].Key != "PRI" || cols[1].Nullable {
				t.Fatalf("columns: %v %+v", err, cols)
			}
			idx, err := e.meta.Indexes(ctx, e.profile.ID, "studio_it", tbl)
			if err != nil || len(idx) != 3 || !idx[0].Primary {
				t.Fatalf("indexes: %v %+v", err, idx)
			}
			// Cache invalidation after DDL.
			e.exec(t, "tab-meta-001", fmt.Sprintf("ALTER TABLE %s ADD COLUMN note TEXT", tbl), "statement")
			e.meta.Invalidate(e.profile.ID, "studio_it")
			cols, _ = e.meta.Columns(ctx, e.profile.ID, "studio_it", tbl)
			if len(cols) != 4 {
				t.Fatalf("cache not invalidated: %+v", cols)
			}
			// Autocomplete backed by real metadata.
			cat := catalogAdapter{e.meta, e.profile.ID}
			sql := fmt.Sprintf("SELECT m. FROM %s m", tbl)
			resp := autocomplete.Engine{}.Complete(ctx, cat, autocomplete.Request{SQL: sql, Offset: len("SELECT m."), Database: "studio_it"})
			var names []string
			for _, it := range resp.Items {
				names = append(names, it.Label)
			}
			if strings.Join(names, ",") != "id,email,team_id,note" {
				t.Fatalf("autocomplete columns: %v (note=%s)", names, resp.MetadataNote)
			}
		})
	}
}

type catalogAdapter struct {
	c  *metadata.Cache
	id string
}

func (c catalogAdapter) Databases(ctx context.Context) ([]database.DatabaseInfo, error) {
	return c.c.Databases(ctx, c.id)
}
func (c catalogAdapter) Tables(ctx context.Context, db string) ([]database.TableInfo, error) {
	return c.c.Tables(ctx, c.id, db)
}
func (c catalogAdapter) Columns(ctx context.Context, db, t string) ([]database.ColumnInfo, error) {
	return c.c.Columns(ctx, c.id, db, t)
}
func (c catalogAdapter) Routines(ctx context.Context, db string) ([]database.RoutineInfo, error) {
	return c.c.Routines(ctx, c.id, db)
}

func containsDB(l []database.DatabaseInfo, n string) bool {
	for _, d := range l {
		if d.Name == n {
			return true
		}
	}
	return false
}

func containsTable(l []database.TableInfo, n string) bool {
	for _, d := range l {
		if d.Name == n {
			return true
		}
	}
	return false
}

func TestDisconnectReleasesTabs(t *testing.T) {
	for _, tg := range targets(t) {
		t.Run(tg.name, func(t *testing.T) {
			e := setup(t, tg)
			e.connect(t)
			e.exec(t, "tab-rel-0001", "SELECT 1", "statement")
			// A long query running while the session is closed is aborted.
			done := make(chan *query.ExecuteResponse)
			go func() {
				r, _ := e.queries.Execute(context.Background(), query.ExecuteRequest{TabID: "tab-rel-0002", ConnectionID: e.profile.ID, SQL: "SELECT SLEEP(30)", CursorOffset: -1})
				done <- r
			}()
			time.Sleep(400 * time.Millisecond)
			start := time.Now()
			if err := e.mgr.Disconnect(e.profile.ID); err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("running query not aborted on disconnect")
			}
			if time.Since(start) > 9*time.Second {
				t.Fatal("disconnect too slow")
			}
			_, err := e.queries.Execute(context.Background(), query.ExecuteRequest{TabID: "tab-rel-0001", ConnectionID: e.profile.ID, SQL: "SELECT 1", CursorOffset: -1})
			if appErr(err) == nil || appErr(err).Code != apperr.CodeNotConnected {
				t.Fatalf("expected not_connected, got %v", err)
			}
			// Reconnecting gives the tab a fresh connection.
			e.connect(t)
			e.exec(t, "tab-rel-0001", "SELECT 1", "statement")
		})
	}
}

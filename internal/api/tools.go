package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"anansi/internal/apperr"
	"anansi/internal/connections"
	"anansi/internal/database"
	"anansi/internal/database/mysql"
	"anansi/internal/query"
	"anansi/internal/tasks"
)

// The table tools and the user manager generate MySQL/MariaDB-specific SQL.
func (a *App) mysqlDriver(id string) (*connections.Session, *mysql.Driver, error) {
	sess, err := a.session(id)
	if err != nil {
		return nil, nil, err
	}
	d, ok := sess.Driver.(*mysql.Driver)
	if !ok {
		return nil, nil, apperr.New(apperr.CodeValidation, "ferramenta disponível apenas para MySQL/MariaDB")
	}
	return sess, d, nil
}

// TableRef names a table in tool requests.
type TableRef struct {
	Database string `json:"database"`
	Table    string `json:"table"`
}

// ---- maintenance -------------------------------------------------------------------

// MaintenanceRow is one line returned by CHECK/ANALYZE/OPTIMIZE/REPAIR/CHECKSUM.
type MaintenanceRow struct {
	Table   string `json:"table"`
	Op      string `json:"op"`
	MsgType string `json:"msgType"`
	Message string `json:"message"`
}

// StartMaintenance runs a maintenance operation on tables in the background.
// OPTIMIZE and REPAIR change tables and follow the write policies.
func (a *App) StartMaintenance(id, op string, options []string, tables []TableRef, confirmed bool) (string, error) {
	_, d, err := a.mysqlDriver(id)
	if err != nil {
		return "", err
	}
	if len(tables) == 0 {
		return "", apperr.New(apperr.CodeValidation, "selecione as tabelas")
	}
	var stmts []string
	for _, t := range tables {
		s, err := d.MaintenanceSQL(op, options, t.Database, t.Table)
		if err != nil {
			return "", wrap(err)
		}
		stmts = append(stmts, s)
	}
	up := strings.ToUpper(op)
	if up == "OPTIMIZE" || up == "REPAIR" {
		if err := a.writeGuard(id, fmt.Sprintf("%s TABLE em %d tabela(s)", up, len(tables)), confirmed); err != nil {
			return "", err
		}
	}
	return a.tasks.Start("maintenance", fmt.Sprintf("%s TABLE (%d)", up, len(tables)), "tables", func(ctx context.Context, p *tasks.Progress) (string, error) {
		var out []MaintenanceRow
		p.Set(0, int64(len(stmts)))
		err := a.queries.WithConn(ctx, id, func(conn *sql.Conn, s *connections.Session) error {
			for i, st := range stmts {
				p.Message(tables[i].Database + "." + tables[i].Table)
				err := query.StreamRows(ctx, conn, st, nil, nil, func(row []any) error {
					str := func(i int) string {
						if i < len(row) && row[i] != nil {
							return fmt.Sprint(row[i])
						}
						return ""
					}
					if len(row) == 2 { // CHECKSUM: Table, Checksum
						out = append(out, MaintenanceRow{Table: str(0), Op: "checksum", MsgType: "checksum", Message: str(1)})
					} else {
						out = append(out, MaintenanceRow{Table: str(0), Op: str(1), MsgType: str(2), Message: str(3)})
					}
					return nil
				})
				if err != nil {
					out = append(out, MaintenanceRow{Table: tables[i].Database + "." + tables[i].Table, Op: strings.ToLower(op), MsgType: "Error", Message: query.DescribeErr(s, err).Error()})
					p.Warn("")
				}
				p.Set(int64(i+1), int64(len(stmts)))
				if ctx.Err() != nil {
					return ctx.Err()
				}
			}
			return nil
		})
		p.SetData(out)
		return fmt.Sprintf("%d tabela(s) processada(s)", len(stmts)), err
	}), nil
}

// ---- find text on server ---------------------------------------------------------------

// SearchHit is a table with matches.
type SearchHit struct {
	Database string `json:"database"`
	Table    string `json:"table"`
	Rows     int64  `json:"rows"`
	Total    int64  `json:"total"`
	// Filter is the WHERE condition with the search text (opens the grid).
	Filter string `json:"filter"`
}

// StartSearch looks for text in every column of the selected tables (all
// tables of the databases when Tables is empty).
func (a *App) StartSearch(id string, databases []string, tables []TableRef, spec mysql.SearchSpec) (string, error) {
	sess, d, err := a.mysqlDriver(id)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(spec.Text) == "" {
		return "", apperr.New(apperr.CodeValidation, "informe o texto a procurar")
	}
	maria := isMaria(sess)
	g, ok := sess.Driver.(database.ObjectSQL)
	if !ok {
		return "", apperr.New(apperr.CodeValidation, "busca indisponível")
	}
	return a.tasks.Start("search", "Buscar “"+truncate(spec.Text, 40)+"”", "tables", func(ctx context.Context, p *tasks.Progress) (string, error) {
		targets := append([]TableRef(nil), tables...)
		for _, db := range databases {
			list, err := a.meta.Tables(ctx, id, db)
			if err != nil {
				return "", err
			}
			for _, t := range list {
				if t.Type == "table" || t.Type == "view" {
					targets = append(targets, TableRef{db, t.Name})
				}
			}
		}
		var hits []SearchHit
		p.Set(0, int64(len(targets)))
		err := a.queries.WithConn(ctx, id, func(conn *sql.Conn, s *connections.Session) error {
			for i, t := range targets {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				p.Message(t.Database + "." + t.Table)
				cols, err := a.meta.Columns(ctx, id, t.Database, t.Table)
				if err != nil {
					p.Warn(err.Error())
					continue
				}
				cond, args, err := d.SearchCondition(cols, spec, maria)
				if err != nil {
					return err
				}
				if cond != "" {
					var n int64
					q := "SELECT COUNT(*) FROM " + g.QuoteIdent(t.Database) + "." + g.QuoteIdent(t.Table) + " WHERE " + cond
					if err := conn.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
						p.Warn(t.Database + "." + t.Table + ": " + query.DescribeErr(s, err).Error())
					} else if n > 0 {
						hits = append(hits, SearchHit{Database: t.Database, Table: t.Table, Rows: n, Filter: literalFilter(cond, args, g)})
						p.SetData(append([]SearchHit(nil), hits...))
					}
				}
				p.Set(int64(i+1), int64(len(targets)))
			}
			return nil
		})
		p.SetData(hits)
		return fmt.Sprintf("%d tabela(s) com ocorrências em %d pesquisada(s)", len(hits), len(targets)), err
	}), nil
}

// literalFilter renders the condition with its arguments as SQL literals so
// it can be typed into the data grid filter.
func literalFilter(cond string, args []any, g database.ObjectSQL) string {
	var b strings.Builder
	i := 0
	for _, r := range cond {
		if r == '?' && i < len(args) {
			b.WriteString(g.QuoteString(fmt.Sprint(args[i])))
			i++
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ---- bulk table editor ----------------------------------------------------------------

// PreviewBulkEdit returns the statements of the bulk table editor.
func (a *App) PreviewBulkEdit(id string, tables []TableRef, spec mysql.BulkSpec) ([]string, error) {
	_, d, err := a.mysqlDriver(id)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, t := range tables {
		s, err := d.BulkSQL(t.Database, t.Table, spec)
		if err != nil {
			return nil, wrap(err)
		}
		out = append(out, s...)
	}
	return out, nil
}

// ApplyBulkEdit runs the bulk table editor (every table is attempted).
func (a *App) ApplyBulkEdit(id string, tables []TableRef, spec mysql.BulkSpec, confirmed bool) ([]query.AdminResult, error) {
	stmts, err := a.PreviewBulkEdit(id, tables, spec)
	if err != nil {
		return nil, err
	}
	res, err := a.queries.RunAdmin(context.Background(), query.AdminRequest{ConnectionID: id, Statements: stmts, Confirmed: confirmed, ContinueOnError: true, Timeout: time.Hour})
	if err != nil {
		return nil, wrap(err)
	}
	a.meta.Invalidate(id, "")
	return res, nil
}

// ---- copy table -------------------------------------------------------------------------

// CopyTableSpec describes HeidiSQL's "Create table copy".
type CopyTableSpec struct {
	Database       string   `json:"database"`
	Table          string   `json:"table"`
	TargetDatabase string   `json:"targetDatabase"`
	TargetTable    string   `json:"targetTable"`
	Columns        []string `json:"columns"` // empty = all
	Indexes        bool     `json:"indexes"`
	ForeignKeys    bool     `json:"foreignKeys"`
	Checks         bool     `json:"checks"`
	Data           bool     `json:"data"`
	Filter         string   `json:"filter"`
	DropExisting   bool     `json:"dropExisting"`
}

func (a *App) copyStatements(id string, spec CopyTableSpec) ([]string, error) {
	sess, e, err := a.objectEditor(id)
	if err != nil {
		return nil, err
	}
	g, _ := sess.Driver.(database.ObjectSQL)
	if spec.TargetDatabase == "" {
		spec.TargetDatabase = spec.Database
	}
	if strings.TrimSpace(spec.TargetTable) == "" {
		return nil, apperr.New(apperr.CodeValidation, "informe o nome da cópia")
	}
	if spec.TargetDatabase == spec.Database && spec.TargetTable == spec.Table {
		return nil, apperr.New(apperr.CodeValidation, "a cópia precisa de outro nome ou outro database")
	}
	ctx, cancel := editCtx()
	defer cancel()
	def, err := e.LoadTableDef(ctx, sess.DB, spec.Database, spec.Table)
	if err != nil {
		return nil, dbErr(sess, err)
	}
	keep := map[string]bool{}
	for _, c := range spec.Columns {
		keep[strings.ToLower(c)] = true
	}
	var cols []database.ColumnDef
	var dataCols []string
	for _, c := range def.Columns {
		if len(keep) > 0 && !keep[strings.ToLower(c.Name)] {
			continue
		}
		c.OrigName = ""
		cols = append(cols, c)
		if c.Generated == "" {
			dataCols = append(dataCols, g.QuoteIdent(c.Name))
		}
	}
	if len(cols) == 0 {
		return nil, apperr.New(apperr.CodeValidation, "selecione ao menos uma coluna")
	}
	has := func(n string) bool {
		for _, c := range cols {
			if strings.EqualFold(c.Name, n) {
				return true
			}
		}
		return false
	}
	var idx []database.IndexDef
	for _, ix := range def.Indexes {
		ok := ix.Kind == "PRIMARY" || spec.Indexes
		for _, p := range ix.Parts {
			if p.Column != "" && !has(p.Column) {
				ok = false
			}
		}
		if ok {
			ix.OrigName = ""
			idx = append(idx, ix)
		}
	}
	indexed := map[string]bool{}
	for _, ix := range idx {
		if len(ix.Parts) > 0 {
			indexed[strings.ToLower(ix.Parts[0].Column)] = true
		}
	}
	for i := range cols {
		// AUTO_INCREMENT needs an index on the column (HeidiSQL drops it too).
		if cols[i].AutoIncrement && !indexed[strings.ToLower(cols[i].Name)] {
			cols[i].AutoIncrement = false
		}
	}
	var fks []database.ForeignKeyDef
	if spec.ForeignKeys {
		for i, fk := range def.ForeignKeys {
			ok := true
			for _, c := range fk.Columns {
				if !has(c) {
					ok = false
				}
			}
			if ok {
				// Constraint names are unique per database: derive new ones.
				fk.OrigName, fk.Name = "", fmt.Sprintf("%s_ibfk_%d", spec.TargetTable, i+1)
				if fk.RefDatabase == "" && spec.TargetDatabase != spec.Database {
					fk.RefDatabase = spec.Database
				}
				fks = append(fks, fk)
			}
		}
	}
	var checks []database.CheckDef
	if spec.Checks {
		for i, c := range def.Checks {
			c.OrigName, c.Name = "", fmt.Sprintf("%s_chk_%d", spec.TargetTable, i+1)
			checks = append(checks, c)
		}
	}
	out := database.TableDef{Database: spec.TargetDatabase, Name: spec.TargetTable, Columns: cols, Indexes: idx, ForeignKeys: fks, Checks: checks, Options: def.Options, MariaDB: def.MariaDB}
	out.Options.AutoIncrement = ""
	create, err := e.CreateTableDefSQL(out)
	if err != nil {
		return nil, wrap(err)
	}
	var stmts []string
	target := g.QuoteIdent(spec.TargetDatabase) + "." + g.QuoteIdent(spec.TargetTable)
	if spec.DropExisting {
		stmts = append(stmts, "DROP TABLE IF EXISTS "+target)
	}
	stmts = append(stmts, create)
	if spec.Data {
		s := "INSERT INTO " + target + " (" + strings.Join(dataCols, ", ") + ") SELECT " + strings.Join(dataCols, ", ") + " FROM " + g.QuoteIdent(spec.Database) + "." + g.QuoteIdent(spec.Table)
		if f := strings.TrimSpace(spec.Filter); f != "" {
			if strings.HasPrefix(strings.ToUpper(f), "WHERE ") {
				f = strings.TrimSpace(f[6:])
			}
			s += " WHERE " + f
		}
		stmts = append(stmts, s)
	}
	return stmts, nil
}

// PreviewCopyTable returns the statements of a table copy.
func (a *App) PreviewCopyTable(id string, spec CopyTableSpec) ([]string, error) {
	return a.copyStatements(id, spec)
}

// CopyTable copies a table's structure (and optionally data).
func (a *App) CopyTable(id string, spec CopyTableSpec, confirmed bool) ([]query.AdminResult, error) {
	stmts, err := a.copyStatements(id, spec)
	if err != nil {
		return nil, err
	}
	res, err := a.queries.RunAdmin(context.Background(), query.AdminRequest{ConnectionID: id, Statements: stmts, Confirmed: confirmed, DisableFKChecks: true, Timeout: time.Hour})
	if err != nil {
		return nil, wrap(err)
	}
	a.meta.Invalidate(id, spec.TargetDatabase)
	return res, nil
}

// ---- generate test data ---------------------------------------------------------------

// GenerateSpec asks for random rows (HeidiSQL "Generate data").
type GenerateSpec struct {
	Database    string `json:"database"`
	Table       string `json:"table"`
	Rows        int    `json:"rows"`
	NullPercent int    `json:"nullPercent"`
}

func randInt(n int64) int64 {
	if n <= 0 {
		return 0
	}
	v, _ := rand.Int(rand.Reader, big.NewInt(n))
	return v.Int64()
}

var words = strings.Fields("lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod tempor incididunt ut labore et dolore magna aliqua ana bia carlos dora edu fabi gabi hugo iris joao")

func randomValue(c database.ColumnInfo) (any, bool) {
	t := strings.ToLower(c.DataType)
	ct := strings.ToLower(c.ColumnType)
	unsigned := strings.Contains(ct, "unsigned")
	switch t {
	case "tinyint":
		if strings.HasPrefix(ct, "tinyint(1)") {
			return randInt(2), true
		}
		if unsigned {
			return randInt(256), true
		}
		return randInt(256) - 128, true
	case "smallint":
		return randInt(30000), true
	case "mediumint", "int", "integer":
		return randInt(1_000_000), true
	case "bigint":
		return randInt(1_000_000_000), true
	case "decimal", "numeric":
		return fmt.Sprintf("%d.%02d", randInt(100000), randInt(100)), true
	case "float", "double", "real":
		return float64(randInt(1_000_000)) / 100, true
	case "bit":
		return randInt(2), true
	case "year":
		return 1970 + randInt(60), true
	case "date":
		return time.Unix(randInt(1_800_000_000), 0).UTC().Format("2006-01-02"), true
	case "datetime", "timestamp":
		return time.Unix(86400+randInt(1_700_000_000), 0).UTC().Format("2006-01-02 15:04:05"), true
	case "time":
		return fmt.Sprintf("%02d:%02d:%02d", randInt(24), randInt(60), randInt(60)), true
	case "enum", "set":
		vals := enumValues(c.ColumnType)
		if len(vals) == 0 {
			return nil, false
		}
		return vals[randInt(int64(len(vals)))], true
	case "json":
		return fmt.Sprintf(`{"n": %d, "txt": "%s"}`, randInt(1000), words[randInt(int64(len(words)))]), true
	case "char", "varchar", "tinytext", "text", "mediumtext", "longtext":
		max := 255
		if n := typeLen(c.ColumnType); n > 0 {
			max = n
		}
		var b strings.Builder
		for b.Len() < max && b.Len() < 60 {
			w := words[randInt(int64(len(words)))]
			if b.Len()+len(w)+1 > max {
				break
			}
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(w)
		}
		return b.String(), true
	case "binary", "varbinary", "tinyblob", "blob", "mediumblob", "longblob":
		n := 16
		if l := typeLen(c.ColumnType); l > 0 && l < n {
			n = l
		}
		buf := make([]byte, n)
		_, _ = rand.Read(buf)
		return buf, true
	}
	return nil, false // spatial and unknown types
}

func typeLen(ct string) int {
	i := strings.IndexByte(ct, '(')
	j := strings.IndexByte(ct, ')')
	if i < 0 || j < i {
		return 0
	}
	n, _ := strconv.Atoi(ct[i+1 : j])
	return n
}

func enumValues(ct string) []string {
	i := strings.IndexByte(ct, '(')
	j := strings.LastIndexByte(ct, ')')
	if i < 0 || j < i {
		return nil
	}
	var out []string
	for _, p := range strings.Split(ct[i+1:j], "','") {
		out = append(out, strings.ReplaceAll(strings.Trim(p, "'"), "''", "'"))
	}
	return out
}

// StartGenerateData inserts random rows (INSERT IGNORE, so unique keys may
// reduce the count) with foreign key checks disabled.
func (a *App) StartGenerateData(id string, spec GenerateSpec, confirmed bool) (string, error) {
	sess, err := a.session(id)
	if err != nil {
		return "", err
	}
	if spec.Rows <= 0 || spec.Rows > 1_000_000 {
		return "", apperr.New(apperr.CodeValidation, "informe de 1 a 1.000.000 linhas")
	}
	if err := a.writeGuard(id, fmt.Sprintf("Gerar %d linhas em %s.%s", spec.Rows, spec.Database, spec.Table), confirmed); err != nil {
		return "", err
	}
	g, _ := sess.Driver.(database.ObjectSQL)
	ctx0, cancel := metaCtx()
	cols, err := a.meta.Columns(ctx0, id, spec.Database, spec.Table)
	cancel()
	if err != nil {
		return "", wrap(err)
	}
	var use []database.ColumnInfo
	for _, c := range cols {
		ex := strings.ToLower(c.Extra)
		if strings.Contains(ex, "auto_increment") || (strings.Contains(ex, "generated") && !strings.Contains(ex, "default_generated")) {
			continue
		}
		if _, ok := randomValue(c); !ok {
			if !c.Nullable && c.Default == nil {
				return "", apperr.New(apperr.CodeValidation, fmt.Sprintf("não sei gerar valores para a coluna %s (%s)", c.Name, c.ColumnType))
			}
			continue
		}
		use = append(use, c)
	}
	if len(use) == 0 {
		return "", apperr.New(apperr.CodeValidation, "nenhuma coluna para preencher")
	}
	names := make([]string, len(use))
	for i, c := range use {
		names[i] = g.QuoteIdent(c.Name)
	}
	head := "INSERT IGNORE INTO " + g.QuoteIdent(spec.Database) + "." + g.QuoteIdent(spec.Table) + " (" + strings.Join(names, ", ") + ") VALUES "
	tuple := "(" + strings.TrimSuffix(strings.Repeat("?, ", len(use)), ", ") + ")"
	return a.tasks.Start("generate", fmt.Sprintf("Gerar dados em %s", spec.Table), "rows", func(ctx context.Context, p *tasks.Progress) (string, error) {
		var inserted int64
		err := a.queries.WithConn(ctx, id, func(conn *sql.Conn, s *connections.Session) error {
			if _, err := conn.ExecContext(ctx, "SET SESSION FOREIGN_KEY_CHECKS = 0"); err != nil {
				return query.DescribeErr(s, err)
			}
			defer func() { _, _ = conn.ExecContext(context.Background(), "SET SESSION FOREIGN_KEY_CHECKS = 1") }()
			batch := max(1, min(500, 60000/len(use)))
			for done := 0; done < spec.Rows; {
				n := min(batch, spec.Rows-done)
				args := make([]any, 0, n*len(use))
				for r := 0; r < n; r++ {
					for _, c := range use {
						if c.Nullable && int(randInt(100)) < spec.NullPercent {
							args = append(args, nil)
							continue
						}
						v, _ := randomValue(c)
						args = append(args, v)
					}
				}
				res, err := conn.ExecContext(ctx, head+strings.TrimSuffix(strings.Repeat(tuple+", ", n), ", "), args...)
				if err != nil {
					return query.DescribeErr(s, err)
				}
				if k, err := res.RowsAffected(); err == nil {
					inserted += k
				}
				done += n
				p.Set(int64(done), int64(spec.Rows))
			}
			return nil
		})
		a.meta.Invalidate(id, spec.Database)
		return fmt.Sprintf("%d linhas inseridas", inserted), err
	}), nil
}

// ---- server flush ------------------------------------------------------------------------

// Flush runs FLUSH <what> (HOSTS, LOGS, PRIVILEGES, TABLES, STATUS…).
func (a *App) Flush(id, what string, confirmed bool) error {
	_, d, err := a.mysqlDriver(id)
	if err != nil {
		return err
	}
	s, err := d.FlushSQL(what)
	if err != nil {
		return wrap(err)
	}
	res, err := a.adminRun(id, "", []string{s}, confirmed, false, "", false)
	if err != nil {
		return err
	}
	return firstError(res)
}

// ---- user manager ------------------------------------------------------------------------

// ListAccounts lists users and roles.
func (a *App) ListAccounts(id string) ([]mysql.Account, error) {
	sess, d, err := a.mysqlDriver(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := metaCtx()
	defer cancel()
	v, err := d.ListAccounts(ctx, sess.DB, isMaria(sess))
	if err != nil {
		return nil, dbErr(sess, err)
	}
	return v, nil
}

// PrivilegeCatalog lists the privileges per level and auth plugins.
func (a *App) PrivilegeCatalog(id string) (mysql.PrivilegeCatalog, error) {
	sess, d, err := a.mysqlDriver(id)
	if err != nil {
		return mysql.PrivilegeCatalog{}, err
	}
	ctx, cancel := metaCtx()
	defer cancel()
	v, err := d.PrivilegeCatalogFor(ctx, sess.DB, isMaria(sess))
	return v, dbErr(sess, err)
}

// LoadAccount reads an account for the user manager.
func (a *App) LoadAccount(id, user, host string, isRole bool) (mysql.UserDef, error) {
	sess, d, err := a.mysqlDriver(id)
	if err != nil {
		return mysql.UserDef{}, err
	}
	ctx, cancel := metaCtx()
	defer cancel()
	u, err := d.LoadUser(ctx, sess.DB, user, host, isRole, isMaria(sess))
	if err != nil {
		return u, dbErr(sess, err)
	}
	return u, nil
}

func (a *App) accountStatements(id string, u mysql.UserDef) ([]string, error) {
	sess, d, err := a.mysqlDriver(id)
	if err != nil {
		return nil, err
	}
	maria := isMaria(sess)
	var cur *mysql.UserDef
	if u.OrigUser != "" {
		ctx, cancel := metaCtx()
		c, err := d.LoadUser(ctx, sess.DB, u.OrigUser, u.OrigHost, u.IsRole, maria)
		cancel()
		if err != nil {
			return nil, dbErr(sess, err)
		}
		cur = &c
	}
	stmts, err := d.UserSQL(cur, u, maria)
	return stmts, wrap(err)
}

// PreviewAccount returns the statements of a save, passwords masked.
func (a *App) PreviewAccount(id string, u mysql.UserDef) ([]string, error) {
	stmts, err := a.accountStatements(id, u)
	for i := range stmts {
		stmts[i] = mysql.MaskPasswords(stmts[i])
	}
	return stmts, err
}

// SaveAccount creates or changes an account (diffed against the server's
// current definition).
func (a *App) SaveAccount(id string, u mysql.UserDef, confirmed bool) ([]query.AdminResult, error) {
	stmts, err := a.accountStatements(id, u)
	if err != nil {
		return nil, err
	}
	if len(stmts) == 0 {
		return []query.AdminResult{}, nil
	}
	display := make([]string, len(stmts))
	for i, s := range stmts {
		display[i] = mysql.MaskPasswords(s)
	}
	res, err := a.queries.RunAdmin(context.Background(), query.AdminRequest{ConnectionID: id, Statements: stmts, Display: display, Confirmed: confirmed})
	if err != nil {
		return nil, wrap(err)
	}
	for i := range res {
		res[i].SQL = mysql.MaskPasswords(res[i].SQL)
	}
	return res, nil
}

// DropAccount removes a user or role (always asks for confirmation).
func (a *App) DropAccount(id, user, host string, isRole, confirmed bool) error {
	sess, d, err := a.mysqlDriver(id)
	if err != nil {
		return err
	}
	res, err := a.adminRun(id, "", []string{d.DropUserSQL(user, host, isRole, isMaria(sess))}, confirmed, true, "remove a conta e seus privilégios", false)
	if err != nil {
		return err
	}
	return firstError(res)
}

// ChangeOwnPassword changes the password of the connected account
// (HeidiSQL "Change password") and updates the keyring when it is stored.
func (a *App) ChangeOwnPassword(id, newPassword string) error {
	sess, _, err := a.mysqlDriver(id)
	if err != nil {
		return err
	}
	if newPassword == "" {
		return apperr.New(apperr.CodeValidation, "informe a nova senha")
	}
	if sess.Profile.ReadOnly {
		return apperr.New(apperr.CodeReadOnly, "perfil somente leitura")
	}
	ctx, cancel := metaCtx()
	defer cancel()
	stmt := "ALTER USER CURRENT_USER() IDENTIFIED BY " + mysql.New().QuoteString(newPassword)
	if _, err := sess.DB.ExecContext(ctx, stmt); err != nil {
		return dbErr(sess, err)
	}
	return a.conns.PasswordChanged(id, newPassword)
}

// RandomPassword returns a random password (user manager generator).
func (a *App) RandomPassword(length int) string {
	if length < 8 || length > 64 {
		length = 16
	}
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789-_.!@#%"
	b := make([]byte, length)
	for i := range b {
		b[i] = chars[randInt(int64(len(chars)))]
	}
	return string(b)
}

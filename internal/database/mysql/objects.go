package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	gomysql "github.com/go-sql-driver/mysql"

	"anansi/internal/database"
)

var (
	_ database.ObjectBrowser = (*inspector)(nil)
	_ database.ServerAdmin   = (*inspector)(nil)
)

func newInspector(db *sql.DB) *inspector { return &inspector{db: db} }

// flavors remembers whether a pool talks to MariaDB (set by ServerInfo at
// connect time) so catalog queries pick the right variant at the first try
// instead of failing on MySQL and appearing as errors in the SQL log.
var flavors sync.Map // *sql.DB -> bool (true = MariaDB)

func rememberFlavor(db *sql.DB, mariadb bool) { flavors.Store(db, mariadb) }

// Release implements database.Releaser.
func (d *Driver) Release(db *sql.DB) { flavors.Delete(db) }

// mariadb reports the flavor: known=false when it was never detected.
func (i *inspector) mariadb() (isMaria, known bool) {
	v, ok := flavors.Load(i.db)
	if !ok {
		return false, false
	}
	return v.(bool), true
}

// queryer is satisfied by *sql.DB and *sql.Conn.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func ni(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	x := v.Int64
	return &x
}

// isErr reports a server error with one of the given numbers.
func isErr(err error, codes ...uint16) bool {
	var me *gomysql.MySQLError
	if !errors.As(err, &me) {
		return false
	}
	for _, c := range codes {
		if me.Number == c {
			return true
		}
	}
	return false
}

// withFreshStats runs fn on a dedicated connection where MySQL 8 does not
// serve cached information_schema statistics (sizes, row counts, update
// times are cached for 24 h by default). MariaDB has no such cache and
// rejects the variable, which is ignored.
func (i *inspector) withFreshStats(ctx context.Context, fn func(q queryer) error) error {
	conn, err := i.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, setErr := conn.ExecContext(ctx, "SET SESSION information_schema_stats_expiry = 0")
	err = fn(conn)
	if setErr == nil {
		_, _ = conn.ExecContext(context.Background(), "SET SESSION information_schema_stats_expiry = DEFAULT")
	}
	return err
}

// DatabaseSummaries implements database.ObjectBrowser.
func (i *inspector) DatabaseSummaries(ctx context.Context) ([]database.DatabaseSummary, error) {
	var out []database.DatabaseSummary
	err := i.withFreshStats(ctx, func(q queryer) error {
		rows, err := q.QueryContext(ctx, `
SELECT s.SCHEMA_NAME, s.DEFAULT_CHARACTER_SET_NAME, s.DEFAULT_COLLATION_NAME,
       COALESCE(SUM(t.TABLE_TYPE IN ('BASE TABLE', 'SYSTEM VERSIONED', 'SEQUENCE')), 0),
       COALESCE(SUM(t.TABLE_TYPE = 'VIEW'), 0),
       COALESCE(SUM(COALESCE(t.DATA_LENGTH, 0) + COALESCE(t.INDEX_LENGTH, 0)), 0),
       MAX(t.UPDATE_TIME)
FROM information_schema.SCHEMATA s
LEFT JOIN information_schema.TABLES t ON t.TABLE_SCHEMA = s.SCHEMA_NAME
GROUP BY s.SCHEMA_NAME, s.DEFAULT_CHARACTER_SET_NAME, s.DEFAULT_COLLATION_NAME
ORDER BY s.SCHEMA_NAME`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d database.DatabaseSummary
			var upd sql.NullString
			if err := rows.Scan(&d.Name, &d.Charset, &d.Collation, &d.Tables, &d.Views, &d.Size, &upd); err != nil {
				return err
			}
			d.LastUpdate = upd.String
			d.System = systemSchemas[strings.ToLower(d.Name)]
			out = append(out, d)
		}
		return rows.Err()
	})
	return out, err
}

func tableType(t string) string {
	switch strings.ToUpper(t) {
	case "VIEW":
		return "view"
	case "SYSTEM VIEW":
		return "system view"
	case "SEQUENCE":
		return "sequence"
	default:
		return "table"
	}
}

// TableStatus implements database.ObjectBrowser.
func (i *inspector) TableStatus(ctx context.Context, db string) ([]database.TableStatus, error) {
	var out []database.TableStatus
	err := i.withFreshStats(ctx, func(q queryer) error {
		rows, err := q.QueryContext(ctx, `
SELECT TABLE_NAME, TABLE_TYPE, ENGINE, VERSION, ROW_FORMAT, TABLE_ROWS, AVG_ROW_LENGTH, DATA_LENGTH,
       MAX_DATA_LENGTH, INDEX_LENGTH, DATA_FREE, AUTO_INCREMENT, CREATE_TIME, UPDATE_TIME, CHECK_TIME,
       TABLE_COLLATION, CHECKSUM, CREATE_OPTIONS, TABLE_COMMENT
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = ?
ORDER BY TABLE_NAME`, db)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t database.TableStatus
			var typ, engine, rowFormat, autoInc, created, updated, checked, coll, checksum, opts, comment sql.NullString
			var version, nrows, avg, data, maxData, idx, free sql.NullInt64
			if err := rows.Scan(&t.Name, &typ, &engine, &version, &rowFormat, &nrows, &avg, &data, &maxData, &idx, &free,
				&autoInc, &created, &updated, &checked, &coll, &checksum, &opts, &comment); err != nil {
				return err
			}
			t.Type = tableType(typ.String)
			t.Engine, t.RowFormat, t.AutoIncrement = engine.String, rowFormat.String, autoInc.String
			t.CreateTime, t.UpdateTime, t.CheckTime = created.String, updated.String, checked.String
			t.Collation, t.Checksum, t.CreateOptions, t.Comment = coll.String, checksum.String, opts.String, comment.String
			t.Version, t.Rows, t.AvgRowLength, t.DataLength = ni(version), ni(nrows), ni(avg), ni(data)
			t.MaxDataLength, t.IndexLength, t.DataFree = ni(maxData), ni(idx), ni(free)
			if t.Type == "view" && strings.EqualFold(t.Comment, "VIEW") {
				t.Comment = ""
			}
			out = append(out, t)
		}
		return rows.Err()
	})
	return out, err
}

const fkSQL = `
SELECT k.CONSTRAINT_NAME, k.TABLE_SCHEMA, k.TABLE_NAME, k.COLUMN_NAME,
       k.REFERENCED_TABLE_SCHEMA, k.REFERENCED_TABLE_NAME, k.REFERENCED_COLUMN_NAME,
       r.UPDATE_RULE, r.DELETE_RULE
FROM information_schema.KEY_COLUMN_USAGE k
JOIN information_schema.REFERENTIAL_CONSTRAINTS r
  ON r.CONSTRAINT_SCHEMA = k.CONSTRAINT_SCHEMA AND r.CONSTRAINT_NAME = k.CONSTRAINT_NAME AND r.TABLE_NAME = k.TABLE_NAME
WHERE k.REFERENCED_TABLE_NAME IS NOT NULL AND `

func (i *inspector) scanForeignKeys(ctx context.Context, where string, args ...any) ([]database.ForeignKeyInfo, error) {
	rows, err := i.db.QueryContext(ctx, fkSQL+where+" ORDER BY k.TABLE_SCHEMA, k.TABLE_NAME, k.CONSTRAINT_NAME, k.ORDINAL_POSITION", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []database.ForeignKeyInfo
	pos := map[string]int{}
	for rows.Next() {
		var name, schema, table, col, refSchema, refTable, refCol, upd, del sql.NullString
		if err := rows.Scan(&name, &schema, &table, &col, &refSchema, &refTable, &refCol, &upd, &del); err != nil {
			return nil, err
		}
		key := schema.String + "\x00" + table.String + "\x00" + name.String
		p, ok := pos[key]
		if !ok {
			out = append(out, database.ForeignKeyInfo{
				Name: name.String, Database: schema.String, Table: table.String,
				RefDatabase: refSchema.String, RefTable: refTable.String, OnUpdate: upd.String, OnDelete: del.String,
			})
			p = len(out) - 1
			pos[key] = p
		}
		out[p].Columns = append(out[p].Columns, col.String)
		out[p].RefColumns = append(out[p].RefColumns, refCol.String)
	}
	return out, rows.Err()
}

// ForeignKeys implements database.ObjectBrowser.
func (i *inspector) ForeignKeys(ctx context.Context, db, table string) ([]database.ForeignKeyInfo, error) {
	return i.scanForeignKeys(ctx, "k.TABLE_SCHEMA = ? AND k.TABLE_NAME = ?", db, table)
}

// ReferencingKeys implements database.ObjectBrowser.
func (i *inspector) ReferencingKeys(ctx context.Context, db, table string) ([]database.ForeignKeyInfo, error) {
	return i.scanForeignKeys(ctx, "k.REFERENCED_TABLE_SCHEMA = ? AND k.REFERENCED_TABLE_NAME = ?", db, table)
}

// CheckConstraints implements database.ObjectBrowser.
func (i *inspector) CheckConstraints(ctx context.Context, db, table string) ([]database.CheckInfo, error) {
	// MariaDB: CHECK_CONSTRAINTS has TABLE_NAME (constraint names are unique
	// per table). MySQL 8.0.16+: names are unique per schema and the table
	// comes from TABLE_CONSTRAINTS.
	const mysqlSQL = `
SELECT cc.CONSTRAINT_NAME, cc.CHECK_CLAUSE
FROM information_schema.CHECK_CONSTRAINTS cc
JOIN information_schema.TABLE_CONSTRAINTS tc
  ON tc.CONSTRAINT_SCHEMA = cc.CONSTRAINT_SCHEMA AND tc.CONSTRAINT_NAME = cc.CONSTRAINT_NAME
WHERE tc.TABLE_SCHEMA = ? AND tc.TABLE_NAME = ? AND tc.CONSTRAINT_TYPE = 'CHECK'
ORDER BY cc.CONSTRAINT_NAME`
	var rows *sql.Rows
	var err error
	if maria, known := i.mariadb(); known && !maria {
		rows, err = i.db.QueryContext(ctx, mysqlSQL, db, table)
	} else {
		rows, err = i.db.QueryContext(ctx, `
SELECT CONSTRAINT_NAME, CHECK_CLAUSE FROM information_schema.CHECK_CONSTRAINTS
WHERE CONSTRAINT_SCHEMA = ? AND TABLE_NAME = ? ORDER BY CONSTRAINT_NAME`, db, table)
	}
	if isErr(err, 1054) {
		rows, err = i.db.QueryContext(ctx, mysqlSQL, db, table)
	}
	if isErr(err, 1109) { // server without CHECK_CONSTRAINTS
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []database.CheckInfo
	for rows.Next() {
		var c database.CheckInfo
		var clause sql.NullString
		if err := rows.Scan(&c.Name, &clause); err != nil {
			return nil, err
		}
		c.Clause = clause.String
		out = append(out, c)
	}
	return out, rows.Err()
}

// Triggers implements database.ObjectBrowser.
func (i *inspector) Triggers(ctx context.Context, db string) ([]database.TriggerInfo, error) {
	rows, err := i.db.QueryContext(ctx, `
SELECT TRIGGER_NAME, EVENT_OBJECT_TABLE, ACTION_TIMING, EVENT_MANIPULATION, ACTION_STATEMENT, DEFINER,
       CREATED, SQL_MODE, ACTION_ORDER
FROM information_schema.TRIGGERS
WHERE TRIGGER_SCHEMA = ?
ORDER BY TRIGGER_NAME`, db)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []database.TriggerInfo
	for rows.Next() {
		var t database.TriggerInfo
		var stmt, definer, created, mode sql.NullString
		var order sql.NullInt64
		if err := rows.Scan(&t.Name, &t.Table, &t.Timing, &t.Event, &stmt, &definer, &created, &mode, &order); err != nil {
			return nil, err
		}
		t.Statement, t.Definer, t.Created, t.SQLMode, t.Order = stmt.String, definer.String, created.String, mode.String, int(order.Int64)
		out = append(out, t)
	}
	return out, rows.Err()
}

// Events implements database.ObjectBrowser.
func (i *inspector) Events(ctx context.Context, db string) ([]database.EventInfo, error) {
	rows, err := i.db.QueryContext(ctx, `
SELECT EVENT_NAME, DEFINER, EVENT_TYPE, EXECUTE_AT, INTERVAL_VALUE, INTERVAL_FIELD, STARTS, ENDS, STATUS,
       ON_COMPLETION, EVENT_DEFINITION, EVENT_COMMENT, LAST_EXECUTED, CREATED
FROM information_schema.EVENTS
WHERE EVENT_SCHEMA = ?
ORDER BY EVENT_NAME`, db)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []database.EventInfo
	for rows.Next() {
		var e database.EventInfo
		var definer, typ, at, iv, ifld, starts, ends, status, onc, body, comment, last, created sql.NullString
		if err := rows.Scan(&e.Name, &definer, &typ, &at, &iv, &ifld, &starts, &ends, &status, &onc, &body, &comment, &last, &created); err != nil {
			return nil, err
		}
		e.Definer, e.Type, e.ExecuteAt, e.IntervalValue, e.IntervalField = definer.String, typ.String, at.String, iv.String, ifld.String
		e.Starts, e.Ends, e.Status, e.OnCompletion, e.Body = starts.String, ends.String, status.String, onc.String, body.String
		e.Comment, e.LastExecuted, e.Created = comment.String, last.String, created.String
		out = append(out, e)
	}
	return out, rows.Err()
}

// RoutineDetails implements database.ObjectBrowser.
func (i *inspector) RoutineDetails(ctx context.Context, db string) ([]database.RoutineDetail, error) {
	rows, err := i.db.QueryContext(ctx, `
SELECT ROUTINE_NAME, ROUTINE_TYPE, DTD_IDENTIFIER, ROUTINE_DEFINITION, IS_DETERMINISTIC, SQL_DATA_ACCESS,
       SECURITY_TYPE, DEFINER, ROUTINE_COMMENT, CREATED, LAST_ALTERED
FROM information_schema.ROUTINES
WHERE ROUTINE_SCHEMA = ?
ORDER BY ROUTINE_NAME`, db)
	if err != nil {
		return nil, err
	}
	var out []database.RoutineDetail
	pos := map[string]int{}
	for rows.Next() {
		var r database.RoutineDetail
		var ret, body, det, access, sec, definer, comment, created, modified sql.NullString
		if err := rows.Scan(&r.Name, &r.Type, &ret, &body, &det, &access, &sec, &definer, &comment, &created, &modified); err != nil {
			rows.Close()
			return nil, err
		}
		r.Returns, r.Body, r.Deterministic = ret.String, body.String, strings.EqualFold(det.String, "YES")
		r.DataAccess, r.Security, r.Definer, r.Comment = access.String, sec.String, definer.String, comment.String
		r.Created, r.Modified = created.String, modified.String
		r.Params = []database.RoutineParam{}
		out = append(out, r)
		pos[strings.ToUpper(r.Type)+"\x00"+r.Name] = len(out) - 1
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	prows, err := i.db.QueryContext(ctx, `
SELECT SPECIFIC_NAME, ROUTINE_TYPE, PARAMETER_MODE, PARAMETER_NAME, DTD_IDENTIFIER
FROM information_schema.PARAMETERS
WHERE SPECIFIC_SCHEMA = ? AND ORDINAL_POSITION > 0
ORDER BY SPECIFIC_NAME, ORDINAL_POSITION`, db)
	if err != nil {
		return out, nil // parameters are optional details
	}
	defer prows.Close()
	for prows.Next() {
		var name, typ, mode, pname, dtd sql.NullString
		if err := prows.Scan(&name, &typ, &mode, &pname, &dtd); err != nil {
			return out, nil
		}
		if p, ok := pos[strings.ToUpper(typ.String)+"\x00"+name.String]; ok {
			out[p].Params = append(out[p].Params, database.RoutineParam{Mode: mode.String, Name: pname.String, Type: dtd.String})
		}
	}
	return out, nil
}

var showCreate = map[database.ObjectKind]string{
	database.ObjDatabase:  "DATABASE",
	database.ObjTable:     "TABLE",
	database.ObjView:      "VIEW",
	database.ObjProcedure: "PROCEDURE",
	database.ObjFunction:  "FUNCTION",
	database.ObjTrigger:   "TRIGGER",
	database.ObjEvent:     "EVENT",
}

// CreateStatement implements database.ObjectBrowser.
func (i *inspector) CreateStatement(ctx context.Context, db string, kind database.ObjectKind, name string) (string, error) {
	word, ok := showCreate[kind]
	if !ok {
		return "", fmt.Errorf("tipo de objeto inválido: %q", kind)
	}
	target := QuoteIdent(name)
	if kind != database.ObjDatabase {
		target = QuoteIdent(db) + "." + target
	}
	rows, err := i.db.QueryContext(ctx, "SHOW CREATE "+word+" "+target)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return "", err
	}
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return "", err
		}
		return "", fmt.Errorf("objeto não encontrado: %s", name)
	}
	vals := make([]sql.NullString, len(cols))
	ptrs := make([]any, len(cols))
	for k := range vals {
		ptrs[k] = &vals[k]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return "", err
	}
	for k, c := range cols {
		lc := strings.ToLower(c)
		if strings.HasPrefix(lc, "create ") || lc == "sql original statement" {
			if !vals[k].Valid {
				return "", errors.New("o servidor não retornou o código do objeto (privilégio insuficiente?)")
			}
			return vals[k].String, nil
		}
	}
	return "", errors.New("resposta inesperada de SHOW CREATE")
}

// ---- server administration --------------------------------------------------

// Variables implements database.ServerAdmin.
func (i *inspector) Variables(ctx context.Context) ([]database.ServerVariable, error) {
	byName := map[string]*database.ServerVariable{}
	for _, scope := range []string{"SESSION", "GLOBAL"} {
		rows, err := i.db.QueryContext(ctx, "SHOW "+scope+" VARIABLES")
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var name string
			var value sql.NullString
			if err := rows.Scan(&name, &value); err != nil {
				rows.Close()
				return nil, err
			}
			v := byName[name]
			if v == nil {
				v = &database.ServerVariable{Name: name}
				byName[name] = v
			}
			s := value.String
			if scope == "SESSION" {
				v.Session = &s
			} else {
				v.Global = &s
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	out := make([]database.ServerVariable, 0, len(byName))
	for _, v := range byName {
		out = append(out, *v)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out, nil
}

// Status implements database.ServerAdmin.
func (i *inspector) Status(ctx context.Context) ([]database.StatusValue, error) {
	rows, err := i.db.QueryContext(ctx, "SHOW GLOBAL STATUS")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []database.StatusValue
	for rows.Next() {
		var s database.StatusValue
		var v sql.NullString
		if err := rows.Scan(&s.Name, &v); err != nil {
			return nil, err
		}
		s.Value = v.String
		out = append(out, s)
	}
	return out, rows.Err()
}

// Processes implements database.ServerAdmin.
func (i *inspector) Processes(ctx context.Context) ([]database.Process, error) {
	const base = `SELECT ID, USER, HOST, DB, COMMAND, TIME, STATE, LEFT(INFO, 8192)%s FROM information_schema.PROCESSLIST ORDER BY ID`
	maria, known := i.mariadb()
	withProgress := maria || !known
	query := fmt.Sprintf(base, "")
	if withProgress {
		query = fmt.Sprintf(base, ", PROGRESS")
	}
	rows, err := i.db.QueryContext(ctx, query)
	if withProgress && isErr(err, 1054) {
		withProgress = false
		rows, err = i.db.QueryContext(ctx, fmt.Sprintf(base, ""))
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []database.Process
	for rows.Next() {
		var p database.Process
		var user, host, db, cmd, state, info sql.NullString
		var t sql.NullInt64
		var progress sql.NullFloat64
		dest := []any{&p.ID, &user, &host, &db, &cmd, &t, &state, &info}
		if withProgress {
			dest = append(dest, &progress)
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		p.User, p.Host, p.DB, p.Command, p.Time, p.State, p.Info = user.String, host.String, db.String, cmd.String, t.Int64, state.String, info.String
		if progress.Valid && progress.Float64 > 0 {
			v := progress.Float64
			p.Progress = &v
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

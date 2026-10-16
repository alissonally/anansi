// Package dump writes MySQL/MariaDB databases as SQL (HeidiSQL's "Export
// database as SQL"): structure, data, views, routines, events and triggers,
// to a file, a gzip file, a directory (one file per object) or straight into
// another database or connection.
//
// The dump is read inside a consistent snapshot (REPEATABLE READ + START
// TRANSACTION WITH CONSISTENT SNAPSHOT, like mysqldump --single-transaction)
// with TIME_ZONE '+00:00', and written with the same session header as
// mysqldump/HeidiSQL so that it can be replayed by any client.
package dump

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	"anansi/internal/database"
	"anansi/internal/gridexport"
	"anansi/internal/query"
)

// Data modes.
const (
	DataNone         = "none"
	DataInsert       = "insert"
	DataInsertIgnore = "insert-ignore"
	DataReplace      = "replace"
	DataDeleteInsert = "delete-insert"
)

// Selection picks objects of one database. Empty lists mean "all".
type Selection struct {
	Database   string   `json:"database"`
	Tables     []string `json:"tables"` // tables and views
	Routines   bool     `json:"routines"`
	Events     bool     `json:"events"`
	Triggers   bool     `json:"triggers"`
	AllObjects bool     `json:"allObjects"`
}

// Options configure a dump.
type Options struct {
	Selections          []Selection `json:"selections"`
	DropDatabase        bool        `json:"dropDatabase"`
	CreateDatabase      bool        `json:"createDatabase"`
	DropObjects         bool        `json:"dropObjects"`
	CreateObjects       bool        `json:"createObjects"`
	Data                string      `json:"data"`
	MaxInsertKB         int         `json:"maxInsertKB"` // 0 = one row per INSERT
	Comments            bool        `json:"comments"`
	WrapTransactions    bool        `json:"wrapTransactions"`
	RemoveAutoIncrement bool        `json:"removeAutoIncrement"`
	RemoveDefiner       bool        `json:"removeDefiner"`
	// TargetDatabase renames the database in the output (exec targets and
	// "copy to another database").
	TargetDatabase string `json:"targetDatabase"`
}

// Stmt is one statement of the dump. Compound statements (routines,
// triggers, events) contain semicolons and need a DELIMITER in files.
type Stmt struct {
	SQL      string
	Compound bool
	// Object is "<kind>-<name>" for directory sinks ("" = database level).
	Object string
}

// Sink receives the dump.
type Sink interface {
	// Comment writes a comment line (ignored by exec sinks).
	Comment(text string) error
	Exec(st Stmt) error
	// Database announces the database whose objects follow.
	Database(name string) error
	Close() error
}

// Progress reports advancement.
type Progress interface {
	Set(done, total int64)
	Add(n int64)
	Message(s string)
	Warn(msg string)
}

var (
	definerRe = regexp.MustCompile("(?i)\\sDEFINER\\s*=\\s*(`[^`]*`|'[^']*'|[^\\s@]+)@(`[^`]*`|'[^']*'|[^\\s]+)")
	autoIncRe = regexp.MustCompile(`(?i)\s+AUTO_INCREMENT=\d+`)
)

// header are the session settings at the top of every dump file.
func header() []string {
	return []string{
		"/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */",
		"/*!40101 SET NAMES utf8mb4 */",
		"/*!40103 SET @OLD_TIME_ZONE=@@TIME_ZONE */",
		"/*!40103 SET TIME_ZONE='+00:00' */",
		"/*!40014 SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0 */",
		"/*!40101 SET @OLD_SQL_MODE=@@SQL_MODE, SQL_MODE='NO_AUTO_VALUE_ON_ZERO' */",
		"/*!40111 SET @OLD_SQL_NOTES=@@SQL_NOTES, SQL_NOTES=0 */",
	}
}

func footer() []string {
	return []string{
		"/*!40101 SET SQL_MODE=IFNULL(@OLD_SQL_MODE, '') */",
		"/*!40014 SET FOREIGN_KEY_CHECKS=IFNULL(@OLD_FOREIGN_KEY_CHECKS, 1) */",
		"/*!40103 SET TIME_ZONE=IFNULL(@OLD_TIME_ZONE, 'SYSTEM') */",
		"/*!40101 SET CHARACTER_SET_CLIENT=@OLD_CHARACTER_SET_CLIENT */",
		"/*!40111 SET SQL_NOTES=IFNULL(@OLD_SQL_NOTES, 1) */",
	}
}

// Dumper reads from a source connection.
type Dumper struct {
	Conn     *sql.Conn
	Browser  database.ObjectBrowser
	Opts     Options
	Sink     Sink
	Progress Progress
	// Info describes the source in comments.
	Server string
}

func (d *Dumper) cleanCreate(s string) string {
	if d.Opts.RemoveDefiner {
		s = definerRe.ReplaceAllString(s, "")
	}
	return s
}

func (d *Dumper) comment(format string, a ...any) error {
	if !d.Opts.Comments {
		return nil
	}
	return d.Sink.Comment(fmt.Sprintf(format, a...))
}

// Run writes the dump.
func (d *Dumper) Run(ctx context.Context) error {
	if d.Opts.Data == "" {
		d.Opts.Data = DataInsert
	}
	for _, s := range []string{"SET SESSION TIME_ZONE = '+00:00'", "SET SESSION TRANSACTION ISOLATION LEVEL REPEATABLE READ", "START TRANSACTION WITH CONSISTENT SNAPSHOT"} {
		if _, err := d.Conn.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	defer func() { _, _ = d.Conn.ExecContext(context.Background(), "COMMIT") }()

	if err := d.Sink.Comment(fmt.Sprintf("Anansi — dump gerado em %s", time.Now().Format("2006-01-02 15:04:05 -0700"))); err != nil {
		return err
	}
	if d.Server != "" {
		if err := d.Sink.Comment("Servidor: " + d.Server); err != nil {
			return err
		}
	}
	for _, h := range header() {
		if err := d.Sink.Exec(Stmt{SQL: h}); err != nil {
			return err
		}
	}
	// Total rows (estimates) for the progress bar.
	var total int64
	type plan struct {
		sel    Selection
		tables []database.TableStatus
	}
	var plans []plan
	for _, sel := range d.Opts.Selections {
		st, err := d.Browser.TableStatus(ctx, sel.Database)
		if err != nil {
			return err
		}
		want := map[string]bool{}
		for _, t := range sel.Tables {
			want[t] = true
		}
		var tables []database.TableStatus
		for _, t := range st {
			if sel.AllObjects || want[t.Name] {
				tables = append(tables, t)
				if t.Rows != nil && t.Type == "table" && d.Opts.Data != DataNone {
					total += *t.Rows
				}
			}
		}
		plans = append(plans, plan{sel, tables})
	}
	d.Progress.Set(0, total)
	for _, p := range plans {
		if err := d.database(ctx, p.sel, p.tables); err != nil {
			return err
		}
	}
	for _, f := range footer() {
		if err := d.Sink.Exec(Stmt{SQL: f}); err != nil {
			return err
		}
	}
	return d.Sink.Close()
}

func (d *Dumper) target(sel Selection) string {
	if d.Opts.TargetDatabase != "" && len(d.Opts.Selections) == 1 {
		return d.Opts.TargetDatabase
	}
	return sel.Database
}

func (d *Dumper) database(ctx context.Context, sel Selection, tables []database.TableStatus) error {
	src := sel.Database
	dst := d.target(sel)
	if err := d.Sink.Database(dst); err != nil {
		return err
	}
	_ = d.comment("Database %s", dst)
	if d.Opts.DropDatabase {
		if err := d.Sink.Exec(Stmt{SQL: "DROP DATABASE IF EXISTS " + gridexport.QuoteIdent(dst)}); err != nil {
			return err
		}
	}
	if d.Opts.CreateDatabase {
		create, err := d.Browser.CreateStatement(ctx, "", database.ObjDatabase, src)
		if err != nil {
			return err
		}
		create = strings.Replace(create, "CREATE DATABASE "+gridexport.QuoteIdent(src), "CREATE DATABASE IF NOT EXISTS "+gridexport.QuoteIdent(dst), 1)
		create = strings.Replace(create, "IF NOT EXISTS /*!32312 IF NOT EXISTS*/", "IF NOT EXISTS", 1)
		if err := d.Sink.Exec(Stmt{SQL: create}); err != nil {
			return err
		}
	}
	if err := d.Sink.Exec(Stmt{SQL: "USE " + gridexport.QuoteIdent(dst)}); err != nil {
		return err
	}

	var views []database.TableStatus
	for _, t := range tables {
		if err := ctx.Err(); err != nil {
			return err
		}
		if t.Type == "view" {
			views = append(views, t)
			if err := d.viewPlaceholder(ctx, src, t.Name); err != nil {
				return err
			}
			continue
		}
		if t.Type != "table" && t.Type != "sequence" {
			continue
		}
		if err := d.table(ctx, src, t); err != nil {
			return err
		}
	}
	for _, v := range views {
		if err := d.view(ctx, src, v.Name); err != nil {
			return err
		}
	}
	if sel.Routines || sel.AllObjects {
		if err := d.routines(ctx, src); err != nil {
			return err
		}
	}
	if sel.Events || sel.AllObjects {
		if err := d.events(ctx, src); err != nil {
			return err
		}
	}
	if sel.Triggers || sel.AllObjects {
		if err := d.triggers(ctx, src, tables); err != nil {
			return err
		}
	}
	return nil
}

// columnsOf returns the insertable columns (generated ones are skipped).
func (d *Dumper) columnsOf(ctx context.Context, db, table string) ([]string, error) {
	rows, err := d.Conn.QueryContext(ctx, "SELECT COLUMN_NAME, EXTRA FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? ORDER BY ORDINAL_POSITION", db, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name, extra string
		if err := rows.Scan(&name, &extra); err != nil {
			return nil, err
		}
		if strings.Contains(strings.ToUpper(extra), "GENERATED") && !strings.Contains(strings.ToUpper(extra), "DEFAULT_GENERATED") {
			continue
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

func (d *Dumper) table(ctx context.Context, db string, t database.TableStatus) error {
	obj := "table-" + t.Name
	q := gridexport.QuoteIdent(t.Name)
	d.Progress.Message("Tabela " + db + "." + t.Name)
	if d.Opts.CreateObjects || d.Opts.DropObjects {
		_ = d.comment("Estrutura da tabela %s", t.Name)
	}
	if d.Opts.DropObjects {
		if err := d.Sink.Exec(Stmt{SQL: "DROP TABLE IF EXISTS " + q, Object: obj}); err != nil {
			return err
		}
	}
	if d.Opts.CreateObjects {
		create, err := d.Browser.CreateStatement(ctx, db, database.ObjTable, t.Name)
		if err != nil {
			return err
		}
		if !d.Opts.DropObjects {
			create = strings.Replace(create, "CREATE TABLE ", "CREATE TABLE IF NOT EXISTS ", 1)
		}
		if d.Opts.RemoveAutoIncrement {
			create = autoIncRe.ReplaceAllString(create, "")
		}
		if err := d.Sink.Exec(Stmt{SQL: create, Object: obj}); err != nil {
			return err
		}
	}
	if d.Opts.Data == DataNone || strings.EqualFold(t.Engine, "MRG_MYISAM") || strings.EqualFold(t.Engine, "FEDERATED") || t.Type == "sequence" {
		return nil
	}
	cols, err := d.columnsOf(ctx, db, t.Name)
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	qcols := make([]string, len(cols))
	for i, c := range cols {
		qcols[i] = gridexport.QuoteIdent(c)
	}
	_ = d.comment("Dados da tabela %s", t.Name)
	if d.Opts.Data == DataDeleteInsert {
		if err := d.Sink.Exec(Stmt{SQL: "DELETE FROM " + q, Object: obj}); err != nil {
			return err
		}
	}
	verb := "INSERT INTO"
	switch d.Opts.Data {
	case DataInsertIgnore:
		verb = "INSERT IGNORE INTO"
	case DataReplace:
		verb = "REPLACE INTO"
	}
	prefix := verb + " " + q + " (" + strings.Join(qcols, ", ") + ") VALUES"
	maxBytes := d.Opts.MaxInsertKB * 1024 * 9 / 10
	var batch []string
	size := 0
	wrapped := false
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if d.Opts.WrapTransactions && !wrapped {
			if err := d.Sink.Exec(Stmt{SQL: "START TRANSACTION", Object: obj}); err != nil {
				return err
			}
			wrapped = true
		}
		var s string
		if len(batch) == 1 {
			s = prefix + " " + batch[0]
		} else {
			s = prefix + "\n\t" + strings.Join(batch, ",\n\t")
		}
		batch, size = batch[:0], 0
		return d.Sink.Exec(Stmt{SQL: s, Object: obj})
	}
	var ecols []gridexport.Column
	n := int64(0)
	sel := "SELECT " + strings.Join(qcols, ", ") + " FROM " + gridexport.QuoteIdent(db) + "." + q
	err = query.StreamRows(ctx, d.Conn, sel, nil, func(cs []query.Column) error {
		ecols = make([]gridexport.Column, len(cs))
		for i, c := range cs {
			ecols[i] = gridexport.Column{Name: c.Name, Numeric: c.Numeric, Binary: c.Binary}
		}
		return nil
	}, func(row []any) error {
		lits := make([]string, len(row))
		for i, v := range row {
			lits[i] = gridexport.Literal(v, ecols[i])
		}
		tuple := "(" + strings.Join(lits, ",") + ")"
		batch = append(batch, tuple)
		size += len(tuple) + 3
		n++
		if n%500 == 0 {
			d.Progress.Add(500)
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if maxBytes <= 0 || size >= maxBytes {
			return flush()
		}
		return nil
	})
	if err != nil {
		return err
	}
	d.Progress.Add(n % 500)
	if err := flush(); err != nil {
		return err
	}
	if wrapped {
		return d.Sink.Exec(Stmt{SQL: "COMMIT", Object: obj})
	}
	return nil
}

// viewPlaceholder creates a table with the view's columns so other views
// depending on it can be created in any order (mysqldump's approach).
func (d *Dumper) viewPlaceholder(ctx context.Context, db, name string) error {
	if !d.Opts.CreateObjects {
		return nil
	}
	cols, err := d.columnsOf(ctx, db, name)
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	defs := make([]string, len(cols))
	for i, c := range cols {
		defs[i] = gridexport.QuoteIdent(c) + " TINYINT NOT NULL"
	}
	obj := "view-" + name
	_ = d.comment("Estrutura temporária da view %s", name)
	if d.Opts.DropObjects {
		if err := d.Sink.Exec(Stmt{SQL: "DROP VIEW IF EXISTS " + gridexport.QuoteIdent(name), Object: obj}); err != nil {
			return err
		}
	}
	return d.Sink.Exec(Stmt{SQL: "CREATE TABLE IF NOT EXISTS " + gridexport.QuoteIdent(name) + " (\n\t" + strings.Join(defs, ",\n\t") + "\n) ENGINE=MyISAM", Object: obj})
}

func (d *Dumper) view(ctx context.Context, db, name string) error {
	if !d.Opts.CreateObjects {
		return nil
	}
	create, err := d.Browser.CreateStatement(ctx, db, database.ObjView, name)
	if err != nil {
		return err
	}
	obj := "view-" + name
	_ = d.comment("View %s", name)
	if err := d.Sink.Exec(Stmt{SQL: "DROP TABLE IF EXISTS " + gridexport.QuoteIdent(name), Object: obj}); err != nil {
		return err
	}
	// The server qualifies the view and its tables with the source
	// database: make the name follow USE and, when copying to another
	// database, point the references at the copy.
	q := gridexport.QuoteIdent(db) + "."
	create = strings.Replace(create, "VIEW "+q+gridexport.QuoteIdent(name), "VIEW "+gridexport.QuoteIdent(name), 1)
	if dst := d.Opts.TargetDatabase; dst != "" && dst != db {
		create = strings.ReplaceAll(create, q, gridexport.QuoteIdent(dst)+".")
	}
	return d.Sink.Exec(Stmt{SQL: d.cleanCreate(create), Object: obj})
}

func (d *Dumper) routines(ctx context.Context, db string) error {
	list, err := d.Browser.RoutineDetails(ctx, db)
	if err != nil {
		return err
	}
	for _, r := range list {
		kind := database.ObjProcedure
		if strings.EqualFold(r.Type, "FUNCTION") {
			kind = database.ObjFunction
		}
		obj := string(kind) + "-" + r.Name
		d.Progress.Message("Rotina " + r.Name)
		if d.Opts.DropObjects {
			if err := d.Sink.Exec(Stmt{SQL: "DROP " + strings.ToUpper(string(kind)) + " IF EXISTS " + gridexport.QuoteIdent(r.Name), Object: obj}); err != nil {
				return err
			}
		}
		if !d.Opts.CreateObjects {
			continue
		}
		create, err := d.Browser.CreateStatement(ctx, db, kind, r.Name)
		if err != nil {
			d.Progress.Warn(fmt.Sprintf("%s %s: %v", kind, r.Name, err))
			continue
		}
		_ = d.comment("%s %s", strings.ToUpper(string(kind)), r.Name)
		if err := d.Sink.Exec(Stmt{SQL: d.cleanCreate(create), Compound: true, Object: obj}); err != nil {
			return err
		}
	}
	return nil
}

func (d *Dumper) events(ctx context.Context, db string) error {
	list, err := d.Browser.Events(ctx, db)
	if err != nil {
		return err
	}
	for _, e := range list {
		obj := "event-" + e.Name
		if d.Opts.DropObjects {
			if err := d.Sink.Exec(Stmt{SQL: "DROP EVENT IF EXISTS " + gridexport.QuoteIdent(e.Name), Object: obj}); err != nil {
				return err
			}
		}
		if !d.Opts.CreateObjects {
			continue
		}
		create, err := d.Browser.CreateStatement(ctx, db, database.ObjEvent, e.Name)
		if err != nil {
			d.Progress.Warn(fmt.Sprintf("evento %s: %v", e.Name, err))
			continue
		}
		_ = d.comment("Evento %s", e.Name)
		if err := d.Sink.Exec(Stmt{SQL: d.cleanCreate(create), Compound: true, Object: obj}); err != nil {
			return err
		}
	}
	return nil
}

func (d *Dumper) triggers(ctx context.Context, db string, tables []database.TableStatus) error {
	if !d.Opts.CreateObjects {
		return nil
	}
	list, err := d.Browser.Triggers(ctx, db)
	if err != nil {
		return err
	}
	inDump := map[string]bool{}
	for _, t := range tables {
		inDump[t.Name] = true
	}
	for _, t := range list {
		if !inDump[t.Table] {
			continue
		}
		// Rebuilt from the catalog: MariaDB's SHOW CREATE TRIGGER returns the
		// original text, which may qualify the trigger and table with the
		// source database.
		def := ""
		if !d.Opts.RemoveDefiner && t.Definer != "" {
			if i := strings.LastIndexByte(t.Definer, '@'); i > 0 {
				def = "DEFINER=" + gridexport.QuoteIdent(t.Definer[:i]) + "@" + gridexport.QuoteIdent(t.Definer[i+1:]) + " "
			}
		}
		create := fmt.Sprintf("CREATE %sTRIGGER %s %s %s ON %s FOR EACH ROW %s", def, gridexport.QuoteIdent(t.Name), t.Timing, t.Event, gridexport.QuoteIdent(t.Table), t.Statement)
		obj := "trigger-" + t.Name
		_ = d.comment("Trigger %s", t.Name)
		if d.Opts.DropObjects {
			if err := d.Sink.Exec(Stmt{SQL: "DROP TRIGGER IF EXISTS " + gridexport.QuoteIdent(t.Name), Object: obj}); err != nil {
				return err
			}
		}
		// Triggers keep the sql_mode they were created with.
		if err := d.Sink.Exec(Stmt{SQL: "SET @OLDTMP_SQL_MODE=@@SQL_MODE, SQL_MODE=" + gridexport.QuoteString(t.SQLMode), Object: obj}); err != nil {
			return err
		}
		if err := d.Sink.Exec(Stmt{SQL: create, Compound: true, Object: obj}); err != nil {
			return err
		}
		if err := d.Sink.Exec(Stmt{SQL: "SET SQL_MODE=@OLDTMP_SQL_MODE", Object: obj}); err != nil {
			return err
		}
	}
	return nil
}

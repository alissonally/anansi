package api

import (
	"context"
	"sort"
	"strings"
	"time"

	"anansi/internal/apperr"
	"anansi/internal/connections"
	"anansi/internal/database"
	"anansi/internal/query"
)

// SchemaDiffItem is one object of a structure comparison.
type SchemaDiffItem struct {
	Kind string `json:"kind"` // table | view | procedure | function | trigger | event
	Name string `json:"name"`
	// Status: "create" (only in the source), "drop" (only in the target),
	// "alter" (different) or "same".
	Status string `json:"status"`
	// Statements bring the target in line with the source.
	Statements []string `json:"statements"`
	// SourceSQL/TargetSQL are both sides rendered by the same generator, for
	// a side-by-side view ("" when the object is missing on that side).
	SourceSQL string `json:"sourceSQL"`
	TargetSQL string `json:"targetSQL"`
}

// SchemaDiff is the result of CompareSchemas.
type SchemaDiff struct {
	Items    []SchemaDiffItem `json:"items"`
	Warnings []string         `json:"warnings"`
}

type compareSide struct {
	sess *connections.Session
	e    database.ObjectEditor
	b    database.ObjectBrowser
	g    database.ObjectSQL
	db   string
}

func (a *App) compareSide(id, db string) (*compareSide, error) {
	sess, e, err := a.objectEditor(id)
	if err != nil {
		return nil, err
	}
	_, b, err := a.browser(id)
	if err != nil {
		return nil, err
	}
	g, ok := sess.Driver.(database.ObjectSQL)
	if !ok {
		return nil, apperr.New(apperr.CodeValidation, "comparação indisponível para este banco")
	}
	if strings.TrimSpace(db) == "" {
		return nil, apperr.New(apperr.CodeValidation, "escolha os databases")
	}
	return &compareSide{sess: sess, e: e, b: b, g: g, db: db}, nil
}

type objKey struct{ kind, name string }

// objects lists every comparable object of the side's database.
func (s *compareSide) objects(ctx context.Context, a *App, id string) (map[objKey]bool, error) {
	out := map[objKey]bool{}
	tables, err := a.meta.Tables(ctx, id, s.db)
	if err != nil {
		return nil, err
	}
	for _, t := range tables {
		switch t.Type {
		case "table":
			out[objKey{"table", t.Name}] = true
		case "view":
			out[objKey{"view", t.Name}] = true
		}
	}
	routines, err := s.b.RoutineDetails(ctx, s.db)
	if err != nil {
		return nil, err
	}
	for _, r := range routines {
		out[objKey{strings.ToLower(r.Type), r.Name}] = true
	}
	trs, err := s.b.Triggers(ctx, s.db)
	if err != nil {
		return nil, err
	}
	for _, t := range trs {
		out[objKey{"trigger", t.Name}] = true
	}
	evs, err := s.b.Events(ctx, s.db)
	if err != nil {
		return nil, err
	}
	for _, e := range evs {
		out[objKey{"event", e.Name}] = true
	}
	return out, nil
}

var kindOrder = map[string]int{"table": 0, "view": 1, "function": 2, "procedure": 3, "trigger": 4, "event": 5}

// CompareSchemas compares the structure of two databases (same or different
// connections) and returns, per object, the statements that make the target
// equal to the source (HeidiSQL "Synchronize": structure only).
func (a *App) CompareSchemas(sourceID, sourceDB, targetID, targetDB string) (SchemaDiff, error) {
	src, err := a.compareSide(sourceID, sourceDB)
	if err != nil {
		return SchemaDiff{}, err
	}
	tgt, err := a.compareSide(targetID, targetDB)
	if err != nil {
		return SchemaDiff{}, err
	}
	if sourceID == targetID && sourceDB == targetDB {
		return SchemaDiff{}, apperr.New(apperr.CodeValidation, "origem e destino são o mesmo database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	a.meta.Invalidate(sourceID, sourceDB)
	a.meta.Invalidate(targetID, targetDB)
	so, err := src.objects(ctx, a, sourceID)
	if err != nil {
		return SchemaDiff{}, dbErr(src.sess, err)
	}
	to, err := tgt.objects(ctx, a, targetID)
	if err != nil {
		return SchemaDiff{}, dbErr(tgt.sess, err)
	}
	keys := map[objKey]bool{}
	for k := range so {
		keys[k] = true
	}
	for k := range to {
		keys[k] = true
	}
	list := make([]objKey, 0, len(keys))
	for k := range keys {
		list = append(list, k)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].kind != list[j].kind {
			return kindOrder[list[i].kind] < kindOrder[list[j].kind]
		}
		return list[i].name < list[j].name
	})
	c := comparer{src: src, tgt: tgt, maria: isMaria(tgt.sess)}
	var diff SchemaDiff
	for _, k := range list {
		item, err := c.compare(ctx, k, so[k], to[k])
		if err != nil {
			diff.Warnings = append(diff.Warnings, k.kind+" "+k.name+": "+err.Error())
			continue
		}
		diff.Items = append(diff.Items, item)
	}
	return diff, nil
}

type comparer struct {
	src, tgt *compareSide
	maria    bool
}

// retarget points references to the source database at the target one.
func (c *comparer) retarget(s string) string {
	if c.src.db == c.tgt.db {
		return s
	}
	return strings.ReplaceAll(s, "`"+c.src.db+"`.", "`"+c.tgt.db+"`.")
}

func (c *comparer) compare(ctx context.Context, k objKey, inSrc, inTgt bool) (SchemaDiffItem, error) {
	item := SchemaDiffItem{Kind: k.kind, Name: k.name}
	if k.kind == "table" {
		return c.compareTable(ctx, item, inSrc, inTgt)
	}
	render := func(side *compareSide, fromSource bool) (string, error) {
		var create string
		switch k.kind {
		case "view":
			v, err := side.e.LoadViewDef(ctx, side.sess.DB, side.db, k.name)
			if err != nil {
				return "", err
			}
			v.Database, v.OrigName, v.Definer = c.tgt.db, "", ""
			if fromSource {
				v.Select = c.retarget(v.Select)
			}
			stmts, err := c.tgt.e.ViewSQL(v)
			if err != nil {
				return "", err
			}
			create = strings.Join(stmts, ";\n")
		case "procedure", "function":
			r, err := side.e.LoadRoutineDef(ctx, side.sess.DB, side.db, strings.ToUpper(k.kind), k.name)
			if err != nil {
				return "", err
			}
			r.Database, r.OrigName, r.OrigType, r.Definer = c.tgt.db, "", "", ""
			s, err := c.tgt.e.RoutineSQL(r)
			if err != nil {
				return "", err
			}
			create = s
		case "trigger":
			t, err := side.e.LoadTriggerDef(ctx, side.sess.DB, side.db, k.name)
			if err != nil {
				return "", err
			}
			t.Database, t.OrigName, t.Definer = c.tgt.db, "", ""
			s, err := c.tgt.e.TriggerSQL(t)
			if err != nil {
				return "", err
			}
			create = s
		case "event":
			ev, err := side.e.LoadEventDef(ctx, side.sess.DB, side.db, k.name)
			if err != nil {
				return "", err
			}
			ev.Database, ev.OrigName, ev.Definer = c.tgt.db, "", ""
			s, err := c.tgt.e.EventSQL(ev, nil)
			if err != nil {
				return "", err
			}
			create = s
		}
		return create, nil
	}
	drop := "DROP " + strings.ToUpper(k.kind) + " IF EXISTS " + c.tgt.g.QuoteIdent(c.tgt.db) + "." + c.tgt.g.QuoteIdent(k.name)
	var err error
	if inSrc {
		if item.SourceSQL, err = render(c.src, true); err != nil {
			return item, err
		}
	}
	if inTgt {
		if item.TargetSQL, err = render(c.tgt, false); err != nil {
			return item, err
		}
	}
	switch {
	case !inTgt:
		item.Status, item.Statements = "create", []string{item.SourceSQL}
	case !inSrc:
		item.Status, item.Statements = "drop", []string{drop}
	case normalizeSQL(item.SourceSQL) == normalizeSQL(item.TargetSQL):
		item.Status = "same"
	default:
		item.Status, item.Statements = "alter", []string{drop, item.SourceSQL}
	}
	if item.Statements == nil {
		item.Statements = []string{}
	}
	return item, nil
}

func normalizeSQL(s string) string { return strings.Join(strings.Fields(s), " ") }

func (c *comparer) compareTable(ctx context.Context, item SchemaDiffItem, inSrc, inTgt bool) (SchemaDiffItem, error) {
	item.Statements = []string{}
	var sdef, tdef database.TableDef
	var err error
	if inTgt {
		if tdef, err = c.tgt.e.LoadTableDef(ctx, c.tgt.sess.DB, c.tgt.db, item.Name); err != nil {
			return item, err
		}
		plain := tdef
		plain.OrigName, plain.Options.AutoIncrement = "", ""
		if item.TargetSQL, err = c.tgt.e.CreateTableDefSQL(clearOrig(plain)); err != nil {
			return item, err
		}
	}
	if !inSrc {
		item.Status = "drop"
		item.Statements = []string{"DROP TABLE IF EXISTS " + c.tgt.g.QuoteIdent(c.tgt.db) + "." + c.tgt.g.QuoteIdent(item.Name)}
		return item, nil
	}
	if sdef, err = c.src.e.LoadTableDef(ctx, c.src.sess.DB, c.src.db, item.Name); err != nil {
		return item, err
	}
	// The source definition, moved to the target database.
	sdef.Database, sdef.MariaDB = c.tgt.db, c.maria
	sdef.Options.AutoIncrement = ""
	for i := range sdef.ForeignKeys {
		if sdef.ForeignKeys[i].RefDatabase == c.src.db {
			sdef.ForeignKeys[i].RefDatabase = c.tgt.db
		}
	}
	for i := range sdef.Checks {
		sdef.Checks[i].Clause = c.retarget(sdef.Checks[i].Clause)
	}
	plain := clearOrig(sdef)
	plain.OrigName = ""
	if item.SourceSQL, err = c.tgt.e.CreateTableDefSQL(plain); err != nil {
		return item, err
	}
	if !inTgt {
		item.Status, item.Statements = "create", []string{item.SourceSQL}
		return item, nil
	}
	// Link the source objects to the target's ones by name so the editor's
	// diff produces ALTERs instead of drop/add.
	sdef.OrigName = item.Name
	sdef.Options.AutoIncrement = tdef.Options.AutoIncrement
	for i := range sdef.Columns {
		sdef.Columns[i].OrigName = ""
		for _, tc := range tdef.Columns {
			if strings.EqualFold(tc.Name, sdef.Columns[i].Name) {
				sdef.Columns[i].OrigName = tc.Name
			}
		}
	}
	for i := range sdef.Indexes {
		sdef.Indexes[i].OrigName = ""
		for _, ti := range tdef.Indexes {
			if strings.EqualFold(ti.Name, sdef.Indexes[i].Name) {
				sdef.Indexes[i].OrigName = ti.Name
			}
		}
	}
	for i := range sdef.ForeignKeys {
		sdef.ForeignKeys[i].OrigName = ""
		for _, tf := range tdef.ForeignKeys {
			if strings.EqualFold(tf.Name, sdef.ForeignKeys[i].Name) {
				sdef.ForeignKeys[i].OrigName = tf.Name
			}
		}
	}
	for i := range sdef.Checks {
		sdef.Checks[i].OrigName = ""
		for _, tc := range tdef.Checks {
			if strings.EqualFold(tc.Name, sdef.Checks[i].Name) {
				sdef.Checks[i].OrigName = tc.Name
			}
		}
	}
	stmts, err := c.tgt.e.AlterTableSQL(tdef, sdef)
	if err != nil {
		return item, err
	}
	if len(stmts) == 0 {
		item.Status = "same"
		return item, nil
	}
	item.Status, item.Statements = "alter", stmts
	return item, nil
}

func clearOrig(d database.TableDef) database.TableDef {
	d.Columns = append([]database.ColumnDef(nil), d.Columns...)
	d.Indexes = append([]database.IndexDef(nil), d.Indexes...)
	d.ForeignKeys = append([]database.ForeignKeyDef(nil), d.ForeignKeys...)
	d.Checks = append([]database.CheckDef(nil), d.Checks...)
	for i := range d.Columns {
		d.Columns[i].OrigName = ""
	}
	for i := range d.Indexes {
		d.Indexes[i].OrigName = ""
	}
	for i := range d.ForeignKeys {
		d.ForeignKeys[i].OrigName = ""
	}
	for i := range d.Checks {
		d.Checks[i].OrigName = ""
	}
	return d
}

// ApplySchemaDiff runs the chosen statements on the target (foreign key
// checks off, as tables may be created before the ones they reference).
func (a *App) ApplySchemaDiff(targetID, targetDB string, statements []string, continueOnError, confirmed bool) ([]query.AdminResult, error) {
	if len(statements) == 0 {
		return []query.AdminResult{}, nil
	}
	res, err := a.queries.RunAdmin(context.Background(), query.AdminRequest{ConnectionID: targetID, Database: targetDB, Statements: statements, Confirmed: confirmed,
		DisableFKChecks: true, ContinueOnError: continueOnError, Timeout: time.Hour})
	if err != nil {
		return nil, wrap(err)
	}
	a.meta.Invalidate(targetID, targetDB)
	return res, nil
}

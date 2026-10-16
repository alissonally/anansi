package api

import (
	"context"
	"strings"
	"time"

	"anansi/internal/apperr"
	"anansi/internal/connections"
	"anansi/internal/database"
	"anansi/internal/query"
)

func (a *App) objectEditor(id string) (*connections.Session, database.ObjectEditor, error) {
	sess, err := a.session(id)
	if err != nil {
		return nil, nil, err
	}
	e, ok := sess.Driver.(database.ObjectEditor)
	if !ok {
		return nil, nil, apperr.New(apperr.CodeValidation, "editores de objetos não suportados por este banco de dados")
	}
	return sess, e, nil
}

func editCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// runDDL runs editor statements sequentially (DDL commits implicitly, so
// there is no transaction) and invalidates the database metadata.
func (a *App) runDDL(id, db string, stmts []string, confirmed bool) ([]query.AdminResult, error) {
	if len(stmts) == 0 {
		return []query.AdminResult{}, nil
	}
	res, err := a.queries.RunAdmin(context.Background(), query.AdminRequest{ConnectionID: id, Database: db, Statements: stmts, Confirmed: confirmed, Timeout: 30 * time.Minute})
	if err != nil {
		return nil, wrap(err)
	}
	a.meta.Invalidate(id, db)
	return res, nil
}

func isMaria(sess *connections.Session) bool { return strings.EqualFold(sess.Server.Flavor, "MariaDB") }

// ---- tables -------------------------------------------------------------------

// LoadTableDef reads the structure of a table for the table editor.
func (a *App) LoadTableDef(id, db, table string) (database.TableDef, error) {
	sess, e, err := a.objectEditor(id)
	if err != nil {
		return database.TableDef{}, err
	}
	ctx, cancel := editCtx()
	defer cancel()
	def, err := e.LoadTableDef(ctx, sess.DB, db, table)
	if err != nil {
		return def, dbErr(sess, err)
	}
	return def, nil
}

// tableStatements returns CREATE TABLE for new tables (OrigName == "") or
// the ALTER statements computed against the table as it is now on the
// server (the editor's own copy of the original is not trusted).
func (a *App) tableStatements(id string, def database.TableDef) ([]string, error) {
	sess, e, err := a.objectEditor(id)
	if err != nil {
		return nil, err
	}
	def.MariaDB = isMaria(sess)
	if def.OrigName == "" {
		s, err := e.CreateTableDefSQL(def)
		if err != nil {
			return nil, wrap(err)
		}
		return []string{s}, nil
	}
	ctx, cancel := editCtx()
	defer cancel()
	cur, err := e.LoadTableDef(ctx, sess.DB, def.Database, def.OrigName)
	if err != nil {
		return nil, dbErr(sess, err)
	}
	stmts, err := e.AlterTableSQL(cur, def)
	return stmts, wrap(err)
}

// PreviewTableDef returns the statements SaveTableDef would run.
func (a *App) PreviewTableDef(id string, def database.TableDef) ([]string, error) {
	return a.tableStatements(id, def)
}

// SaveTableDef creates or alters a table.
func (a *App) SaveTableDef(id string, def database.TableDef, confirmed bool) ([]query.AdminResult, error) {
	stmts, err := a.tableStatements(id, def)
	if err != nil {
		return nil, err
	}
	return a.runDDL(id, def.Database, stmts, confirmed)
}

// ---- views --------------------------------------------------------------------

// LoadViewDef reads a view for the view editor.
func (a *App) LoadViewDef(id, db, name string) (database.ViewDef, error) {
	sess, e, err := a.objectEditor(id)
	if err != nil {
		return database.ViewDef{}, err
	}
	ctx, cancel := editCtx()
	defer cancel()
	v, err := e.LoadViewDef(ctx, sess.DB, db, name)
	if err != nil {
		return v, dbErr(sess, err)
	}
	return v, nil
}

// PreviewViewDef returns the statements SaveViewDef would run.
func (a *App) PreviewViewDef(id string, v database.ViewDef) ([]string, error) {
	_, e, err := a.objectEditor(id)
	if err != nil {
		return nil, err
	}
	s, err := e.ViewSQL(v)
	return s, wrap(err)
}

// SaveViewDef creates or alters a view.
func (a *App) SaveViewDef(id string, v database.ViewDef, confirmed bool) ([]query.AdminResult, error) {
	stmts, err := a.PreviewViewDef(id, v)
	if err != nil {
		return nil, err
	}
	return a.runDDL(id, v.Database, stmts, confirmed)
}

// ---- routines and triggers (recreated safely) --------------------------------------

func tempName(name string) string {
	n := name + "_anansi_tmp"
	if r := []rune(n); len(r) > 64 {
		n = string(r[len(r)-64:])
	}
	return n
}

// replaceObject recreates a routine or trigger. Servers cannot ALTER their
// bodies, so like HeidiSQL the new definition is first created under a
// temporary name (a full syntax and reference check that leaves the
// original untouched), the temporary copy is dropped, then the original is
// dropped and recreated. If that last CREATE still fails, the original is
// restored from its previous CREATE statement.
func (a *App) replaceObject(id, db string, kind database.ObjectKind, origName string, origKind database.ObjectKind, createSQL, tempSQL string, confirmed bool) ([]query.AdminResult, error) {
	if origName == "" {
		return a.runDDL(id, db, []string{createSQL}, confirmed)
	}
	_, g, err := a.objectSQL(id)
	if err != nil {
		return nil, err
	}
	sess, b, err := a.browser(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := editCtx()
	old, err := b.CreateStatement(ctx, db, origKind, origName)
	cancel()
	if err != nil {
		return nil, dbErr(sess, err)
	}
	dropTemp, err := g.DropSQL(kind, db, tempName(origName))
	if err != nil {
		return nil, wrap(err)
	}
	dropOrig, err := g.DropSQL(origKind, db, origName)
	if err != nil {
		return nil, wrap(err)
	}
	stmts := []string{tempSQL, dropTemp, dropOrig, createSQL}
	// Replacing the object loses no data: outside production the DROP of the
	// old version does not need the destructive-statement confirmation.
	if sess.Profile.Environment != connections.EnvProduction {
		confirmed = true
	}
	res, err := a.runDDL(id, db, stmts, confirmed)
	if err != nil {
		return nil, err
	}
	if len(res) == len(stmts) && res[3].Error != nil {
		// The original is gone and the new definition failed: put it back.
		restore, rerr := a.queries.RunAdmin(context.Background(), query.AdminRequest{ConnectionID: id, Database: db, Statements: []string{old}, Confirmed: true})
		if rerr == nil && len(restore) == 1 {
			restore[0].SQL = "-- restauração da versão anterior\n" + restore[0].SQL
			res = append(res, restore[0])
		}
	}
	if len(res) >= 2 && res[0].Error == nil && res[1].Error != nil {
		// Could not drop the temporary copy; report it but keep going.
		res[1].Error.Message += " (remova manualmente o objeto temporário " + tempName(origName) + ")"
	}
	return res, nil
}

// LoadRoutineDef reads a procedure or function for the routine editor.
func (a *App) LoadRoutineDef(id, db, typ, name string) (database.RoutineDef, error) {
	sess, e, err := a.objectEditor(id)
	if err != nil {
		return database.RoutineDef{}, err
	}
	ctx, cancel := editCtx()
	defer cancel()
	r, err := e.LoadRoutineDef(ctx, sess.DB, db, typ, name)
	if err != nil {
		return r, dbErr(sess, err)
	}
	return r, nil
}

// PreviewRoutineDef returns the CREATE statement of a routine.
func (a *App) PreviewRoutineDef(id string, r database.RoutineDef) (string, error) {
	_, e, err := a.objectEditor(id)
	if err != nil {
		return "", err
	}
	s, err := e.RoutineSQL(r)
	return s, wrap(err)
}

func routineKind(typ string) database.ObjectKind {
	if strings.EqualFold(typ, "FUNCTION") {
		return database.ObjFunction
	}
	return database.ObjProcedure
}

// SaveRoutineDef creates a routine or replaces an existing one safely.
func (a *App) SaveRoutineDef(id string, r database.RoutineDef, confirmed bool) ([]query.AdminResult, error) {
	_, e, err := a.objectEditor(id)
	if err != nil {
		return nil, err
	}
	create, err := e.RoutineSQL(r)
	if err != nil {
		return nil, wrap(err)
	}
	if r.OrigName == "" {
		return a.runDDL(id, r.Database, []string{create}, confirmed)
	}
	tmp := r
	tmp.Name = tempName(r.OrigName)
	tempSQL, err := e.RoutineSQL(tmp)
	if err != nil {
		return nil, wrap(err)
	}
	origType := r.OrigType
	if origType == "" {
		origType = r.Type
	}
	return a.replaceObject(id, r.Database, routineKind(r.Type), r.OrigName, routineKind(origType), create, tempSQL, confirmed)
}

// LoadTriggerDef reads a trigger for the trigger editor.
func (a *App) LoadTriggerDef(id, db, name string) (database.TriggerDef, error) {
	sess, e, err := a.objectEditor(id)
	if err != nil {
		return database.TriggerDef{}, err
	}
	ctx, cancel := editCtx()
	defer cancel()
	t, err := e.LoadTriggerDef(ctx, sess.DB, db, name)
	if err != nil {
		return t, dbErr(sess, err)
	}
	return t, nil
}

// PreviewTriggerDef returns the CREATE statement of a trigger.
func (a *App) PreviewTriggerDef(id string, t database.TriggerDef) (string, error) {
	_, e, err := a.objectEditor(id)
	if err != nil {
		return "", err
	}
	s, err := e.TriggerSQL(t)
	return s, wrap(err)
}

// SaveTriggerDef creates a trigger or replaces an existing one safely.
func (a *App) SaveTriggerDef(id string, t database.TriggerDef, confirmed bool) ([]query.AdminResult, error) {
	_, e, err := a.objectEditor(id)
	if err != nil {
		return nil, err
	}
	create, err := e.TriggerSQL(t)
	if err != nil {
		return nil, wrap(err)
	}
	if t.OrigName == "" {
		return a.runDDL(id, t.Database, []string{create}, confirmed)
	}
	tmp := t
	tmp.Name = tempName(t.OrigName)
	tempSQL, err := e.TriggerSQL(tmp)
	if err != nil {
		return nil, wrap(err)
	}
	return a.replaceObject(id, t.Database, database.ObjTrigger, t.OrigName, database.ObjTrigger, create, tempSQL, confirmed)
}

// ---- events -------------------------------------------------------------------------

// LoadEventDef reads an event for the event editor.
func (a *App) LoadEventDef(id, db, name string) (database.EventDef, error) {
	sess, e, err := a.objectEditor(id)
	if err != nil {
		return database.EventDef{}, err
	}
	ctx, cancel := editCtx()
	defer cancel()
	ev, err := e.LoadEventDef(ctx, sess.DB, db, name)
	if err != nil {
		return ev, dbErr(sess, err)
	}
	return ev, nil
}

func (a *App) eventStatement(id string, ev database.EventDef) (string, error) {
	sess, e, err := a.objectEditor(id)
	if err != nil {
		return "", err
	}
	var cur *database.EventDef
	if ev.OrigName != "" {
		ctx, cancel := editCtx()
		c, err := e.LoadEventDef(ctx, sess.DB, ev.Database, ev.OrigName)
		cancel()
		if err != nil {
			return "", dbErr(sess, err)
		}
		cur = &c
	}
	s, err := e.EventSQL(ev, cur)
	return s, wrap(err)
}

// PreviewEventDef returns CREATE EVENT or ALTER EVENT.
func (a *App) PreviewEventDef(id string, ev database.EventDef) (string, error) {
	return a.eventStatement(id, ev)
}

// SaveEventDef creates or alters an event.
func (a *App) SaveEventDef(id string, ev database.EventDef, confirmed bool) ([]query.AdminResult, error) {
	s, err := a.eventStatement(id, ev)
	if err != nil {
		return nil, err
	}
	return a.runDDL(id, ev.Database, []string{s}, confirmed)
}

// Definers lists user@host accounts for definer pickers.
func (a *App) Definers(id string) ([]string, error) {
	sess, e, err := a.objectEditor(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := editCtx()
	defer cancel()
	v, err := e.Definers(ctx, sess.DB)
	return v, dbErr(sess, err)
}

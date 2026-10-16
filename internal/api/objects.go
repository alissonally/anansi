package api

import (
	"context"

	"dbstudio/internal/apperr"
	"dbstudio/internal/connections"
	"dbstudio/internal/database"
	"dbstudio/internal/query"
	"dbstudio/internal/sqllex"
)

// session returns an open session after validating the id.
func (a *App) session(id string) (*connections.Session, error) {
	if err := checkConnID(id); err != nil {
		return nil, err
	}
	return a.manager.Get(id)
}

// dbErr converts a driver error into an apperr with the password redacted.
func dbErr(sess *connections.Session, err error) error {
	if err == nil {
		return nil
	}
	de := *sess.Driver.DescribeError(err)
	de.Message = sess.Redact(de.Message)
	return apperr.FromDB(&de)
}

func (a *App) browser(id string) (*connections.Session, database.ObjectBrowser, error) {
	sess, err := a.session(id)
	if err != nil {
		return nil, nil, err
	}
	b, ok := sess.Driver.Inspector(sess.DB).(database.ObjectBrowser)
	if !ok {
		return nil, nil, apperr.New(apperr.CodeValidation, "este banco de dados não oferece o catálogo detalhado")
	}
	return sess, b, nil
}

func (a *App) serverAdmin(id string) (*connections.Session, database.ServerAdmin, error) {
	sess, err := a.session(id)
	if err != nil {
		return nil, nil, err
	}
	s, ok := sess.Driver.Inspector(sess.DB).(database.ServerAdmin)
	if !ok {
		return nil, nil, apperr.New(apperr.CodeValidation, "este banco de dados não oferece informações do servidor")
	}
	return sess, s, nil
}

func (a *App) objectSQL(id string) (*connections.Session, database.ObjectSQL, error) {
	sess, err := a.session(id)
	if err != nil {
		return nil, nil, err
	}
	g, ok := sess.Driver.(database.ObjectSQL)
	if !ok {
		return nil, nil, apperr.New(apperr.CodeValidation, "operação não suportada por este banco de dados")
	}
	return sess, g, nil
}

// browse runs fn with a timeout and converts its error.
func browse[T any](a *App, id string, fn func(ctx context.Context, b database.ObjectBrowser) (T, error)) (T, error) {
	var zero T
	sess, b, err := a.browser(id)
	if err != nil {
		return zero, err
	}
	ctx, cancel := metaCtx()
	defer cancel()
	v, err := fn(ctx, b)
	if err != nil {
		return zero, dbErr(sess, err)
	}
	return v, nil
}

// ---- catalog ----------------------------------------------------------------

// DatabaseSummaries lists databases with sizes and object counts.
func (a *App) DatabaseSummaries(id string) ([]database.DatabaseSummary, error) {
	return browse(a, id, func(ctx context.Context, b database.ObjectBrowser) ([]database.DatabaseSummary, error) {
		return b.DatabaseSummaries(ctx)
	})
}

// TableStatus lists tables and views of a database with sizes and options.
func (a *App) TableStatus(id, db string) ([]database.TableStatus, error) {
	return browse(a, id, func(ctx context.Context, b database.ObjectBrowser) ([]database.TableStatus, error) {
		return b.TableStatus(ctx, db)
	})
}

// ForeignKeys lists the foreign keys of a table.
func (a *App) ForeignKeys(id, db, table string) ([]database.ForeignKeyInfo, error) {
	return browse(a, id, func(ctx context.Context, b database.ObjectBrowser) ([]database.ForeignKeyInfo, error) {
		return b.ForeignKeys(ctx, db, table)
	})
}

// ReferencingKeys lists foreign keys of other tables that point to table.
func (a *App) ReferencingKeys(id, db, table string) ([]database.ForeignKeyInfo, error) {
	return browse(a, id, func(ctx context.Context, b database.ObjectBrowser) ([]database.ForeignKeyInfo, error) {
		return b.ReferencingKeys(ctx, db, table)
	})
}

// CheckConstraints lists the CHECK constraints of a table.
func (a *App) CheckConstraints(id, db, table string) ([]database.CheckInfo, error) {
	return browse(a, id, func(ctx context.Context, b database.ObjectBrowser) ([]database.CheckInfo, error) {
		return b.CheckConstraints(ctx, db, table)
	})
}

// Triggers lists the triggers of a database.
func (a *App) Triggers(id, db string) ([]database.TriggerInfo, error) {
	return browse(a, id, func(ctx context.Context, b database.ObjectBrowser) ([]database.TriggerInfo, error) {
		return b.Triggers(ctx, db)
	})
}

// Events lists the scheduled events of a database.
func (a *App) Events(id, db string) ([]database.EventInfo, error) {
	return browse(a, id, func(ctx context.Context, b database.ObjectBrowser) ([]database.EventInfo, error) {
		return b.Events(ctx, db)
	})
}

// RoutineDetails lists procedures and functions with parameters and bodies.
func (a *App) RoutineDetails(id, db string) ([]database.RoutineDetail, error) {
	return browse(a, id, func(ctx context.Context, b database.ObjectBrowser) ([]database.RoutineDetail, error) {
		return b.RoutineDetails(ctx, db)
	})
}

// CreateStatement returns the CREATE statement of an object.
func (a *App) CreateStatement(id, db string, kind database.ObjectKind, name string) (string, error) {
	if !kind.Valid() {
		return "", apperr.New(apperr.CodeValidation, "tipo de objeto inválido")
	}
	return browse(a, id, func(ctx context.Context, b database.ObjectBrowser) (string, error) {
		return b.CreateStatement(ctx, db, kind, name)
	})
}

// ---- server -------------------------------------------------------------------

// ServerVariables lists system variables (session and global values).
func (a *App) ServerVariables(id string) ([]database.ServerVariable, error) {
	sess, s, err := a.serverAdmin(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := metaCtx()
	defer cancel()
	v, err := s.Variables(ctx)
	return v, dbErr(sess, err)
}

// ServerStatus lists global status counters.
func (a *App) ServerStatus(id string) ([]database.StatusValue, error) {
	sess, s, err := a.serverAdmin(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := metaCtx()
	defer cancel()
	v, err := s.Status(ctx)
	return v, dbErr(sess, err)
}

// ProcessList lists server threads.
func (a *App) ProcessList(id string) ([]database.Process, error) {
	sess, s, err := a.serverAdmin(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := metaCtx()
	defer cancel()
	v, err := s.Processes(ctx)
	return v, dbErr(sess, err)
}

// ---- actions ------------------------------------------------------------------

// ObjectRef identifies an object in action requests.
type ObjectRef struct {
	Kind     database.ObjectKind `json:"kind"`
	Database string              `json:"database"`
	Name     string              `json:"name"`
}

// adminRun executes application-generated statements. With alwaysConfirm,
// a confirmation is required even when the preferences would not ask for
// one (dropping objects from the tree, killing processes).
func (a *App) adminRun(id, db string, stmts []string, confirmed, alwaysConfirm bool, reason string, continueOnError bool, noFKChecks ...bool) ([]query.AdminResult, error) {
	sess, err := a.session(id)
	if err != nil {
		return nil, err
	}
	if alwaysConfirm && !confirmed {
		if sess.Profile.ReadOnly {
			// Let the policy report the read-only error.
			_, err := a.queries.RunAdmin(context.Background(), query.AdminRequest{ConnectionID: id, Database: db, Statements: stmts})
			return nil, wrap(err)
		}
		details := query.ConfirmationDetails{Production: sess.Profile.Environment == connections.EnvProduction, Environment: sess.Profile.Environment, Connection: sess.Profile.Name}
		for _, s := range stmts {
			kind, _, why := sqllex.Classify(s)
			if why == "" {
				why = reason
			}
			details.Statements = append(details.Statements, query.ConfirmStatement{SQL: s, Kind: kind, Reason: why})
		}
		e := apperr.New(apperr.CodeConfirmationRequired, "confirmação necessária antes de executar")
		e.Details = details
		return nil, e
	}
	res, err := a.queries.RunAdmin(context.Background(), query.AdminRequest{
		ConnectionID: id, Database: db, Statements: stmts, Confirmed: confirmed, ContinueOnError: continueOnError,
		DisableFKChecks: len(noFKChecks) > 0 && noFKChecks[0],
	})
	return res, wrap(err)
}

// firstError returns the first statement error as an apperr.
func firstError(res []query.AdminResult) error {
	for _, r := range res {
		if r.Error != nil {
			return apperr.FromDB(r.Error)
		}
	}
	return nil
}

// DropObjects drops objects (always asks for confirmation). Every object is
// attempted; the results report failures individually. Like HeidiSQL,
// foreign key checks are disabled so tables referencing each other can be
// dropped together.
func (a *App) DropObjects(id string, objects []ObjectRef, confirmed bool) ([]query.AdminResult, error) {
	_, g, err := a.objectSQL(id)
	if err != nil {
		return nil, err
	}
	if len(objects) == 0 || len(objects) > 1000 {
		return nil, apperr.New(apperr.CodeValidation, "selecione entre 1 e 1000 objetos")
	}
	stmts := make([]string, 0, len(objects))
	for _, o := range objects {
		s, err := g.DropSQL(o.Kind, o.Database, o.Name)
		if err != nil {
			return nil, wrap(err)
		}
		stmts = append(stmts, s)
	}
	return a.adminRun(id, "", stmts, confirmed, true, "remove o objeto permanentemente", true, true)
}

// TruncateTables empties tables (always asks for confirmation). With
// noFKChecks, foreign key checks are disabled while truncating.
func (a *App) TruncateTables(id, db string, tables []string, noFKChecks, confirmed bool) ([]query.AdminResult, error) {
	_, g, err := a.objectSQL(id)
	if err != nil {
		return nil, err
	}
	if len(tables) == 0 || len(tables) > 1000 {
		return nil, apperr.New(apperr.CodeValidation, "selecione entre 1 e 1000 tabelas")
	}
	stmts := make([]string, 0, len(tables))
	for _, t := range tables {
		s, err := g.TruncateSQL(db, t)
		if err != nil {
			return nil, wrap(err)
		}
		stmts = append(stmts, s)
	}
	return a.adminRun(id, "", stmts, confirmed, true, "remove todas as linhas", true, noFKChecks)
}

// RenameObject renames (or moves) a table, view or event.
func (a *App) RenameObject(id string, obj ObjectRef, toDatabase, toName string, confirmed bool) error {
	_, g, err := a.objectSQL(id)
	if err != nil {
		return err
	}
	s, err := g.RenameSQL(obj.Kind, obj.Database, obj.Name, toDatabase, toName)
	if err != nil {
		return wrap(err)
	}
	res, err := a.adminRun(id, "", []string{s}, confirmed, false, "", false)
	if err != nil {
		return err
	}
	return firstError(res)
}

// AlterDatabase changes the default charset/collation of a database.
func (a *App) AlterDatabase(id, db, charset, collation string, confirmed bool) error {
	_, g, err := a.objectSQL(id)
	if err != nil {
		return err
	}
	s, err := g.AlterDatabaseSQL(db, charset, collation)
	if err != nil {
		return wrap(err)
	}
	res, err := a.adminRun(id, "", []string{s}, confirmed, false, "", false)
	if err != nil {
		return err
	}
	return firstError(res)
}

// SetVariable changes a system variable. Session changes apply to a pooled
// connection, so only GLOBAL is useful from the server tab; SESSION values
// are set in SQL tabs.
func (a *App) SetVariable(id string, global bool, name, value, valueKind string, confirmed bool) error {
	_, g, err := a.objectSQL(id)
	if err != nil {
		return err
	}
	s, err := g.SetVariableSQL(global, name, value, valueKind)
	if err != nil {
		return wrap(err)
	}
	res, err := a.adminRun(id, "", []string{s}, confirmed, global, "altera a configuração do servidor para todas as conexões", false)
	if err != nil {
		return err
	}
	return firstError(res)
}

// KillProcesses terminates server threads or only their running statements
// (always asks for confirmation).
func (a *App) KillProcesses(id string, ids []int64, queryOnly, confirmed bool) ([]query.AdminResult, error) {
	_, g, err := a.objectSQL(id)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 || len(ids) > 500 {
		return nil, apperr.New(apperr.CodeValidation, "selecione entre 1 e 500 processos")
	}
	stmts := make([]string, 0, len(ids))
	for _, pid := range ids {
		s, err := g.KillSQL(pid, queryOnly)
		if err != nil {
			return nil, wrap(err)
		}
		stmts = append(stmts, s)
	}
	reason := "encerra a conexão e reverte transações abertas"
	if queryOnly {
		reason = "interrompe a instrução em execução"
	}
	return a.adminRun(id, "", stmts, confirmed, true, reason, true)
}

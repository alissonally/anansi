package api

import (
	"context"
	"time"

	"anansi/internal/apperr"
	"anansi/internal/database"
)

func (a *App) ddlBuilder(id string) (database.DDLBuilder, error) {
	if err := checkConnID(id); err != nil {
		return nil, err
	}
	sess, err := a.manager.Get(id)
	if err != nil {
		return nil, err
	}
	b, ok := sess.Driver.(database.DDLBuilder)
	if !ok {
		return nil, apperr.New(apperr.CodeValidation, "este banco de dados não suporta a criação assistida de objetos")
	}
	return b, nil
}

// DDLOptions lists charsets, collations, engines and column types offered by
// the "new database" and "new table" dialogs.
func (a *App) DDLOptions(id string) (database.DDLOptions, error) {
	b, err := a.ddlBuilder(id)
	if err != nil {
		return database.DDLOptions{}, err
	}
	sess, _ := a.manager.Get(id)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	opts, err := b.DDLOptions(ctx, sess.DB)
	if err != nil {
		return opts, apperr.FromDB(sess.Driver.DescribeError(err))
	}
	return opts, nil
}

// PreviewCreateDatabase returns the CREATE DATABASE statement for spec.
func (a *App) PreviewCreateDatabase(id string, spec database.CreateDatabaseSpec) (string, error) {
	b, err := a.ddlBuilder(id)
	if err != nil {
		return "", err
	}
	s, err := b.CreateDatabaseSQL(spec)
	return s, wrap(err)
}

// CreateDatabase creates a database. Production connections require
// confirmed=true (confirmation_required otherwise).
func (a *App) CreateDatabase(id string, spec database.CreateDatabaseSpec, confirmed bool) error {
	b, err := a.ddlBuilder(id)
	if err != nil {
		return err
	}
	stmt, err := b.CreateDatabaseSQL(spec)
	if err != nil {
		return wrap(err)
	}
	if err := a.queries.RunDDL(context.Background(), id, stmt, "", confirmed); err != nil {
		return wrap(err)
	}
	a.meta.Invalidate(id, "")
	return nil
}

// PreviewCreateTable returns the CREATE TABLE statement for spec.
func (a *App) PreviewCreateTable(id string, spec database.CreateTableSpec) (string, error) {
	b, err := a.ddlBuilder(id)
	if err != nil {
		return "", err
	}
	s, err := b.CreateTableSQL(spec)
	return s, wrap(err)
}

// CreateTable creates a table. Production connections require confirmed=true.
func (a *App) CreateTable(id string, spec database.CreateTableSpec, confirmed bool) error {
	b, err := a.ddlBuilder(id)
	if err != nil {
		return err
	}
	stmt, err := b.CreateTableSQL(spec)
	if err != nil {
		return wrap(err)
	}
	if err := a.queries.RunDDL(context.Background(), id, stmt, spec.Database, confirmed); err != nil {
		return wrap(err)
	}
	a.meta.Invalidate(id, spec.Database)
	return nil
}

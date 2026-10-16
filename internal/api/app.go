// Package api exposes the application services to the frontend through Wails
// bindings. Every method validates its input (ids, sizes) and converts errors
// into apperr.Error JSON documents. No method returns secrets.
package api

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"dbstudio/internal/apperr"
	"dbstudio/internal/autocomplete"
	"dbstudio/internal/config"
	"dbstudio/internal/connections"
	"dbstudio/internal/database"
	"dbstudio/internal/database/mysql"
	"dbstudio/internal/history"
	"dbstudio/internal/metadata"
	"dbstudio/internal/query"
	"dbstudio/internal/security"
	"dbstudio/internal/sshtunnel"
	"dbstudio/internal/tasks"
	"dbstudio/internal/workspace"
)

// Version of the application.
const Version = "0.1.0"

// App is bound to the frontend.
type App struct {
	ctx context.Context
	log *slog.Logger

	configDir string
	prefs     *config.PreferencesStore
	conns     *connections.Service
	manager   *connections.Manager
	hostKeys  *sshtunnel.HostKeys
	meta      *metadata.Cache
	queries   *query.Service
	history   *history.Store
	work      *workspace.Store
	engine    autocomplete.Engine
	sqlLog    *sqlLog
	tasks     *tasks.Manager

	startupWarnings []string

	filesMu  sync.Mutex
	approved map[string]bool // file paths obtained through native dialogs
}

// New wires all services. Problems loading local files are reported to the
// UI as warnings; the app still starts with defaults.
func New(log *slog.Logger, secrets security.SecretStore) (*App, error) {
	dir, err := config.Dir()
	if err != nil {
		return nil, fmt.Errorf("diretório de configuração indisponível: %w", err)
	}
	a := &App{log: log, configDir: dir, approved: map[string]bool{}}
	warn := func(err error) {
		if err != nil {
			a.startupWarnings = append(a.startupWarnings, err.Error())
			log.Warn("startup", "error", err.Error())
		}
	}
	a.prefs, err = config.OpenPreferences(dir)
	warn(err)
	store, err := connections.OpenProfileStore(dir)
	warn(err)
	a.history, err = history.Open(dir, a.prefs.Get().HistoryLimit)
	warn(err)
	a.work, err = workspace.Open(dir)
	warn(err)

	a.sqlLog = newSQLLog(a.emitSQLLog, func(id, msg string) string {
		if sess, err := a.manager.Get(id); err == nil {
			return sess.Redact(msg)
		}
		return msg
	})
	a.sqlLog.setCapacity(a.prefs.Get().SQLLogLimit)
	reg := database.NewRegistry(mysql.New())
	a.hostKeys = sshtunnel.NewHostKeys(filepath.Join(dir, "known_hosts"))
	a.manager = connections.NewManager(connections.RegistryOpener(reg, a.sqlLog.funcFor, connections.WithHostKeys(a.hostKeys)))
	a.conns = &connections.Service{Store: store, Secrets: secrets, Memory: security.NewMemory(), Manager: a.manager, Registry: reg}
	a.meta = metadata.NewCache(a.manager)
	a.tasks = tasks.NewManager(a.emitTask)
	a.queries = query.NewService(a.manager, a.queryOptions, query.Hooks{AfterStatement: a.afterStatement})
	return a, nil
}

func (a *App) queryOptions() query.Options {
	p := a.prefs.Get()
	return query.Options{PageSize: p.MaxRows, Timeout: time.Duration(p.QueryTimeoutSeconds) * time.Second, ConfirmDestructive: p.ConfirmDestructive, Warnings: !p.HideWarnings}
}

// afterStatement records history and invalidates metadata after DDL.
func (a *App) afterStatement(ev query.StatementEvent) {
	if ev.Err == nil && (ev.Kind == "ddl") {
		a.meta.Invalidate(ev.ConnectionID, ev.Database)
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "metadata:invalidated", ev.ConnectionID, ev.Database)
		}
	}
	// Data browsing is not history; app-generated reads (metadata, server
	// overview) never reach this hook.
	if !a.prefs.Get().HistoryEnabled || (ev.Source == query.SourceData && ev.Kind == "read") {
		return
	}
	name := ""
	if p, err := a.conns.Store.Get(ev.ConnectionID); err == nil {
		name = p.Name
	}
	e := history.Entry{
		ConnectionID: ev.ConnectionID, ConnectionName: name, Database: ev.Database, SQL: ev.SQL,
		ExecutedAt: ev.At.UTC(), DurationMs: float64(ev.Duration.Microseconds()) / 1000, Success: ev.Err == nil, RowCount: ev.RowCount,
	}
	if ev.Err != nil {
		e.Error = ev.Err.Message
	}
	a.history.Add(e)
}

// Startup is called by Wails when the window is ready.
func (a *App) Startup(ctx context.Context) { a.ctx = ctx }

// Shutdown closes every session and flushes local files.
func (a *App) Shutdown(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		tctx, tcancel := context.WithTimeout(context.Background(), 3*time.Second)
		a.tasks.Close(tctx)
		tcancel()
		a.queries.Close()
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.manager.CloseAll(cctx); err != nil {
			a.log.Warn("shutdown", "error", err.Error())
		}
		_ = a.history.Flush()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		a.log.Warn("shutdown timed out")
	}
}

func wrap(err error) error { return apperr.Wrap(err) }

func checkConnID(id string) error {
	if !connections.ValidID(id) {
		return apperr.New(apperr.CodeValidation, "identificador de conexão inválido")
	}
	return nil
}

// ---- application -----------------------------------------------------------

// AppInfo describes the environment.
type AppInfo struct {
	Version          string   `json:"version"`
	KeyringAvailable bool     `json:"keyringAvailable"`
	ConfigDir        string   `json:"configDir"`
	Warnings         []string `json:"warnings"`
}

// GetAppInfo returns version and environment information.
func (a *App) GetAppInfo() AppInfo {
	return AppInfo{Version: Version, KeyringAvailable: a.conns.KeyringAvailable(), ConfigDir: a.configDir, Warnings: a.startupWarnings}
}

// GetPreferences returns the preferences.
func (a *App) GetPreferences() config.Preferences { return a.prefs.Get() }

// SavePreferences validates and stores preferences.
func (a *App) SavePreferences(p config.Preferences) (config.Preferences, error) {
	out, err := a.prefs.Set(p)
	if err == nil {
		a.history.SetLimit(out.HistoryLimit)
		a.sqlLog.setCapacity(out.SQLLogLimit)
	}
	return out, wrap(err)
}

// ---- connections -----------------------------------------------------------

// ListConnections returns saved profiles.
func (a *App) ListConnections() []connections.ProfileView { return a.conns.List() }

// SaveConnection creates or updates a profile.
func (a *App) SaveConnection(in connections.SaveInput) (connections.SaveResult, error) {
	r, err := a.conns.Save(in)
	return r, wrap(err)
}

// DuplicateConnection copies a profile.
func (a *App) DuplicateConnection(id string) (connections.ProfileView, error) {
	if err := checkConnID(id); err != nil {
		return connections.ProfileView{}, err
	}
	p, err := a.conns.Duplicate(id)
	return p, wrap(err)
}

// DeleteConnection removes a profile, its password and closes its session.
func (a *App) DeleteConnection(id string) error {
	if err := checkConnID(id); err != nil {
		return err
	}
	return wrap(a.conns.Delete(id))
}

// ReorderConnections sets the sidebar order.
func (a *App) ReorderConnections(ids []string) error {
	for _, id := range ids {
		if err := checkConnID(id); err != nil {
			return err
		}
	}
	return wrap(a.conns.Store.Reorder(ids))
}

// TestConnection opens and closes a temporary connection.
func (a *App) TestConnection(in connections.TestInput) (database.ServerInfo, error) {
	timeout := time.Duration(in.Profile.Normalize().ConnectTimeoutSeconds)*time.Second + 5*time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	info, err := a.conns.Test(ctx, in)
	return info, wrap(err)
}

// Connect opens a session for a profile.
func (a *App) Connect(id string, password *string, remember bool) (connections.SessionState, error) {
	if err := checkConnID(id); err != nil {
		return connections.SessionState{}, err
	}
	p, err := a.conns.Store.Get(id)
	if err != nil {
		return connections.SessionState{}, wrap(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(p.ConnectTimeoutSeconds)*time.Second+10*time.Second)
	defer cancel()
	st, err := a.conns.Connect(ctx, id, password, remember)
	if err == nil {
		a.log.Info("connected", "connection", id, "server", st.Server.Flavor+" "+st.Server.Version, "tls", st.Server.TLSActive)
	}
	return st, wrap(err)
}

// Disconnect closes a session.
func (a *App) Disconnect(id string) error {
	if err := checkConnID(id); err != nil {
		return err
	}
	return wrap(a.manager.Disconnect(id))
}

// SessionStates lists open sessions.
func (a *App) SessionStates() []connections.SessionState { return a.manager.States() }

// ---- metadata --------------------------------------------------------------

func metaCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// ListDatabases lists databases of a connection.
func (a *App) ListDatabases(id string) ([]database.DatabaseInfo, error) {
	if err := checkConnID(id); err != nil {
		return nil, err
	}
	ctx, cancel := metaCtx()
	defer cancel()
	v, err := a.meta.Databases(ctx, id)
	return v, wrap(err)
}

// ListTables lists tables and views.
func (a *App) ListTables(id, db string) ([]database.TableInfo, error) {
	if err := checkConnID(id); err != nil {
		return nil, err
	}
	ctx, cancel := metaCtx()
	defer cancel()
	v, err := a.meta.Tables(ctx, id, db)
	return v, wrap(err)
}

// ListColumns lists columns of a table.
func (a *App) ListColumns(id, db, table string) ([]database.ColumnInfo, error) {
	if err := checkConnID(id); err != nil {
		return nil, err
	}
	ctx, cancel := metaCtx()
	defer cancel()
	v, err := a.meta.Columns(ctx, id, db, table)
	return v, wrap(err)
}

// ListIndexes lists indexes of a table.
func (a *App) ListIndexes(id, db, table string) ([]database.IndexInfo, error) {
	if err := checkConnID(id); err != nil {
		return nil, err
	}
	ctx, cancel := metaCtx()
	defer cancel()
	v, err := a.meta.Indexes(ctx, id, db, table)
	return v, wrap(err)
}

// ListRoutines lists procedures and functions.
func (a *App) ListRoutines(id, db string) ([]database.RoutineInfo, error) {
	if err := checkConnID(id); err != nil {
		return nil, err
	}
	ctx, cancel := metaCtx()
	defer cancel()
	v, err := a.meta.Routines(ctx, id, db)
	return v, wrap(err)
}

// RefreshMetadata drops cached metadata ("" = whole connection).
func (a *App) RefreshMetadata(id, db string) error {
	if err := checkConnID(id); err != nil {
		return err
	}
	a.meta.Invalidate(id, db)
	return nil
}

// WarmMetadata preloads the columns of a database for autocompletion.
func (a *App) WarmMetadata(id, db string) error {
	if err := checkConnID(id); err != nil || db == "" {
		return err
	}
	go func() {
		ctx, cancel := metaCtx()
		defer cancel()
		_, _ = a.meta.AllColumns(ctx, id, db)
		_, _ = a.meta.Tables(ctx, id, db)
	}()
	return nil
}

// catalog adapts the metadata cache to autocomplete.Catalog.
type catalog struct {
	c  *metadata.Cache
	id string
}

func (c catalog) Databases(ctx context.Context) ([]database.DatabaseInfo, error) {
	return c.c.Databases(ctx, c.id)
}
func (c catalog) Tables(ctx context.Context, db string) ([]database.TableInfo, error) {
	return c.c.Tables(ctx, c.id, db)
}
func (c catalog) Columns(ctx context.Context, db, table string) ([]database.ColumnInfo, error) {
	return c.c.Columns(ctx, c.id, db, table)
}
func (c catalog) Routines(ctx context.Context, db string) ([]database.RoutineInfo, error) {
	return c.c.Routines(ctx, c.id, db)
}

// offlineCatalog is used when the tab has no open connection.
type offlineCatalog struct{}

func (offlineCatalog) Databases(context.Context) ([]database.DatabaseInfo, error) { return nil, nil }
func (offlineCatalog) Tables(context.Context, string) ([]database.TableInfo, error) {
	return nil, nil
}
func (offlineCatalog) Columns(context.Context, string, string) ([]database.ColumnInfo, error) {
	return nil, nil
}
func (offlineCatalog) Routines(context.Context, string) ([]database.RoutineInfo, error) {
	return nil, nil
}

// Complete returns completion proposals. Metadata is fetched through the
// cache; a slow server never blocks completion for more than a few seconds.
func (a *App) Complete(id string, req autocomplete.Request) (autocomplete.Response, error) {
	if len(req.SQL) > workspace.MaxTabSQLBytes {
		return autocomplete.Response{}, nil
	}
	var cat autocomplete.Catalog = offlineCatalog{}
	if connections.ValidID(id) {
		if _, err := a.manager.Get(id); err == nil {
			cat = catalog{c: a.meta, id: id}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	return a.engine.Complete(ctx, cat, req), nil
}

// ---- execution -------------------------------------------------------------

// Execute runs SQL in a tab.
func (a *App) Execute(req query.ExecuteRequest) (*query.ExecuteResponse, error) {
	if err := checkConnID(req.ConnectionID); err != nil {
		return nil, err
	}
	if len(req.SQL) > 16*1024*1024 {
		return nil, apperr.New(apperr.CodeValidation, "script grande demais (limite de 16 MB)")
	}
	r, err := a.queries.Execute(context.Background(), req)
	return r, wrap(err)
}

// FetchMore loads the next page of a result.
func (a *App) FetchMore(req query.FetchRequest) (*query.FetchResponse, error) {
	r, err := a.queries.FetchMore(req)
	return r, wrap(err)
}

// CancelExecution cancels the running execution of a tab.
func (a *App) CancelExecution(tabID string) error { return wrap(a.queries.Cancel(tabID)) }

// CloseResult discards the open cursor of a tab.
func (a *App) CloseResult(tabID string) error { return wrap(a.queries.CloseCursor(tabID)) }

// ReleaseTab frees a tab's connection (tab closed or connection changed).
func (a *App) ReleaseTab(tabID string, force bool) error {
	return wrap(a.queries.ReleaseTab(tabID, force))
}

// TabState reports a tab's execution context.
func (a *App) TabState(tabID string) (query.TabState, error) {
	s, err := a.queries.State(tabID)
	return s, wrap(err)
}

// ---- history ---------------------------------------------------------------

// ListHistory returns history entries.
func (a *App) ListHistory(q history.Query) []history.Entry { return a.history.List(q) }

// ClearHistory removes history ("" = all connections).
func (a *App) ClearHistory(connectionID string) error {
	if connectionID != "" {
		if err := checkConnID(connectionID); err != nil {
			return err
		}
	}
	return wrap(a.history.Clear(connectionID))
}

// ---- workspace -------------------------------------------------------------

// LoadWorkspace returns persisted tabs.
func (a *App) LoadWorkspace() (workspace.State, error) {
	st, err := a.work.LoadState()
	a.filesMu.Lock()
	for _, t := range st.Tabs {
		if t.FilePath != "" {
			a.approved[filepath.Clean(t.FilePath)] = true
		}
	}
	a.filesMu.Unlock()
	return st, wrap(err)
}

// SaveWorkspace persists tabs. File paths must have been approved through a
// dialog.
func (a *App) SaveWorkspace(st workspace.State) error {
	a.filesMu.Lock()
	for i := range st.Tabs {
		if p := st.Tabs[i].FilePath; p != "" && !a.approved[filepath.Clean(p)] {
			st.Tabs[i].FilePath = ""
		}
	}
	a.filesMu.Unlock()
	return wrap(a.work.SaveState(st))
}

// ListSavedQueries lists saved queries.
func (a *App) ListSavedQueries() []workspace.SavedQuery { return a.work.Queries() }

// SaveQuery creates or updates a saved query.
func (a *App) SaveQuery(q workspace.SavedQuery) (workspace.SavedQuery, error) {
	r, err := a.work.SaveQuery(q, connections.NewID)
	return r, wrap(err)
}

// DeleteSavedQuery removes a saved query.
func (a *App) DeleteSavedQuery(id string) error { return wrap(a.work.DeleteQuery(id)) }

// FileContent is a SQL file opened by the user.
type FileContent struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

var sqlFilters = []runtime.FileFilter{{DisplayName: "Arquivos SQL (*.sql)", Pattern: "*.sql"}, {DisplayName: "Todos os arquivos", Pattern: "*"}}

// OpenSQLFile shows the native open dialog and returns the file content.
// It returns nil when the user cancels.
func (a *App) OpenSQLFile() (*FileContent, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Abrir arquivo SQL", Filters: sqlFilters})
	if err != nil || path == "" {
		return nil, wrap(err)
	}
	content, err := workspace.ReadSQLFile(path)
	if err != nil {
		return nil, wrap(err)
	}
	a.approve(path)
	return &FileContent{Path: path, Name: filepath.Base(path), Content: content}, nil
}

func (a *App) approve(path string) {
	a.filesMu.Lock()
	a.approved[filepath.Clean(path)] = true
	a.filesMu.Unlock()
}

// SaveSQLFile writes to a path previously obtained through a dialog.
func (a *App) SaveSQLFile(path, content string) error {
	a.filesMu.Lock()
	ok := a.approved[filepath.Clean(path)]
	a.filesMu.Unlock()
	if !ok {
		return apperr.New(apperr.CodeValidation, "caminho de arquivo não autorizado; use \"Salvar como\"")
	}
	return wrap(workspace.WriteSQLFile(path, content))
}

// SaveSQLFileAs shows the native save dialog and writes the file. It returns
// nil when the user cancels.
func (a *App) SaveSQLFileAs(suggestedName, content string) (*FileContent, error) {
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, suggestedName)
	if !strings.HasSuffix(strings.ToLower(name), ".sql") {
		name += ".sql"
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "Salvar arquivo SQL", DefaultFilename: name, Filters: sqlFilters})
	if err != nil || path == "" {
		return nil, wrap(err)
	}
	if err := workspace.WriteSQLFile(path, content); err != nil {
		return nil, wrap(err)
	}
	a.approve(path)
	return &FileContent{Path: path, Name: filepath.Base(path), Content: content}, nil
}

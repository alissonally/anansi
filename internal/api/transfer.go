package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	gomysql "github.com/go-sql-driver/mysql"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"anansi/internal/apperr"
	"anansi/internal/connections"
	"anansi/internal/csvimport"
	"anansi/internal/database"
	"anansi/internal/dump"
	"anansi/internal/gridexport"
	"anansi/internal/query"
	"anansi/internal/sqlfile"
	"anansi/internal/sqllex"
	"anansi/internal/tasks"
)

// ---- tasks -----------------------------------------------------------------------

func (a *App) emitTask(t tasks.Task) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "task:update", t)
	}
}

// ListTasks returns background tasks, newest first.
func (a *App) ListTasks() []tasks.Task { return a.tasks.List() }

// CancelTask stops a running task.
func (a *App) CancelTask(id string) { a.tasks.Cancel(id) }

// DismissTask forgets a finished task.
func (a *App) DismissTask(id string) { a.tasks.Dismiss(id) }

// OpenTaskFolder opens the folder of the file produced by a task.
func (a *App) OpenTaskFolder(id string) error {
	for _, t := range a.tasks.List() {
		if t.ID == id && t.Path != "" {
			dir := t.Path
			if st, err := os.Stat(dir); err != nil || !st.IsDir() {
				dir = filepath.Dir(dir)
			}
			return wrap(exec.Command("xdg-open", dir).Start())
		}
	}
	return apperr.New(apperr.CodeNotFound, "tarefa sem arquivo")
}

// writeGuard enforces read-only profiles and asks for confirmation on
// production before an operation that writes through its own connection
// (imports, SQL files, copies into a database).
func (a *App) writeGuard(id, what string, confirmed bool) error {
	sess, err := a.session(id)
	if err != nil {
		return err
	}
	if sess.Profile.ReadOnly {
		return apperr.New(apperr.CodeReadOnly, "a conexão \""+sess.Profile.Name+"\" está marcada como somente leitura")
	}
	if sess.Profile.Environment == connections.EnvProduction && !confirmed {
		e := apperr.New(apperr.CodeConfirmationRequired, "confirmação necessária antes de executar")
		e.Details = query.ConfirmationDetails{Production: true, Environment: sess.Profile.Environment, Connection: sess.Profile.Name,
			Statements: []query.ConfirmStatement{{SQL: what, Kind: sqllex.CatWrite, Reason: "alteração em conexão de produção"}}}
		return e
	}
	return nil
}

// ---- export of grid rows ------------------------------------------------------------

func formatRows(cols []gridexport.Column, rows [][]any, opts gridexport.Options, w io.Writer) error {
	wr, err := gridexport.New(w, opts)
	if err != nil {
		return err
	}
	if err := wr.Begin(cols); err != nil {
		return err
	}
	for _, r := range rows {
		if err := wr.Row(r); err != nil {
			return err
		}
	}
	return wr.End()
}

// FormatRows renders rows already loaded in the UI (copy to clipboard).
func (a *App) FormatRows(cols []gridexport.Column, rows [][]any, opts gridexport.Options) (string, error) {
	if len(rows) > 500_000 {
		return "", apperr.New(apperr.CodeValidation, "linhas demais para a área de transferência; exporte para arquivo")
	}
	var b bytes.Buffer
	if err := formatRows(cols, rows, opts, &b); err != nil {
		return "", wrap(err)
	}
	return b.String(), nil
}

func sanitizeFileName(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 32 {
			return '_'
		}
		return r
	}, strings.TrimSpace(s))
	if s == "" {
		s = "exportacao"
	}
	return s
}

func (a *App) saveDialog(title, suggested, ext string) (string, error) {
	name := sanitizeFileName(suggested)
	if !strings.HasSuffix(strings.ToLower(name), "."+ext) {
		name += "." + ext
	}
	return runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title: title, DefaultFilename: name,
		Filters: []runtime.FileFilter{{DisplayName: strings.ToUpper(ext) + " (*." + ext + ")", Pattern: "*." + ext}, {DisplayName: "Todos os arquivos", Pattern: "*"}},
	})
}

// ExportRowsToFile writes loaded rows to a file chosen in the native
// dialog. Returns the path ("" when cancelled).
func (a *App) ExportRowsToFile(cols []gridexport.Column, rows [][]any, opts gridexport.Options, suggestedName string) (string, error) {
	path, err := a.saveDialog("Exportar linhas", suggestedName, gridexport.Extension(opts.Format))
	if err != nil || path == "" {
		return "", wrap(err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", wrap(err)
	}
	if err := formatRows(cols, rows, opts, f); err != nil {
		f.Close()
		return "", wrap(err)
	}
	return path, wrap(f.Close())
}

// streamExport runs sql on a dedicated connection and writes every row.
func (a *App) streamExport(ctx context.Context, p *tasks.Progress, id, db, sqlText string, args []any, opts gridexport.Options, path string, colInfo func([]query.Column) []gridexport.Column) (string, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	defer f.Close()
	wr, err := gridexport.New(f, opts)
	if err != nil {
		return "", err
	}
	p.SetPath(path)
	var n int64
	err = a.queries.WithConn(ctx, id, func(conn *sql.Conn, sess *connections.Session) error {
		if db != "" {
			if cd, ok := sess.Driver.(database.CurrentDatabaser); ok {
				if err := cd.UseDatabase(ctx, conn, db); err != nil {
					return query.DescribeErr(sess, err)
				}
			}
		}
		err := query.StreamRows(ctx, conn, sqlText, args, func(cols []query.Column) error {
			return wr.Begin(colInfo(cols))
		}, func(row []any) error {
			n++
			if n%1000 == 0 {
				p.Set(n, 0)
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			return wr.Row(row)
		})
		return query.DescribeErr(sess, err)
	})
	if err != nil {
		return "", err
	}
	if err := wr.End(); err != nil {
		return "", err
	}
	p.Set(n, n)
	return fmt.Sprintf("%d linhas exportadas", n), f.Close()
}

func plainCols(cols []query.Column) []gridexport.Column {
	out := make([]gridexport.Column, len(cols))
	for i, c := range cols {
		out[i] = gridexport.Column{Name: c.Name, Type: c.Type, Numeric: c.Numeric, Binary: c.Binary}
	}
	return out
}

// ExportQueryToFile re-runs a read query and streams every row to a file
// (not limited to the rows loaded in the grid). Returns the task id ("" when
// the dialog was cancelled).
func (a *App) ExportQueryToFile(id, db, sqlText string, opts gridexport.Options, suggestedName string) (string, error) {
	if err := checkConnID(id); err != nil {
		return "", err
	}
	stmts := sqllex.Split(sqlText)
	if len(stmts) != 1 || stmts[0].Kind != sqllex.CatRead {
		return "", apperr.New(apperr.CodeValidation, "a exportação completa aceita uma única instrução de leitura (SELECT, SHOW…)")
	}
	path, err := a.saveDialog("Exportar resultado completo", suggestedName, gridexport.Extension(opts.Format))
	if err != nil || path == "" {
		return "", wrap(err)
	}
	if opts.Query == "" {
		opts.Query = stmts[0].Text
	}
	return a.tasks.Start("export", "Exportar "+filepath.Base(path), "rows", func(ctx context.Context, p *tasks.Progress) (string, error) {
		return a.streamExport(ctx, p, id, db, stmts[0].Text, nil, opts, path, plainCols)
	}), nil
}

// ExportTableToFile streams a whole table (with the grid filter and sort) to
// a file in any export format.
func (a *App) ExportTableToFile(id string, spec database.BrowseSpec, opts gridexport.Options) (string, error) {
	_, g, err := a.dataSQL(id)
	if err != nil {
		return "", err
	}
	cols, key, err := a.tableMeta(id, spec.Database, spec.Table, g)
	if err != nil {
		return "", wrap(err)
	}
	spec.NoLimit, spec.PartialLength = true, 0
	plan, err := g.SelectSQL(spec, cols, key)
	if err != nil {
		return "", wrap(err)
	}
	path, err := a.saveDialog("Exportar tabela", spec.Table, gridexport.Extension(opts.Format))
	if err != nil || path == "" {
		return "", wrap(err)
	}
	if opts.Table == "" {
		opts.Table = spec.Table
	}
	isKey := map[string]bool{}
	for _, k := range key.Columns {
		isKey[strings.ToLower(k)] = true
	}
	auto := map[string]bool{}
	for _, c := range cols {
		if strings.Contains(strings.ToLower(c.Extra), "auto_increment") {
			auto[strings.ToLower(c.Name)] = true
		}
	}
	return a.tasks.Start("export", "Exportar "+spec.Table, "rows", func(ctx context.Context, p *tasks.Progress) (string, error) {
		return a.streamExport(ctx, p, id, "", plan.SQL, nil, opts, path, func(qc []query.Column) []gridexport.Column {
			out := plainCols(qc)
			for i := range out {
				out[i].Key = isKey[strings.ToLower(out[i].Name)]
				out[i].AutoIncrement = auto[strings.ToLower(out[i].Name)]
			}
			return out
		})
	}), nil
}

// ---- SQL dump ----------------------------------------------------------------------------

// DumpTarget says where a dump goes.
type DumpTarget struct {
	// Kind: "file" (.sql), "gzip" (.sql.gz), "directory", "database"
	// (another database on the same or another connection).
	Kind         string `json:"kind"`
	ConnectionID string `json:"connectionId"` // database target ("" = same)
	Database     string `json:"database"`     // database target
	// ContinueOnError keeps copying after failed statements (database target).
	ContinueOnError bool `json:"continueOnError"`
}

// StartDump exports databases/objects as SQL. File and directory targets
// are chosen in native dialogs; database targets execute the statements
// (read-only targets are refused, production targets need confirmation).
// Returns the task id ("" when cancelled).
func (a *App) StartDump(id string, opts dump.Options, target DumpTarget, confirmed bool) (string, error) {
	sess, b, err := a.browser(id)
	if err != nil {
		return "", err
	}
	if len(opts.Selections) == 0 {
		return "", apperr.New(apperr.CodeValidation, "selecione pelo menos um database")
	}
	title := opts.Selections[0].Database
	if len(opts.Selections) > 1 {
		title = fmt.Sprintf("%d databases", len(opts.Selections))
	}
	var path string
	var targetConn string
	switch target.Kind {
	case "file", "gzip":
		ext := "sql"
		if target.Kind == "gzip" {
			ext = "sql.gz"
		}
		stamp := time.Now().Format("20060102-150405")
		if path, err = a.saveDialog("Salvar dump SQL", title+"-"+stamp, ext); err != nil || path == "" {
			return "", wrap(err)
		}
	case "directory":
		if path, err = runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Pasta do dump (um arquivo por objeto)", CanCreateDirectories: true}); err != nil || path == "" {
			return "", wrap(err)
		}
	case "database":
		targetConn = target.ConnectionID
		if targetConn == "" {
			targetConn = id
		}
		if target.Database == "" {
			return "", apperr.New(apperr.CodeValidation, "informe o database de destino")
		}
		if targetConn == id && len(opts.Selections) == 1 && target.Database == opts.Selections[0].Database {
			return "", apperr.New(apperr.CodeValidation, "o destino é o próprio database de origem")
		}
		if len(opts.Selections) != 1 {
			return "", apperr.New(apperr.CodeValidation, "copie um database por vez para outro database")
		}
		if err := a.writeGuard(targetConn, fmt.Sprintf("Copiar %s para %s", title, target.Database), confirmed); err != nil {
			return "", err
		}
		opts.TargetDatabase = target.Database
	default:
		return "", apperr.New(apperr.CodeValidation, "destino inválido")
	}
	server := sess.Profile.Host + " — " + sess.Server.Flavor + " " + sess.Server.Version
	return a.tasks.Start("dump", "Dump de "+title, "rows", func(ctx context.Context, p *tasks.Progress) (string, error) {
		var sink dump.Sink
		var execSink *dump.ExecSink
		var closeTarget func()
		switch target.Kind {
		case "file", "gzip":
			s, err := dump.NewFileSink(path)
			if err != nil {
				return "", err
			}
			sink = s
			p.SetPath(path)
		case "directory":
			s, err := dump.NewDirSink(path)
			if err != nil {
				return "", err
			}
			sink = s
			p.SetPath(path)
		case "database":
			tsess, err := a.manager.Get(targetConn)
			if err != nil {
				return "", err
			}
			tconn, err := tsess.DB.Conn(ctx)
			if err != nil {
				return "", query.DescribeErr(tsess, err)
			}
			closeTarget = func() { _ = tconn.Close() }
			execSink = &dump.ExecSink{Ctx: ctx, Conn: tconn}
			if target.ContinueOnError {
				execSink.OnError = func(stmt string, err error) bool {
					p.Warn(query.DescribeErr(tsess, err).Error())
					return true
				}
			}
			sink = execSink
		}
		if closeTarget != nil {
			defer closeTarget()
		}
		err := a.queries.WithConn(ctx, id, func(conn *sql.Conn, s *connections.Session) error {
			d := &dump.Dumper{Conn: conn, Browser: b, Opts: opts, Sink: sink, Progress: p, Server: server}
			return query.DescribeErr(s, d.Run(ctx))
		})
		if err != nil {
			return "", err
		}
		if execSink != nil {
			a.meta.Invalidate(targetConn, target.Database)
			return fmt.Sprintf("%d instruções executadas em %s", execSink.Executed, target.Database), nil
		}
		return "Dump salvo em " + path, nil
	}), nil
}

// ---- SQL files -------------------------------------------------------------------------

// PickSQLFile chooses a .sql file to run (its path is approved for RunSQLFile).
func (a *App) PickSQLFile() (*FileInfo, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Executar arquivo SQL", Filters: append([]runtime.FileFilter{{DisplayName: "SQL (*.sql, *.sql.gz)", Pattern: "*.sql;*.sql.gz"}}, sqlFilters[1:]...)})
	if err != nil || path == "" {
		return nil, wrap(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, wrap(err)
	}
	a.approve(path)
	return &FileInfo{Path: path, Name: filepath.Base(path), Size: st.Size()}, nil
}

// FileInfo describes a file chosen by the user.
type FileInfo struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func (a *App) approvedPath(path string) error {
	a.filesMu.Lock()
	ok := a.approved[filepath.Clean(path)]
	a.filesMu.Unlock()
	if !ok {
		return apperr.New(apperr.CodeValidation, "arquivo não autorizado; escolha-o pelo diálogo")
	}
	return nil
}

// RunSQLFile executes a (possibly huge) SQL file statement by statement in
// the background, without loading it into the editor. The caller must have
// confirmed (the file is not reviewed statement by statement). Read-only
// profiles only accept read and session statements.
func (a *App) RunSQLFile(id, db, path string, stopOnError, confirmed bool) (string, error) {
	sess, err := a.session(id)
	if err != nil {
		return "", err
	}
	if err := a.approvedPath(path); err != nil {
		return "", err
	}
	if !confirmed {
		e := apperr.New(apperr.CodeConfirmationRequired, "confirmação necessária antes de executar")
		e.Details = query.ConfirmationDetails{Production: sess.Profile.Environment == connections.EnvProduction, Environment: sess.Profile.Environment, Connection: sess.Profile.Name,
			Statements: []query.ConfirmStatement{{SQL: "-- arquivo " + filepath.Base(path), Kind: sqllex.CatOther, Reason: "todas as instruções do arquivo serão executadas sem revisão individual"}}}
		return "", e
	}
	readOnly := sess.Profile.ReadOnly
	return a.tasks.Start("sqlfile", "Executar "+filepath.Base(path), "bytes", func(ctx context.Context, p *tasks.Progress) (string, error) {
		f, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer f.Close()
		st, _ := f.Stat()
		var r io.Reader = f
		total := st.Size()
		if strings.HasSuffix(strings.ToLower(path), ".gz") {
			gz, err := gzip.NewReader(f)
			if err != nil {
				return "", err
			}
			defer gz.Close()
			r = gz
			total = 0 // decompressed size unknown
		}
		var res sqlfile.Result
		err = a.queries.WithConn(ctx, id, func(conn *sql.Conn, s *connections.Session) error {
			if db != "" {
				if cd, ok := s.Driver.(database.CurrentDatabaser); ok {
					if err := cd.UseDatabase(ctx, conn, db); err != nil {
						return query.DescribeErr(s, err)
					}
				}
			}
			var err error
			res, err = sqlfile.Run(ctx, r, total, func(ctx context.Context, st sqllex.Statement) error {
				if readOnly && st.Kind != sqllex.CatRead && st.Kind != sqllex.CatSession && st.Kind != sqllex.CatTransaction {
					return errors.New("conexão somente leitura: instrução não permitida")
				}
				rows, err := conn.QueryContext(ctx, st.Text)
				if err != nil {
					return query.DescribeErr(s, err)
				}
				for rows.Next() { // discard result sets (SELECT in scripts)
				}
				_ = rows.Close()
				return query.DescribeErr(s, rows.Err())
			}, p, stopOnError)
			return err
		})
		a.meta.Invalidate(id, "")
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "metadata:invalidated", id, "")
		}
		summary := fmt.Sprintf("%d instruções executadas, %d com erro", res.Statements, res.Errors)
		if err != nil {
			return summary, err
		}
		return summary, nil
	}), nil
}

// ---- CSV import -------------------------------------------------------------------------

// CSVPreview is the result of PickCSVFile.
type CSVPreview struct {
	FileInfo
	Detected csvimport.Detected `json:"detected"`
}

// PickCSVFile chooses a delimited text file and detects its layout.
func (a *App) PickCSVFile() (*CSVPreview, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Importar CSV", Filters: []runtime.FileFilter{
		{DisplayName: "Texto delimitado (*.csv, *.tsv, *.txt)", Pattern: "*.csv;*.tsv;*.txt"}, {DisplayName: "Todos os arquivos", Pattern: "*"}}})
	if err != nil || path == "" {
		return nil, wrap(err)
	}
	a.approve(path)
	return a.PreviewCSV(path, nil)
}

// PreviewCSV re-reads the beginning of an approved file with a format
// (nil = detect).
func (a *App) PreviewCSV(path string, format *csvimport.Format) (*CSVPreview, error) {
	if err := a.approvedPath(path); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, wrap(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	head := make([]byte, 256<<10)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	d := csvimport.Detect(head, 500)
	if format != nil {
		// Same detection of types but with the user's layout.
		r, err := csvimport.NewReader(bytes.NewReader(head), *format)
		if err != nil {
			return nil, wrap(err)
		}
		d.Format = *format
		d.Sample = nil
		for len(d.Sample) < 20 {
			rec, err := r.Read()
			if err != nil {
				break
			}
			d.Sample = append(d.Sample, rec)
		}
	}
	return &CSVPreview{FileInfo: FileInfo{Path: path, Name: filepath.Base(path), Size: st.Size()}, Detected: d}, nil
}

// CSVImport describes an import request.
type CSVImport struct {
	Path     string           `json:"path"`
	Format   csvimport.Format `json:"format"`
	Target   csvimport.Target `json:"target"`
	Truncate bool             `json:"truncate"`
	// ServerSide uses LOAD DATA LOCAL INFILE (requires local_infile=ON on
	// the server); otherwise the app parses the file and sends INSERTs.
	ServerSide bool `json:"serverSide"`
}

// ImportCSV loads a delimited file into a table in the background.
func (a *App) ImportCSV(id string, req CSVImport, confirmed bool) (string, error) {
	if err := a.approvedPath(req.Path); err != nil {
		return "", err
	}
	if err := a.writeGuard(id, fmt.Sprintf("Importar %s para %s.%s", filepath.Base(req.Path), req.Target.Database, req.Target.Table), confirmed); err != nil {
		return "", err
	}
	_, g, err := a.objectSQL(id)
	if err != nil {
		return "", err
	}
	if _, err := g.TruncateSQL(req.Target.Database, req.Target.Table); err != nil {
		return "", wrap(err) // validates names
	}
	return a.tasks.Start("import", "Importar "+filepath.Base(req.Path), "bytes", func(ctx context.Context, p *tasks.Progress) (string, error) {
		f, err := os.Open(req.Path)
		if err != nil {
			return "", err
		}
		defer f.Close()
		st, _ := f.Stat()
		var out string
		err = a.queries.WithConn(ctx, id, func(conn *sql.Conn, s *connections.Session) error {
			if req.Truncate {
				t, _ := g.TruncateSQL(req.Target.Database, req.Target.Table)
				if _, err := conn.ExecContext(ctx, t); err != nil {
					return query.DescribeErr(s, err)
				}
			}
			if req.ServerSide {
				n, err := loadDataLocal(ctx, conn, req)
				if err != nil {
					return query.DescribeErr(s, err)
				}
				out = fmt.Sprintf("%d linhas importadas pelo servidor (LOAD DATA)", n)
				return nil
			}
			r, err := csvimport.NewReader(f, req.Format)
			if err != nil {
				return err
			}
			stats, err := csvimport.Import(ctx, conn, r, req.Target, st.Size(), p)
			if err != nil {
				return query.DescribeErr(s, err)
			}
			out = fmt.Sprintf("%d registros lidos, %d linhas afetadas", stats.Rows, stats.Inserted)
			return nil
		})
		a.meta.Invalidate(id, req.Target.Database)
		return out, err
	}), nil
}

func sqlChar(s string) string {
	switch s {
	case `\t`, "tab":
		return gridexport.QuoteString("\t")
	}
	return gridexport.QuoteString(s)
}

// loadDataLocal runs LOAD DATA LOCAL INFILE for one registered file only
// (the driver keeps allowAllFiles off).
func loadDataLocal(ctx context.Context, conn *sql.Conn, req CSVImport) (int64, error) {
	gomysql.RegisterLocalFile(req.Path)
	defer gomysql.DeregisterLocalFile(req.Path)
	// Literals below use backslash escapes.
	if _, err := conn.ExecContext(ctx, "SET SESSION sql_mode = REPLACE(@@sql_mode, 'NO_BACKSLASH_ESCAPES', '')"); err != nil {
		return 0, err
	}
	f := req.Format
	var b strings.Builder
	verb := ""
	switch req.Target.Mode {
	case csvimport.ModeIgnore:
		verb = " IGNORE"
	case csvimport.ModeReplace:
		verb = " REPLACE"
	}
	fmt.Fprintf(&b, "LOAD DATA LOCAL INFILE %s%s INTO TABLE %s.%s", gridexport.QuoteString(req.Path), verb, gridexport.QuoteIdent(req.Target.Database), gridexport.QuoteIdent(req.Target.Table))
	if strings.EqualFold(f.Encoding, "latin1") || strings.EqualFold(f.Encoding, "windows-1252") {
		b.WriteString(" CHARACTER SET latin1")
	} else {
		b.WriteString(" CHARACTER SET utf8mb4")
	}
	sep := f.Separator
	if sep == "" {
		sep = ","
	}
	fmt.Fprintf(&b, " FIELDS TERMINATED BY %s", sqlChar(sep))
	if f.Encloser != "" {
		fmt.Fprintf(&b, " OPTIONALLY ENCLOSED BY %s", sqlChar(f.Encloser))
	}
	esc := f.Escape
	if esc == "" {
		esc = f.Encloser
	}
	fmt.Fprintf(&b, " ESCAPED BY %s LINES TERMINATED BY '\\n'", sqlChar(esc))
	if f.SkipLines > 0 {
		fmt.Fprintf(&b, " IGNORE %d LINES", f.SkipLines)
	}
	var cols []string
	for i, c := range req.Target.Columns {
		if c == "" {
			cols = append(cols, fmt.Sprintf("@skip%d", i))
		} else {
			cols = append(cols, gridexport.QuoteIdent(c))
		}
	}
	b.WriteString(" (" + strings.Join(cols, ", ") + ")")
	res, err := conn.ExecContext(ctx, b.String())
	if err != nil {
		if strings.Contains(err.Error(), "local infile") || strings.Contains(err.Error(), "3948") || strings.Contains(err.Error(), "1148") {
			return 0, fmt.Errorf("o servidor não permite LOAD DATA LOCAL (local_infile desligado); use a leitura pelo aplicativo: %w", err)
		}
		return 0, err
	}
	return res.RowsAffected()
}

// ---- files in cells (insert / save BLOB) --------------------------------------------------

// CellFile is a file read into a cell.
type CellFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Hex  string `json:"hex"`
}

// LoadFileForCell reads a file chosen in the dialog as hex (max 16 MiB),
// HeidiSQL "Insert files" for one cell.
func (a *App) LoadFileForCell() (*CellFile, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Carregar arquivo na célula"})
	if err != nil || path == "" {
		return nil, wrap(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, wrap(err)
	}
	if st.Size() > maxFullCell {
		return nil, apperr.New(apperr.CodeValidation, "arquivo maior que 16 MiB")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, wrap(err)
	}
	return &CellFile{Name: filepath.Base(path), Size: st.Size(), Hex: strings.ToUpper(hex.EncodeToString(b))}, nil
}

// SaveCellToFile writes a cell value (hex for binary data) to a file chosen
// in the dialog.
func (a *App) SaveCellToFile(suggestedName, data string, isHex bool) (string, error) {
	var b []byte
	if isHex {
		var err error
		if b, err = hex.DecodeString(strings.TrimPrefix(strings.TrimPrefix(data, "0x"), "0X")); err != nil {
			return "", apperr.New(apperr.CodeValidation, "valor hexadecimal inválido")
		}
	} else {
		b = []byte(data)
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "Salvar valor em arquivo", DefaultFilename: sanitizeFileName(suggestedName)})
	if err != nil || path == "" {
		return "", wrap(err)
	}
	return path, wrap(os.WriteFile(path, b, 0o644))
}

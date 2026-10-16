// Package mysql implements the database contracts for MySQL and MariaDB using
// github.com/go-sql-driver/mysql.
package mysql

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	gomysql "github.com/go-sql-driver/mysql"

	"dbstudio/internal/database"
	"dbstudio/internal/database/sqllog"
)

// Driver implements database.Driver, database.Canceller and
// database.CurrentDatabaser.
type Driver struct{}

// New returns the MySQL/MariaDB driver.
func New() *Driver { return &Driver{} }

var (
	_ database.Driver           = (*Driver)(nil)
	_ database.Canceller        = (*Driver)(nil)
	_ database.CurrentDatabaser = (*Driver)(nil)
)

// Kinds implements database.Driver.
func (d *Driver) Kinds() []database.Kind { return []database.Kind{database.MySQL, database.MariaDB} }

var paramNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

// blockedParams are driver options that would weaken security or change the
// statement execution policy; they cannot be set as free-form parameters.
var blockedParams = map[string]string{
	"allowallfiles":            "LOAD DATA LOCAL INFILE com acesso irrestrito a arquivos não é permitido",
	"multistatements":          "a execução de múltiplas instruções é controlada pelo aplicativo",
	"allowcleartextpasswords":  "use a opção dedicada de senha em texto claro (exige TLS)",
	"allowoldpasswords":        "o método de autenticação antigo do MySQL é inseguro",
	"allowfallbacktoplaintext": "use o modo TLS \"preferencial\"",
	"tls":                      "use as opções de TLS do formulário",
	"interpolateparams":        "parâmetro gerenciado pelo aplicativo",
	"user":                     "informe o usuário no campo próprio",
	"password":                 "informe a senha no campo próprio",
	"passwd":                   "informe a senha no campo próprio",
}

// Validate implements database.Driver.
func (d *Driver) Validate(p database.ConnectParams) error {
	var problems []string
	if p.Socket != "" {
		if !filepath.IsAbs(p.Socket) {
			problems = append(problems, "o caminho do socket deve ser absoluto")
		}
	} else if strings.TrimSpace(p.Host) == "" {
		problems = append(problems, "o host é obrigatório")
	} else if strings.ContainsAny(p.Host, " /\\?#@") {
		problems = append(problems, "o host contém caracteres inválidos")
	}
	if p.Port < 1 || p.Port > 65535 {
		problems = append(problems, "a porta deve estar entre 1 e 65535")
	}
	if strings.TrimSpace(p.User) == "" {
		problems = append(problems, "o usuário é obrigatório")
	}
	switch p.TLS.Mode {
	case database.TLSDisabled, database.TLSPreferred, database.TLSRequired, database.TLSVerifyCA, database.TLSVerifyFull:
	default:
		problems = append(problems, "modo TLS inválido")
	}
	if p.TLS.Mode == database.TLSVerifyCA && p.TLS.CAFile == "" {
		problems = append(problems, "o modo \"verificar CA\" exige um arquivo de CA")
	}
	for _, f := range []struct{ label, path string }{{"CA", p.TLS.CAFile}, {"certificado do cliente", p.TLS.CertFile}, {"chave do cliente", p.TLS.KeyFile}} {
		if f.path == "" {
			continue
		}
		st, err := os.Stat(f.path)
		if err != nil || !st.Mode().IsRegular() {
			problems = append(problems, fmt.Sprintf("arquivo de %s não encontrado: %s", f.label, f.path))
		}
	}
	if (p.TLS.CertFile == "") != (p.TLS.KeyFile == "") {
		problems = append(problems, "certificado e chave do cliente devem ser informados juntos")
	}
	if p.AllowCleartextPasswords && (p.TLS.Mode == database.TLSDisabled || p.TLS.Mode == database.TLSPreferred) {
		problems = append(problems, "senha em texto claro só é permitida com TLS obrigatório")
	}
	for k, v := range p.Params {
		if !paramNameRe.MatchString(k) {
			problems = append(problems, fmt.Sprintf("nome de parâmetro inválido: %q", k))
			continue
		}
		if why, bad := blockedParams[strings.ToLower(k)]; bad {
			problems = append(problems, fmt.Sprintf("parâmetro %q não permitido: %s", k, why))
			continue
		}
		if strings.ContainsAny(v, ";\n\r\x00") {
			problems = append(problems, fmt.Sprintf("valor inválido para o parâmetro %q", k))
		}
	}
	if len(problems) > 0 {
		return &database.DBError{Category: "validation", Message: strings.Join(problems, "; ")}
	}
	return nil
}

// buildConfig turns params into a driver config without string concatenation
// of user values into a DSN.
func (d *Driver) buildConfig(p database.ConnectParams) (*gomysql.Config, error) {
	if err := d.Validate(p); err != nil {
		return nil, err
	}
	cfg := gomysql.NewConfig()
	cfg.User = p.User
	cfg.Passwd = p.Password
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(strings.TrimSpace(p.Host), strconv.Itoa(p.Port))
	if p.Socket != "" {
		cfg.Net, cfg.Addr = "unix", p.Socket
	}
	if p.Dial != nil {
		cfg.DialFunc = p.Dial
	}
	cfg.DBName = p.Database
	cfg.Timeout = p.ConnectTimeout
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	cfg.WriteTimeout = 60 * time.Second
	cfg.AllowNativePasswords = true
	cfg.AllowCleartextPasswords = p.AllowCleartextPasswords
	cfg.MultiStatements = false
	cfg.AllowAllFiles = false
	cfg.ParseTime = false // keep temporal values exactly as the server sends them
	cfg.CheckConnLiveness = true
	cfg.Params = map[string]string{}
	charset := ""
	for k, v := range p.Params {
		switch strings.ToLower(k) {
		case "charset":
			charset = v
		case "collation":
			cfg.Collation = v
		case "compress":
			if err := cfg.Apply(gomysql.EnableCompression(v == "true" || v == "1")); err != nil {
				return nil, err
			}
		case "connectionattributes":
			cfg.ConnectionAttributes = v
		case "serverpubkey":
			cfg.ServerPubKey = v
		default:
			// Any other key is sent by the driver as a session system
			// variable at connect time (SET k=v).
			cfg.Params[k] = v
		}
	}
	if charset != "" {
		if err := cfg.Apply(gomysql.Charset(charset, cfg.Collation)); err != nil {
			return nil, err
		}
	}
	tlsCfg, err := buildTLS(p)
	if err != nil {
		return nil, err
	}
	cfg.TLS = tlsCfg
	cfg.AllowFallbackToPlaintext = p.TLS.Mode == database.TLSPreferred
	if tlsCfg == nil {
		cfg.TLSConfig = "false"
	}
	return cfg, nil
}

func buildTLS(p database.ConnectParams) (*tls.Config, error) {
	o := p.TLS
	if o.Mode == database.TLSDisabled || o.Mode == "" {
		return nil, nil
	}
	host := strings.TrimSpace(p.Host)
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if o.ServerName != "" {
		cfg.ServerName = o.ServerName
	} else {
		cfg.ServerName = host
	}
	if cfg.ServerName == "" {
		cfg.ServerName = "localhost" // Unix socket
	}
	if o.CertFile != "" {
		pair, err := tls.LoadX509KeyPair(o.CertFile, o.KeyFile)
		if err != nil {
			return nil, &database.DBError{Category: "tls", Message: "não foi possível carregar o certificado do cliente: " + err.Error()}
		}
		cfg.Certificates = []tls.Certificate{pair}
	}
	var roots *x509.CertPool
	if o.CAFile != "" {
		pem, err := os.ReadFile(o.CAFile)
		if err != nil {
			return nil, &database.DBError{Category: "tls", Message: "não foi possível ler o arquivo de CA: " + err.Error()}
		}
		roots = x509.NewCertPool()
		if !roots.AppendCertsFromPEM(pem) {
			return nil, &database.DBError{Category: "tls", Message: "o arquivo de CA não contém certificados PEM válidos"}
		}
		cfg.RootCAs = roots
	}
	switch o.Mode {
	case database.TLSPreferred, database.TLSRequired:
		// Encryption without certificate verification (same semantics as
		// the mysql client's --ssl-mode=REQUIRED/PREFERRED).
		cfg.InsecureSkipVerify = true
	case database.TLSVerifyCA:
		// Verify the chain against the CA but not the host name.
		cfg.InsecureSkipVerify = true
		cfg.VerifyConnection = func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("o servidor não apresentou certificado")
			}
			opts := x509.VerifyOptions{Roots: roots, Intermediates: x509.NewCertPool()}
			for _, c := range cs.PeerCertificates[1:] {
				opts.Intermediates.AddCert(c)
			}
			_, err := cs.PeerCertificates[0].Verify(opts)
			return err
		}
	case database.TLSVerifyFull:
		// Standard verification against RootCAs (or the system pool).
	}
	return cfg, nil
}

// Open implements database.Driver.
func (d *Driver) Open(ctx context.Context, p database.ConnectParams) (*sql.DB, error) {
	cfg, err := d.buildConfig(p)
	if err != nil {
		return nil, err
	}
	cfg.Logger = discardLogger{} // the driver may log addresses and raw errors
	connector, err := gomysql.NewConnector(cfg)
	if err != nil {
		return nil, d.DescribeError(err)
	}
	db := sql.OpenDB(sqllog.Wrap(withInit(connector, p.InitSQL), p.Log))
	maxOpen := p.PoolMaxOpen
	if maxOpen <= 0 {
		maxOpen = 10
	}
	maxIdle := p.PoolMaxIdle
	if maxIdle <= 0 {
		maxIdle = 2
	}
	life := p.PoolMaxLifetime
	if life <= 0 {
		life = 30 * time.Minute
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(life)
	db.SetConnMaxIdleTime(5 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, d.DescribeError(err)
	}
	return db, nil
}

type discardLogger struct{}

func (discardLogger) Print(...any) {}

// ServerInfo implements database.Driver.
func (d *Driver) ServerInfo(ctx context.Context, db *sql.DB) (database.ServerInfo, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return database.ServerInfo{}, d.DescribeError(err)
	}
	defer conn.Close()
	var info database.ServerInfo
	if err := conn.QueryRowContext(ctx, "SELECT VERSION(), CURRENT_USER()").Scan(&info.Version, &info.User); err != nil {
		return info, d.DescribeError(err)
	}
	info.Flavor = "MySQL"
	if strings.Contains(strings.ToLower(info.Version), "mariadb") {
		info.Flavor = "MariaDB"
	}
	rememberFlavor(db, info.Flavor == "MariaDB")
	var name, cipher string
	if err := conn.QueryRowContext(ctx, "SHOW SESSION STATUS LIKE 'Ssl_cipher'").Scan(&name, &cipher); err == nil {
		info.TLSCipher = cipher
		info.TLSActive = cipher != ""
	}
	return info, nil
}

// ConnectionID implements database.Canceller.
func (d *Driver) ConnectionID(ctx context.Context, conn *sql.Conn) (int64, error) {
	var id int64
	err := conn.QueryRowContext(ctx, "SELECT CONNECTION_ID()").Scan(&id)
	return id, err
}

// CancelQuery implements database.Canceller. KILL QUERY stops the running
// statement but keeps the connection (and any open transaction) alive.
func (d *Driver) CancelQuery(ctx context.Context, db *sql.DB, connectionID int64) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf("KILL QUERY %d", connectionID))
	return err
}

// CurrentDatabase implements database.CurrentDatabaser.
func (d *Driver) CurrentDatabase(ctx context.Context, conn *sql.Conn) (string, error) {
	var name sql.NullString
	err := conn.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&name)
	return name.String, err
}

// UseDatabase implements database.CurrentDatabaser.
func (d *Driver) UseDatabase(ctx context.Context, conn *sql.Conn, name string) error {
	_, err := conn.ExecContext(ctx, "USE "+QuoteIdent(name))
	return err
}

// QuoteIdent quotes an identifier with backticks.
func QuoteIdent(s string) string { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }

// Inspector implements database.Driver.
func (d *Driver) Inspector(db *sql.DB) database.Inspector { return newInspector(db) }

var (
	lineRe = regexp.MustCompile(`at line (\d+)\s*$`)
	nearRe = regexp.MustCompile(`(?s)near '(.*)' at line \d+\s*$`)
)

// DescribeError implements database.Driver.
func (d *Driver) DescribeError(err error) *database.DBError {
	if err == nil {
		return nil
	}
	var de *database.DBError
	if errors.As(err, &de) {
		return de
	}
	var me *gomysql.MySQLError
	if errors.As(err, &me) {
		out := &database.DBError{
			Code:     int(me.Number),
			SQLState: strings.TrimRight(string(me.SQLState[:]), "\x00"),
			Message:  me.Message,
			Category: categoryFor(int(me.Number)),
		}
		if m := lineRe.FindStringSubmatch(me.Message); m != nil {
			out.Line, _ = strconv.Atoi(m[1])
		}
		if m := nearRe.FindStringSubmatch(me.Message); m != nil {
			out.Near = m[1]
		}
		return out
	}
	switch {
	case errors.Is(err, context.Canceled):
		return &database.DBError{Category: "cancelled", Message: "operação cancelada"}
	case errors.Is(err, context.DeadlineExceeded):
		return &database.DBError{Category: "timeout", Message: "tempo limite excedido"}
	case errors.Is(err, gomysql.ErrInvalidConn), errors.Is(err, sql.ErrConnDone):
		return &database.DBError{Category: "network", Message: "a conexão com o servidor foi perdida"}
	}
	var ne net.Error
	if errors.As(err, &ne) {
		if ne.Timeout() {
			return &database.DBError{Category: "timeout", Message: "tempo limite de rede excedido: " + err.Error()}
		}
		return &database.DBError{Category: "network", Message: "falha de rede: " + err.Error()}
	}
	msg := err.Error()
	var certErr *tls.CertificateVerificationError
	if errors.As(err, &certErr) || strings.Contains(msg, "x509:") || strings.Contains(msg, "tls:") {
		return &database.DBError{Category: "tls", Message: "falha de TLS: " + msg}
	}
	if strings.Contains(msg, "TLS requested but server does not support TLS") {
		return &database.DBError{Category: "tls", Message: "o servidor não oferece TLS, mas a conexão exige TLS"}
	}
	if strings.Contains(msg, "unknown auth plugin") || strings.Contains(msg, "this authentication plugin is not supported") {
		return &database.DBError{Category: "auth", Message: "método de autenticação não suportado pelo driver: " + msg}
	}
	return &database.DBError{Category: "other", Message: msg}
}

func categoryFor(code int) string {
	switch code {
	case 1064, 1149, 1054, 1146, 1049, 1051, 1305, 1630:
		if code == 1064 || code == 1149 {
			return "syntax"
		}
		return "reference"
	case 1045, 1044, 1698, 2059, 1251, 1524:
		return "auth"
	case 1142, 1143, 1227, 1370, 1290:
		return "permission"
	case 1062, 1451, 1452, 1216, 1217, 1048, 1364, 3819, 4025:
		return "constraint"
	case 1317:
		return "cancelled"
	case 3024, 1969:
		return "timeout"
	case 1205, 1213:
		return "lock"
	}
	return "other"
}

var (
	_ database.Warner   = (*Driver)(nil)
	_ database.Profiler = (*Driver)(nil)
)

// Warnings implements database.Warner.
func (d *Driver) Warnings(ctx context.Context, conn *sql.Conn) ([]database.Warning, error) {
	rows, err := conn.QueryContext(ctx, "SHOW WARNINGS LIMIT 50")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []database.Warning
	for rows.Next() {
		var w database.Warning
		if err := rows.Scan(&w.Level, &w.Code, &w.Message); err != nil {
			return out, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// ClearWarningsSQL implements database.Warner. MySQL resets warnings on
// almost every statement; MariaDB only on statements that use a table, and
// a derived table counts.
func (d *Driver) ClearWarningsSQL() string { return "SELECT 1 FROM (SELECT 1) AS dbstudio_reset" }

// ProfileSQL implements database.Profiler with the session profiler
// (deprecated in MySQL 8 but still available, and native in MariaDB).
func (d *Driver) ProfileSQL(several bool) (enable, show, disable string) {
	show = "SHOW PROFILE"
	if several {
		show = "SHOW PROFILES"
	}
	return "SET SESSION profiling = 1", show, "SET SESSION profiling = 0"
}

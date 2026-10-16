// Package database defines the engine-neutral contracts implemented by each
// database driver (MySQL/MariaDB today; PostgreSQL, SQLite and SQL Server are
// expected later).
//
// The contracts are intentionally small. Generic SQL execution is handled by
// the query package on top of database/sql; drivers only provide what is
// engine-specific: opening connections, catalog introspection, server-side
// cancellation and error decoding.
package database

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"time"

	"anansi/internal/database/sqllog"
)

// Kind identifies a database engine.
type Kind string

const (
	MySQL   Kind = "mysql"
	MariaDB Kind = "mariadb"
)

// TLSMode controls transport security.
type TLSMode string

const (
	TLSDisabled   TLSMode = "disabled"    // never use TLS
	TLSPreferred  TLSMode = "preferred"   // use TLS when the server offers it; plaintext otherwise (reported in the UI)
	TLSRequired   TLSMode = "required"    // TLS mandatory, certificate not verified
	TLSVerifyCA   TLSMode = "verify-ca"   // TLS mandatory, certificate chain verified
	TLSVerifyFull TLSMode = "verify-full" // TLS mandatory, chain and host name verified
)

// TLSOptions configures TLS for a connection.
type TLSOptions struct {
	Mode       TLSMode `json:"mode"`
	CAFile     string  `json:"caFile,omitempty"`
	CertFile   string  `json:"certFile,omitempty"`
	KeyFile    string  `json:"keyFile,omitempty"`
	ServerName string  `json:"serverName,omitempty"`
}

// ConnectParams holds everything needed to open a connection. It contains the
// password and therefore must never be serialized or logged.
type ConnectParams struct {
	Kind     Kind
	Host     string
	Port     int
	User     string
	Password string
	Database string
	TLS      TLSOptions
	// Params are extra driver parameters validated by the driver.
	Params map[string]string
	// AllowCleartextPasswords enables the mysql_clear_password plugin
	// (only accepted together with TLS).
	AllowCleartextPasswords bool
	ConnectTimeout          time.Duration
	PoolMaxOpen             int
	PoolMaxIdle             int
	PoolMaxLifetime         time.Duration
	// Log, when set, receives every statement sent through the pool.
	Log sqllog.Func
	// Socket is a Unix socket path used instead of Host:Port.
	Socket string
	// Dial, when set, opens the network connections (SSH tunnel); Host:Port
	// is then the address as seen from the tunnel endpoint.
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)
	// InitSQL runs on every new physical connection.
	InitSQL []string
}

// ServerInfo describes the server a session is connected to.
type ServerInfo struct {
	Version   string `json:"version"`
	Flavor    string `json:"flavor"` // "MySQL" or "MariaDB"
	TLSActive bool   `json:"tlsActive"`
	TLSCipher string `json:"tlsCipher,omitempty"`
	User      string `json:"user"`
}

// Driver is implemented by every supported engine.
type Driver interface {
	Kinds() []Kind
	// Validate checks parameters without contacting the server.
	Validate(p ConnectParams) error
	// Open creates a connection pool and verifies it with a ping.
	Open(ctx context.Context, p ConnectParams) (*sql.DB, error)
	ServerInfo(ctx context.Context, db *sql.DB) (ServerInfo, error)
	Inspector(db *sql.DB) Inspector
	// DescribeError converts a driver error into a user-facing error.
	DescribeError(err error) *DBError
}

// Canceller is an optional capability: interrupting a running statement on
// the server, in addition to cancelling the client-side context.
type Canceller interface {
	// ConnectionID returns the server-side id of a dedicated connection.
	ConnectionID(ctx context.Context, conn *sql.Conn) (int64, error)
	// CancelQuery asks the server to stop the statement running on the
	// connection with the given id. It must use a different connection.
	CancelQuery(ctx context.Context, db *sql.DB, connectionID int64) error
}

// Releaser is an optional capability: forgetting per-pool driver state
// right before the pool is closed.
type Releaser interface {
	Release(db *sql.DB)
}

// CurrentDatabaser is an optional capability: reading the default database of
// a dedicated connection (after USE statements).
type CurrentDatabaser interface {
	CurrentDatabase(ctx context.Context, conn *sql.Conn) (string, error)
	UseDatabase(ctx context.Context, conn *sql.Conn, name string) error
}

// Inspector reads catalog metadata.
type Inspector interface {
	Databases(ctx context.Context) ([]DatabaseInfo, error)
	Tables(ctx context.Context, database string) ([]TableInfo, error)
	Columns(ctx context.Context, database, table string) ([]ColumnInfo, error)
	Indexes(ctx context.Context, database, table string) ([]IndexInfo, error)
	// AllColumns returns every column of every table in a database; used to
	// warm the autocomplete cache with a single round trip.
	AllColumns(ctx context.Context, database string) (map[string][]ColumnInfo, error)
	Routines(ctx context.Context, database string) ([]RoutineInfo, error)
}

// DatabaseInfo describes a database (MySQL "schema").
type DatabaseInfo struct {
	Name   string `json:"name"`
	System bool   `json:"system"`
}

// TableInfo describes a table or view.
type TableInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"` // "table" | "view" | "system view"
	Engine  string `json:"engine,omitempty"`
	Rows    *int64 `json:"rows,omitempty"` // estimate
	Comment string `json:"comment,omitempty"`
}

// ColumnInfo describes a table column.
type ColumnInfo struct {
	Name       string  `json:"name"`
	Position   int     `json:"position"`
	DataType   string  `json:"dataType"`   // e.g. "varchar"
	ColumnType string  `json:"columnType"` // e.g. "varchar(255)"
	Nullable   bool    `json:"nullable"`
	Default    *string `json:"default,omitempty"`
	Key        string  `json:"key,omitempty"` // PRI, UNI, MUL
	Extra      string  `json:"extra,omitempty"`
	Comment    string  `json:"comment,omitempty"`
}

// IndexInfo describes an index.
type IndexInfo struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique"`
	Primary bool     `json:"primary"`
	Type    string   `json:"type"`
}

// RoutineInfo describes a stored procedure or function.
type RoutineInfo struct {
	Name string `json:"name"`
	Type string `json:"type"` // PROCEDURE | FUNCTION
}

// DBError is a decoded, user-presentable database error. It never contains
// credentials.
type DBError struct {
	Code     int    `json:"code,omitempty"`     // vendor error number, e.g. 1064
	SQLState string `json:"sqlState,omitempty"` // e.g. 42000
	Message  string `json:"message"`
	// Line is the 1-based line inside the statement reported by the server
	// ("... at line N"), 0 when unknown.
	Line int `json:"line,omitempty"`
	// Near is the fragment reported by syntax errors ("near '...'").
	Near string `json:"near,omitempty"`
	// Category: "syntax", "auth", "network", "timeout", "cancelled",
	// "permission", "constraint", "other".
	Category string `json:"category"`
}

func (e *DBError) Error() string { return e.Message }

// ErrUnsupportedKind is returned when no driver handles a kind.
var ErrUnsupportedKind = errors.New("tipo de banco de dados não suportado")

// Registry maps kinds to drivers.
type Registry struct {
	drivers map[Kind]Driver
}

// NewRegistry builds a registry from drivers.
func NewRegistry(drivers ...Driver) *Registry {
	r := &Registry{drivers: map[Kind]Driver{}}
	for _, d := range drivers {
		for _, k := range d.Kinds() {
			r.drivers[k] = d
		}
	}
	return r
}

// Get returns the driver for kind.
func (r *Registry) Get(k Kind) (Driver, error) {
	d, ok := r.drivers[k]
	if !ok {
		return nil, ErrUnsupportedKind
	}
	return d, nil
}

// Kinds lists the registered kinds.
func (r *Registry) Kinds() []Kind {
	out := make([]Kind, 0, len(r.drivers))
	for k := range r.drivers {
		out = append(out, k)
	}
	return out
}

// ---- DDL (optional capability) ----------------------------------------------

// DDLBuilder is implemented by drivers that can generate DDL from structured
// specifications (used by the "new database" and "new table" dialogs).
type DDLBuilder interface {
	CreateDatabaseSQL(spec CreateDatabaseSpec) (string, error)
	CreateTableSQL(spec CreateTableSpec) (string, error)
	DDLOptions(ctx context.Context, db *sql.DB) (DDLOptions, error)
}

// CreateDatabaseSpec describes CREATE DATABASE.
type CreateDatabaseSpec struct {
	Name        string `json:"name"`
	Charset     string `json:"charset"`
	Collation   string `json:"collation"`
	IfNotExists bool   `json:"ifNotExists"`
}

// Default value kinds for ColumnSpec.DefaultKind.
const (
	DefaultNone             = ""
	DefaultNull             = "null"
	DefaultValue            = "value"
	DefaultCurrentTimestamp = "current_timestamp"
	DefaultExpression       = "expression"
)

// ColumnSpec describes a column of CREATE TABLE.
type ColumnSpec struct {
	Name string `json:"name"`
	Type string `json:"type"` // e.g. VARCHAR
	// Length holds the type arguments: "255", "10,2" or, for ENUM/SET, the
	// values separated by commas (Values takes precedence when set).
	Length        string   `json:"length"`
	Values        []string `json:"values"`
	Unsigned      bool     `json:"unsigned"`
	Nullable      bool     `json:"nullable"`
	AutoIncrement bool     `json:"autoIncrement"`
	PrimaryKey    bool     `json:"primaryKey"`
	Unique        bool     `json:"unique"`
	DefaultKind   string   `json:"defaultKind"`
	Default       string   `json:"default"`
	// OnUpdateCurrentTimestamp adds ON UPDATE CURRENT_TIMESTAMP.
	OnUpdateCurrentTimestamp bool   `json:"onUpdateCurrentTimestamp"`
	Comment                  string `json:"comment"`
}

// IndexSpec describes a secondary index.
type IndexSpec struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique"`
}

// CreateTableSpec describes CREATE TABLE.
type CreateTableSpec struct {
	Database    string       `json:"database"`
	Name        string       `json:"name"`
	Columns     []ColumnSpec `json:"columns"`
	Indexes     []IndexSpec  `json:"indexes"`
	Engine      string       `json:"engine"`
	Charset     string       `json:"charset"`
	Collation   string       `json:"collation"`
	Comment     string       `json:"comment"`
	IfNotExists bool         `json:"ifNotExists"`
}

// CharsetInfo describes a character set and its collations.
type CharsetInfo struct {
	Name             string   `json:"name"`
	DefaultCollation string   `json:"defaultCollation"`
	Description      string   `json:"description"`
	Collations       []string `json:"collations"`
}

// DDLOptions lists server choices for the DDL dialogs.
type DDLOptions struct {
	Charsets         []CharsetInfo `json:"charsets"`
	Engines          []string      `json:"engines"`
	DefaultCharset   string        `json:"defaultCharset"`
	DefaultCollation string        `json:"defaultCollation"`
	DefaultEngine    string        `json:"defaultEngine"`
	Types            []string      `json:"types"`
}

// Warning is a server warning or note raised by a statement (SHOW WARNINGS).
type Warning struct {
	Level   string `json:"level"` // Note | Warning | Error
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Warner is an optional capability: reading the warnings of the last
// statement run on a dedicated connection.
type Warner interface {
	Warnings(ctx context.Context, conn *sql.Conn) ([]Warning, error)
	// ClearWarningsSQL resets the diagnostics area, so a later statement that
	// does not reset it by itself (MariaDB: statements without tables) does
	// not appear to have raised the previous statement's warnings.
	ClearWarningsSQL() string
}

// Profiler is an optional capability: statement profiling for SQL tabs.
// Enable/Disable are run before/after the statements; Show returns the
// profile (one statement) or the list of profiles (several statements).
type Profiler interface {
	ProfileSQL(several bool) (enable, show, disable string)
}

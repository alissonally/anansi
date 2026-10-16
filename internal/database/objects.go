package database

import "context"

// ObjectKind identifies a catalog object.
type ObjectKind string

const (
	ObjDatabase  ObjectKind = "database"
	ObjTable     ObjectKind = "table"
	ObjView      ObjectKind = "view"
	ObjProcedure ObjectKind = "procedure"
	ObjFunction  ObjectKind = "function"
	ObjTrigger   ObjectKind = "trigger"
	ObjEvent     ObjectKind = "event"
)

// Valid reports whether k is a known object kind.
func (k ObjectKind) Valid() bool {
	switch k {
	case ObjDatabase, ObjTable, ObjView, ObjProcedure, ObjFunction, ObjTrigger, ObjEvent:
		return true
	}
	return false
}

// ---- object browser (optional inspector capability) -------------------------

// ObjectBrowser is implemented by inspectors that expose the full catalog
// (HeidiSQL's database and table tabs). Callers type-assert the value
// returned by Driver.Inspector.
type ObjectBrowser interface {
	DatabaseSummaries(ctx context.Context) ([]DatabaseSummary, error)
	TableStatus(ctx context.Context, db string) ([]TableStatus, error)
	ForeignKeys(ctx context.Context, db, table string) ([]ForeignKeyInfo, error)
	// ReferencingKeys lists foreign keys of other tables pointing to table.
	ReferencingKeys(ctx context.Context, db, table string) ([]ForeignKeyInfo, error)
	CheckConstraints(ctx context.Context, db, table string) ([]CheckInfo, error)
	Triggers(ctx context.Context, db string) ([]TriggerInfo, error)
	Events(ctx context.Context, db string) ([]EventInfo, error)
	RoutineDetails(ctx context.Context, db string) ([]RoutineDetail, error)
	// CreateStatement returns the server's CREATE statement for an object
	// (SHOW CREATE ...). db is ignored for ObjDatabase.
	CreateStatement(ctx context.Context, db string, kind ObjectKind, name string) (string, error)
}

// DatabaseSummary describes a database in the server overview.
type DatabaseSummary struct {
	Name      string `json:"name"`
	Charset   string `json:"charset"`
	Collation string `json:"collation"`
	Tables    int    `json:"tables"`
	Views     int    `json:"views"`
	// Size is data + index length in bytes (InnoDB values are estimates).
	Size       int64  `json:"size"`
	LastUpdate string `json:"lastUpdate,omitempty"`
	System     bool   `json:"system"`
}

// TableStatus is the detailed status of a table or view.
type TableStatus struct {
	Name          string `json:"name"`
	Type          string `json:"type"` // table | view | system view | sequence
	Engine        string `json:"engine,omitempty"`
	Version       *int64 `json:"version,omitempty"`
	RowFormat     string `json:"rowFormat,omitempty"`
	Rows          *int64 `json:"rows,omitempty"`
	AvgRowLength  *int64 `json:"avgRowLength,omitempty"`
	DataLength    *int64 `json:"dataLength,omitempty"`
	MaxDataLength *int64 `json:"maxDataLength,omitempty"`
	IndexLength   *int64 `json:"indexLength,omitempty"`
	DataFree      *int64 `json:"dataFree,omitempty"`
	// AutoIncrement is a decimal string (it may exceed 2^53).
	AutoIncrement string `json:"autoIncrement,omitempty"`
	CreateTime    string `json:"createTime,omitempty"`
	UpdateTime    string `json:"updateTime,omitempty"`
	CheckTime     string `json:"checkTime,omitempty"`
	Collation     string `json:"collation,omitempty"`
	Checksum      string `json:"checksum,omitempty"`
	CreateOptions string `json:"createOptions,omitempty"`
	Comment       string `json:"comment,omitempty"`
}

// ForeignKeyInfo describes a foreign key constraint.
type ForeignKeyInfo struct {
	Name        string   `json:"name"`
	Database    string   `json:"database"`
	Table       string   `json:"table"`
	Columns     []string `json:"columns"`
	RefDatabase string   `json:"refDatabase"`
	RefTable    string   `json:"refTable"`
	RefColumns  []string `json:"refColumns"`
	OnUpdate    string   `json:"onUpdate"`
	OnDelete    string   `json:"onDelete"`
}

// CheckInfo describes a CHECK constraint.
type CheckInfo struct {
	Name   string `json:"name"`
	Clause string `json:"clause"`
}

// TriggerInfo describes a trigger.
type TriggerInfo struct {
	Name      string `json:"name"`
	Table     string `json:"table"`
	Timing    string `json:"timing"` // BEFORE | AFTER
	Event     string `json:"event"`  // INSERT | UPDATE | DELETE
	Statement string `json:"statement"`
	Definer   string `json:"definer"`
	Created   string `json:"created,omitempty"`
	SQLMode   string `json:"sqlMode,omitempty"`
	Order     int    `json:"order"`
}

// EventInfo describes a scheduled event.
type EventInfo struct {
	Name          string `json:"name"`
	Definer       string `json:"definer"`
	Type          string `json:"type"` // ONE TIME | RECURRING
	ExecuteAt     string `json:"executeAt,omitempty"`
	IntervalValue string `json:"intervalValue,omitempty"`
	IntervalField string `json:"intervalField,omitempty"`
	Starts        string `json:"starts,omitempty"`
	Ends          string `json:"ends,omitempty"`
	Status        string `json:"status"` // ENABLED | DISABLED | SLAVESIDE_DISABLED
	OnCompletion  string `json:"onCompletion"`
	Body          string `json:"body"`
	Comment       string `json:"comment,omitempty"`
	LastExecuted  string `json:"lastExecuted,omitempty"`
	Created       string `json:"created,omitempty"`
}

// RoutineParam is a parameter of a stored routine.
type RoutineParam struct {
	Mode string `json:"mode"` // IN | OUT | INOUT ("" for functions)
	Name string `json:"name"`
	Type string `json:"type"` // full type, e.g. varchar(20)
}

// RoutineDetail describes a stored procedure or function.
type RoutineDetail struct {
	Name          string         `json:"name"`
	Type          string         `json:"type"` // PROCEDURE | FUNCTION
	Params        []RoutineParam `json:"params"`
	Returns       string         `json:"returns,omitempty"`
	Body          string         `json:"body"`
	Deterministic bool           `json:"deterministic"`
	DataAccess    string         `json:"dataAccess"` // CONTAINS SQL | NO SQL | READS SQL DATA | MODIFIES SQL DATA
	Security      string         `json:"security"`   // DEFINER | INVOKER
	Definer       string         `json:"definer"`
	Comment       string         `json:"comment,omitempty"`
	Created       string         `json:"created,omitempty"`
	Modified      string         `json:"modified,omitempty"`
}

// ---- server administration (optional inspector capability) ------------------

// ServerAdmin exposes server-wide information (HeidiSQL's host tab).
type ServerAdmin interface {
	// Variables returns every system variable with its session and global
	// values.
	Variables(ctx context.Context) ([]ServerVariable, error)
	// Status returns the global status counters.
	Status(ctx context.Context) ([]StatusValue, error)
	Processes(ctx context.Context) ([]Process, error)
}

// ServerVariable is a system variable. Session or Global is nil when the
// variable does not exist in that scope.
type ServerVariable struct {
	Name    string  `json:"name"`
	Session *string `json:"session,omitempty"`
	Global  *string `json:"global,omitempty"`
}

// StatusValue is a status counter.
type StatusValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Process is a server thread (SHOW PROCESSLIST).
type Process struct {
	ID      int64  `json:"id"`
	User    string `json:"user"`
	Host    string `json:"host"`
	DB      string `json:"db,omitempty"`
	Command string `json:"command"`
	Time    int64  `json:"time"`
	State   string `json:"state,omitempty"`
	Info    string `json:"info,omitempty"`
	// Progress is reported by MariaDB for some long operations (0-100).
	Progress *float64 `json:"progress,omitempty"`
}

// ---- SQL generation for object operations (optional driver capability) -----

// ObjectSQL generates statements for operations on existing objects. Every
// method validates its input and quotes identifiers and literals.
type ObjectSQL interface {
	QuoteIdent(name string) string
	QuoteString(s string) string
	DropSQL(kind ObjectKind, db, name string) (string, error)
	TruncateSQL(db, table string) (string, error)
	// RenameSQL renames a table or view (possibly moving it to toDB), or a
	// database object of another kind when the engine supports it.
	RenameSQL(kind ObjectKind, db, name, toDB, toName string) (string, error)
	// SetVariableSQL builds SET [GLOBAL|SESSION] name = value. valueKind is
	// "auto" (number or quoted string), "string", "raw" (expression) or
	// "default".
	SetVariableSQL(global bool, name, value, valueKind string) (string, error)
	KillSQL(id int64, queryOnly bool) (string, error)
	AlterDatabaseSQL(db, charset, collation string) (string, error)
}

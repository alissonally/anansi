package database

import (
	"context"
	"database/sql"
)

// ---- structured object definitions (object editors) -------------------------

// ColumnDef is a column in the table editor. OrigName links an existing
// column to its current name ("" for a new column).
type ColumnDef struct {
	OrigName string `json:"origName"`
	Name     string `json:"name"`
	// Type is the base type, e.g. VARCHAR; Length holds its arguments:
	// "255", "10,2" or, for ENUM/SET, the quoted values "'a','b'".
	Type     string `json:"type"`
	Length   string `json:"length"`
	Unsigned bool   `json:"unsigned"`
	Zerofill bool   `json:"zerofill"`
	Nullable bool   `json:"nullable"`
	// DefaultKind: "" (none), "null", "value" (literal), "expression".
	DefaultKind   string `json:"defaultKind"`
	Default       string `json:"default"`
	AutoIncrement bool   `json:"autoIncrement"`
	// OnUpdate is an expression such as CURRENT_TIMESTAMP ("" = none).
	OnUpdate  string `json:"onUpdate"`
	Comment   string `json:"comment"`
	Charset   string `json:"charset"`
	Collation string `json:"collation"`
	// Generated is the expression of a generated column; GeneratedKind is
	// VIRTUAL, STORED or PERSISTENT (MariaDB).
	Generated     string `json:"generated"`
	GeneratedKind string `json:"generatedKind"`
	Invisible     bool   `json:"invisible"`
	SRID          string `json:"srid"`
}

// IndexPart is a column (optionally with prefix length and direction) or an
// expression of a functional index.
type IndexPart struct {
	Column     string `json:"column"`
	Length     int    `json:"length"`
	Desc       bool   `json:"desc"`
	Expression string `json:"expression"`
}

// IndexDef is an index in the table editor.
type IndexDef struct {
	OrigName string `json:"origName"`
	Name     string `json:"name"`
	// Kind: PRIMARY, KEY, UNIQUE, FULLTEXT, SPATIAL.
	Kind      string      `json:"kind"`
	Parts     []IndexPart `json:"parts"`
	Algorithm string      `json:"algorithm"` // "", BTREE, HASH
	Comment   string      `json:"comment"`
	// Invisible: MySQL INVISIBLE / MariaDB IGNORED.
	Invisible bool `json:"invisible"`
}

// ForeignKeyDef is a foreign key in the table editor.
type ForeignKeyDef struct {
	OrigName    string   `json:"origName"`
	Name        string   `json:"name"`
	Columns     []string `json:"columns"`
	RefDatabase string   `json:"refDatabase"`
	RefTable    string   `json:"refTable"`
	RefColumns  []string `json:"refColumns"`
	OnUpdate    string   `json:"onUpdate"`
	OnDelete    string   `json:"onDelete"`
}

// CheckDef is a CHECK constraint in the table editor.
type CheckDef struct {
	OrigName string `json:"origName"`
	Name     string `json:"name"`
	Clause   string `json:"clause"`
}

// TableOptions are table-level options.
type TableOptions struct {
	Engine        string `json:"engine"`
	Collation     string `json:"collation"`
	Comment       string `json:"comment"`
	RowFormat     string `json:"rowFormat"`
	AutoIncrement string `json:"autoIncrement"`
	AvgRowLength  string `json:"avgRowLength"`
	MaxRows       string `json:"maxRows"`
	Checksum      bool   `json:"checksum"`
	// Partition is the raw "PARTITION BY ..." clause ("" = not partitioned).
	Partition string `json:"partition"`
	// ConvertData converts existing data to the new collation's charset
	// (CONVERT TO CHARACTER SET) instead of changing only the default.
	ConvertData bool `json:"convertData"`
}

// TableDef is the full structure edited by the table editor.
type TableDef struct {
	Database    string          `json:"database"`
	Name        string          `json:"name"`
	OrigName    string          `json:"origName"`
	Columns     []ColumnDef     `json:"columns"`
	Indexes     []IndexDef      `json:"indexes"`
	ForeignKeys []ForeignKeyDef `json:"foreignKeys"`
	Checks      []CheckDef      `json:"checks"`
	Options     TableOptions    `json:"options"`
	// MariaDB selects dialect differences (PERSISTENT, IGNORED indexes,
	// DROP CONSTRAINT); filled by LoadTableDef and by the API for new tables.
	MariaDB bool `json:"mariadb"`
}

// ViewDef is a view in the view editor.
type ViewDef struct {
	Database    string `json:"database"`
	Name        string `json:"name"`
	OrigName    string `json:"origName"`
	Definer     string `json:"definer"`     // user@host ("" = current user)
	Security    string `json:"security"`    // DEFINER | INVOKER
	Algorithm   string `json:"algorithm"`   // UNDEFINED | MERGE | TEMPTABLE
	CheckOption string `json:"checkOption"` // "" | CASCADED | LOCAL
	Select      string `json:"select"`
}

// RoutineDef is a stored procedure or function in the routine editor.
type RoutineDef struct {
	Database      string         `json:"database"`
	Name          string         `json:"name"`
	OrigName      string         `json:"origName"`
	Type          string         `json:"type"` // PROCEDURE | FUNCTION
	OrigType      string         `json:"origType"`
	Params        []RoutineParam `json:"params"`
	Returns       string         `json:"returns"`
	Body          string         `json:"body"`
	Deterministic bool           `json:"deterministic"`
	DataAccess    string         `json:"dataAccess"`
	Security      string         `json:"security"`
	Definer       string         `json:"definer"`
	Comment       string         `json:"comment"`
}

// TriggerDef is a trigger in the trigger editor.
type TriggerDef struct {
	Database string `json:"database"`
	Name     string `json:"name"`
	OrigName string `json:"origName"`
	Table    string `json:"table"`
	Timing   string `json:"timing"` // BEFORE | AFTER
	Event    string `json:"event"`  // INSERT | UPDATE | DELETE
	Body     string `json:"body"`
	Definer  string `json:"definer"`
}

// EventDef is a scheduled event in the event editor.
type EventDef struct {
	Database string `json:"database"`
	Name     string `json:"name"`
	OrigName string `json:"origName"`
	Definer  string `json:"definer"`
	// Recurring: EVERY IntervalValue IntervalField [STARTS] [ENDS];
	// otherwise AT ExecuteAt.
	Recurring     bool   `json:"recurring"`
	ExecuteAt     string `json:"executeAt"`
	IntervalValue string `json:"intervalValue"`
	IntervalField string `json:"intervalField"`
	Starts        string `json:"starts"`
	Ends          string `json:"ends"`
	Preserve      bool   `json:"preserve"`
	Status        string `json:"status"` // ENABLE | DISABLE | DISABLE ON SLAVE
	Comment       string `json:"comment"`
	Body          string `json:"body"`
}

// ObjectEditor loads structured definitions and builds their DDL. Every
// builder validates names and quotes identifiers and literals; expressions
// (defaults, generated columns, checks, bodies) are SQL typed by the user
// and only checked for balanced quotes/parentheses and stray semicolons.
type ObjectEditor interface {
	LoadTableDef(ctx context.Context, db *sql.DB, database, table string) (TableDef, error)
	CreateTableDefSQL(def TableDef) (string, error)
	// AlterTableSQL returns the statements turning cur into def (empty when
	// nothing changed).
	AlterTableSQL(cur, def TableDef) ([]string, error)

	LoadViewDef(ctx context.Context, db *sql.DB, database, name string) (ViewDef, error)
	ViewSQL(def ViewDef) ([]string, error)

	LoadRoutineDef(ctx context.Context, db *sql.DB, database, typ, name string) (RoutineDef, error)
	RoutineSQL(def RoutineDef) (string, error)

	LoadTriggerDef(ctx context.Context, db *sql.DB, database, name string) (TriggerDef, error)
	TriggerSQL(def TriggerDef) (string, error)

	LoadEventDef(ctx context.Context, db *sql.DB, database, name string) (EventDef, error)
	// EventSQL builds CREATE EVENT (cur == nil) or ALTER EVENT.
	EventSQL(def EventDef, cur *EventDef) (string, error)

	// Definers lists user@host accounts for definer pickers (empty when the
	// account cannot read mysql.user).
	Definers(ctx context.Context, db *sql.DB) ([]string, error)
}

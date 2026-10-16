package database

// ---- table data browsing and editing (optional driver capability) ----------

// SortSpec is one ORDER BY term.
type SortSpec struct {
	Column string `json:"column"`
	Desc   bool   `json:"desc"`
}

// BrowseSpec describes the SELECT of the table data grid.
type BrowseSpec struct {
	Database string `json:"database"`
	Table    string `json:"table"`
	// Columns lists the visible columns in display order (empty = all). Key
	// columns are always selected so rows stay editable.
	Columns []string `json:"columns"`
	// Filter is a WHERE expression typed by the user (without "WHERE").
	Filter string     `json:"filter"`
	Sort   []SortSpec `json:"sort"`
	Offset int        `json:"offset"`
	Limit  int        `json:"limit"`
	// PartialLength > 0 loads long columns (TEXT, BLOB, JSON...) truncated
	// to that many characters/bytes when the table has a key; the full value
	// is read on demand.
	PartialLength int `json:"partialLength"`
	// NoLimit reads every row (exports).
	NoLimit bool `json:"noLimit"`
}

// SelectPlan is the SQL built for a BrowseSpec.
type SelectPlan struct {
	SQL string
	// Columns are the selected column names, in result order.
	Columns []string
	// Partial marks columns selected truncated (LEFT(...)).
	Partial []bool
}

// Cell value kinds of CellValue.Kind.
const (
	ValueText       = "value"      // literal text, converted by the server to the column type
	ValueNull       = "null"       // SQL NULL
	ValueDefault    = "default"    // DEFAULT
	ValueExpression = "expression" // raw SQL expression, e.g. NOW() (HeidiSQL "SQL function")
	ValueHex        = "hex"        // binary data as hexadecimal digits (no 0x prefix)
)

// CellValue is a value written by the data grid.
type CellValue struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// ColumnValue pairs a column with a value.
type ColumnValue struct {
	Column string    `json:"column"`
	Value  CellValue `json:"value"`
}

// Row edit operations.
const (
	EditUpdate = "update"
	EditInsert = "insert"
	EditDelete = "delete"
)

// RowEdit is one change of the data grid. Where holds the original values
// identifying the row: the key columns, or every column when the table has
// no usable key (then LIMIT 1 is added).
type RowEdit struct {
	Op     string        `json:"op"`
	Where  []ColumnValue `json:"where"`
	Values []ColumnValue `json:"values"`
}

// Statement is SQL text with positional arguments ("?").
type Statement struct {
	SQL  string
	Args []any
	// Display is the statement with arguments rendered as literals, for
	// previews and logs only (never executed).
	Display string
}

// TableKey describes how rows of a table are identified.
type TableKey struct {
	// Kind is "primary", "unique" (UNIQUE index of NOT NULL columns) or
	// "none" (every column is compared and LIMIT 1 is added).
	Kind    string   `json:"kind"`
	Name    string   `json:"name,omitempty"`
	Columns []string `json:"columns"`
}

// DataSQL builds the statements of the table data grid. Values are always
// passed as arguments, never interpolated into the SQL text.
type DataSQL interface {
	// FindKey chooses the row identifier like HeidiSQL: the primary key, else
	// a UNIQUE index whose columns are all NOT NULL.
	FindKey(cols []ColumnInfo, idx []IndexInfo) TableKey
	SelectSQL(spec BrowseSpec, cols []ColumnInfo, key TableKey) (SelectPlan, error)
	CountSQL(db, table, filter string) (string, error)
	// RowSQL builds UPDATE/INSERT/DELETE for one edit. limitOne is added when
	// the WHERE does not cover a key.
	RowSQL(db, table string, cols []ColumnInfo, key TableKey, e RowEdit) (Statement, error)
	// FetchRowSQL reads the full values of one row (for truncated cells).
	FetchRowSQL(db, table string, cols []ColumnInfo, key TableKey, where []ColumnValue, columns []string) (Statement, error)
	// DistinctSQL lists the most frequent values of a column (quick filter
	// "more values").
	DistinctSQL(db, table, column, filter string, limit int) (string, error)
	// LookupSQL lists values of a referenced column with a label taken from
	// the first text column of the referenced table (FK dropdown).
	LookupSQL(db, table, column, labelColumn, search string, limit int) (Statement, error)
}

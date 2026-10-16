package query

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Column describes a result column. Names may repeat; rows are positional.
type Column struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // database type name, e.g. VARCHAR
	Nullable *bool  `json:"nullable,omitempty"`
	// Binary columns are rendered as hexadecimal (0x...).
	Binary bool `json:"binary"`
	// Numeric columns are right-aligned in the grid.
	Numeric bool `json:"numeric"`
}

// MaxCellBytes caps the size of a single cell sent to the UI.
const MaxCellBytes = 256 * 1024

const maxSafeInt = 1<<53 - 1

func describeColumns(cts []*sql.ColumnType) []Column {
	out := make([]Column, len(cts))
	for i, ct := range cts {
		t := strings.ToUpper(ct.DatabaseTypeName())
		c := Column{Name: ct.Name(), Type: t}
		if n, ok := ct.Nullable(); ok {
			c.Nullable = &n
		}
		switch t {
		case "BINARY", "VARBINARY", "TINYBLOB", "BLOB", "MEDIUMBLOB", "LONGBLOB", "BIT", "GEOMETRY":
			c.Binary = true
		case "TINYINT", "SMALLINT", "MEDIUMINT", "INT", "BIGINT", "DECIMAL", "FLOAT", "DOUBLE", "YEAR",
			"UNSIGNED TINYINT", "UNSIGNED SMALLINT", "UNSIGNED MEDIUMINT", "UNSIGNED INT", "UNSIGNED BIGINT":
			c.Numeric = true
		}
		out[i] = c
	}
	return out
}

// convertValue turns a scanned value into a JSON-safe representation without
// losing information:
//   - SQL NULL becomes nil (JSON null), distinct from the string "NULL";
//   - integers outside JavaScript's safe range become decimal strings;
//   - DECIMAL and temporal values stay as the exact text sent by the server;
//   - binary data becomes "0x" + hex.
//
// The second result reports that the value was truncated to MaxCellBytes.
func convertValue(v any, col Column) (any, bool) { return convertValueMax(v, col, MaxCellBytes) }

// convertValueMax is convertValue with a custom size cap.
func convertValueMax(v any, col Column, maxBytes int) (any, bool) {
	switch x := v.(type) {
	case nil:
		return nil, false
	case int64:
		if x > maxSafeInt || x < -maxSafeInt {
			return strconv.FormatInt(x, 10), false
		}
		return x, false
	case uint64:
		if x > maxSafeInt {
			return strconv.FormatUint(x, 10), false
		}
		return x, false
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return strconv.FormatFloat(x, 'g', -1, 64), false
		}
		return x, false
	case float32:
		// Shortest decimal that round-trips at 32-bit precision: FLOAT 1.1
		// is shown as 1.1, not 1.100000023841858.
		f, _ := strconv.ParseFloat(strconv.FormatFloat(float64(x), 'g', -1, 32), 64)
		return f, false
	case bool:
		return x, false
	case time.Time:
		return x.Format("2006-01-02 15:04:05.999999"), false
	case []byte:
		if col.Binary || !utf8.Valid(x) {
			truncated := false
			if len(x)*2+2 > maxBytes {
				x = x[:(maxBytes-2)/2]
				truncated = true
			}
			return "0x" + strings.ToUpper(hex.EncodeToString(x)), truncated
		}
		return capString(string(x), maxBytes)
	case string:
		return capString(x, maxBytes)
	default:
		return capString(fmt.Sprint(x), maxBytes)
	}
}

func capString(s string, maxBytes int) (any, bool) {
	if len(s) <= maxBytes {
		return s, false
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut], true
}

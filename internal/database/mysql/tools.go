package mysql

import (
	"regexp"
	"strconv"
	"strings"

	"anansi/internal/database"
)

// ---- maintenance (HeidiSQL table tools) --------------------------------------------

// maintenanceOptions lists the options each operation accepts.
var maintenanceOptions = map[string]map[string]bool{
	"CHECK":    {"QUICK": true, "FAST": true, "MEDIUM": true, "EXTENDED": true, "CHANGED": true, "FOR UPGRADE": true},
	"ANALYZE":  {},
	"CHECKSUM": {"QUICK": true, "EXTENDED": true},
	"OPTIMIZE": {},
	"REPAIR":   {"QUICK": true, "EXTENDED": true, "USE_FRM": true},
}

// MaintenanceSQL builds CHECK/ANALYZE/CHECKSUM/OPTIMIZE/REPAIR TABLE for one
// table with validated options.
func (d *Driver) MaintenanceSQL(op string, options []string, db, table string) (string, error) {
	op = strings.ToUpper(strings.TrimSpace(op))
	allowed, ok := maintenanceOptions[op]
	if !ok {
		return "", ddlError("operação de manutenção inválida: %q", op)
	}
	ref, err := qualified(db, table)
	if err != nil {
		return "", err
	}
	var opts []string
	seen := map[string]bool{}
	for _, o := range options {
		o = strings.ToUpper(strings.TrimSpace(o))
		if o == "" || seen[o] {
			continue
		}
		if !allowed[o] {
			return "", ddlError("a opção %s não se aplica a %s", o, op)
		}
		seen[o] = true
		opts = append(opts, o)
	}
	if op == "CHECKSUM" && seen["QUICK"] && seen["EXTENDED"] {
		return "", ddlError("CHECKSUM aceita QUICK ou EXTENDED, não os dois")
	}
	s := op + " TABLE " + ref
	if len(opts) > 0 {
		s += " " + strings.Join(opts, " ")
	}
	return s, nil
}

// ---- find text on server -------------------------------------------------------------

// SearchSpec describes a text search across tables.
type SearchSpec struct {
	Text string `json:"text"`
	// Match: "contains" (default), "exact", "starts", "ends", "regex".
	Match         string `json:"match"`
	CaseSensitive bool   `json:"caseSensitive"`
	// Types: "all" (default), "text", "numeric", "date", "binary".
	Types string `json:"types"`
}

func searchable(c database.ColumnInfo, types string) (cat string, ok bool) {
	t := strings.ToLower(c.DataType)
	switch {
	case t == "char" || t == "varchar" || strings.HasSuffix(t, "text") || t == "enum" || t == "set" || t == "json":
		cat = "text"
	case strings.Contains(t, "int") || t == "decimal" || t == "numeric" || t == "float" || t == "double" || t == "real" || t == "bit" || t == "year":
		cat = "numeric"
	case t == "date" || t == "datetime" || t == "timestamp" || t == "time":
		cat = "date"
	case strings.HasSuffix(t, "blob") || strings.HasSuffix(t, "binary"):
		cat = "binary"
	default:
		return "", false // spatial and others
	}
	return cat, types == "" || types == "all" || types == cat
}

// SearchCondition builds the WHERE condition (and its arguments) matching
// spec in any of the table's columns, or "" when no column qualifies.
func (d *Driver) SearchCondition(cols []database.ColumnInfo, spec SearchSpec, mariadb bool) (string, []any, error) {
	if spec.Text == "" {
		return "", nil, ddlError("informe o texto a procurar")
	}
	var parts []string
	var args []any
	pattern := spec.Text
	esc := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(spec.Text)
	like := true
	switch spec.Match {
	case "", "contains":
		pattern = "%" + esc + "%"
	case "starts":
		pattern = esc + "%"
	case "ends":
		pattern = "%" + esc
	case "exact":
		like = false
	case "regex":
		if _, err := regexp.Compile(spec.Text); err != nil {
			return "", nil, ddlError("expressão regular inválida: %v", err)
		}
	default:
		return "", nil, ddlError("tipo de busca inválido: %q", spec.Match)
	}
	_, isNum := strconv.ParseFloat(strings.TrimSpace(spec.Text), 64)
	for _, c := range cols {
		cat, ok := searchable(c, spec.Types)
		if !ok {
			continue
		}
		q := QuoteIdent(c.Name)
		if cat == "numeric" {
			// Numbers only match exactly (as HeidiSQL does).
			if isNum != nil {
				continue
			}
			parts = append(parts, q+" = ?")
			args = append(args, strings.TrimSpace(spec.Text))
			continue
		}
		expr := q
		if cat != "binary" {
			expr = "CONVERT(" + q + " USING utf8mb4)"
		}
		switch {
		case spec.Match == "regex" && mariadb:
			flag := "(?i)"
			if spec.CaseSensitive {
				flag = "(?-i)"
			}
			parts = append(parts, expr+" REGEXP ?")
			args = append(args, flag+spec.Text)
		case spec.Match == "regex":
			mode := "i"
			if spec.CaseSensitive {
				mode = "c"
			}
			parts = append(parts, "REGEXP_LIKE("+expr+", ?, '"+mode+"')")
			args = append(args, spec.Text)
		case spec.CaseSensitive && cat != "binary":
			op := " = ?"
			if like {
				op = " LIKE ? ESCAPE '!'"
			}
			parts = append(parts, expr+" COLLATE utf8mb4_bin"+op)
			args = append(args, pattern)
		case like:
			parts = append(parts, "LOWER("+expr+") LIKE LOWER(?) ESCAPE '!'")
			args = append(args, pattern)
		default:
			parts = append(parts, "LOWER("+expr+") = LOWER(?)")
			args = append(args, spec.Text)
		}
	}
	return strings.Join(parts, " OR "), args, nil
}

// ---- bulk table editor ------------------------------------------------------------------

// BulkSpec describes HeidiSQL's bulk table editor changes.
type BulkSpec struct {
	MoveTo           string `json:"moveTo"`
	Engine           string `json:"engine"`
	ConvertCharset   string `json:"convertCharset"`
	ConvertCollation string `json:"convertCollation"`
	DefaultCollation string `json:"defaultCollation"`
	ResetAutoInc     bool   `json:"resetAutoIncrement"`
}

// BulkSQL returns the statements applying spec to one table.
func (d *Driver) BulkSQL(db, table string, spec BulkSpec) ([]string, error) {
	ref, err := qualified(db, table)
	if err != nil {
		return nil, err
	}
	var specs []string
	if spec.Engine != "" {
		if err := checkOption("engine", spec.Engine); err != nil {
			return nil, err
		}
		specs = append(specs, "ENGINE="+spec.Engine)
	}
	if spec.ConvertCharset != "" {
		if err := checkOption("charset", spec.ConvertCharset); err != nil {
			return nil, err
		}
		s := "CONVERT TO CHARACTER SET " + spec.ConvertCharset
		if spec.ConvertCollation != "" {
			if err := checkOption("collation", spec.ConvertCollation); err != nil {
				return nil, err
			}
			s += " COLLATE " + spec.ConvertCollation
		}
		specs = append(specs, s)
	}
	if spec.DefaultCollation != "" {
		if err := checkOption("collation", spec.DefaultCollation); err != nil {
			return nil, err
		}
		specs = append(specs, "COLLATE="+spec.DefaultCollation)
	}
	if spec.ResetAutoInc {
		specs = append(specs, "AUTO_INCREMENT=1")
	}
	var out []string
	if len(specs) > 0 {
		out = append(out, "ALTER TABLE "+ref+" "+strings.Join(specs, ", "))
	}
	if spec.MoveTo != "" && spec.MoveTo != db {
		to, err := qualified(spec.MoveTo, table)
		if err != nil {
			return nil, err
		}
		out = append(out, "RENAME TABLE "+ref+" TO "+to)
	}
	if len(out) == 0 {
		return nil, ddlError("nenhuma alteração escolhida")
	}
	return out, nil
}

// FlushSQL builds FLUSH statements.
func (d *Driver) FlushSQL(what string) (string, error) {
	switch w := strings.ToUpper(strings.TrimSpace(what)); w {
	case "HOSTS", "LOGS", "PRIVILEGES", "TABLES", "STATUS", "QUERY CACHE", "USER_RESOURCES", "BINARY LOGS", "ERROR LOGS", "SLOW LOGS", "GENERAL LOGS", "ENGINE LOGS", "RELAY LOGS":
		return "FLUSH " + w, nil
	}
	return "", ddlError("FLUSH inválido: %q", what)
}

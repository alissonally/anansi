package mysql

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"dbstudio/internal/database"
)

var _ database.ObjectSQL = (*Driver)(nil)

// QuoteIdent implements database.ObjectSQL.
func (d *Driver) QuoteIdent(name string) string { return QuoteIdent(name) }

// QuoteString implements database.ObjectSQL.
func (d *Driver) QuoteString(s string) string { return quoteString(s) }

var dropWord = map[database.ObjectKind]string{
	database.ObjDatabase:  "DATABASE",
	database.ObjTable:     "TABLE",
	database.ObjView:      "VIEW",
	database.ObjProcedure: "PROCEDURE",
	database.ObjFunction:  "FUNCTION",
	database.ObjTrigger:   "TRIGGER",
	database.ObjEvent:     "EVENT",
}

// qualified returns `db`.`name` after validating both parts.
func qualified(db, name string) (string, error) {
	if err := checkObjectName("database", db); err != nil {
		return "", err
	}
	if strings.TrimSpace(name) == "" || len([]rune(name)) > 64 || strings.ContainsRune(name, 0) {
		return "", ddlError("nome de objeto inválido: %q", name)
	}
	return QuoteIdent(db) + "." + QuoteIdent(name), nil
}

// DropSQL implements database.ObjectSQL.
func (d *Driver) DropSQL(kind database.ObjectKind, db, name string) (string, error) {
	word, ok := dropWord[kind]
	if !ok {
		return "", ddlError("tipo de objeto inválido: %q", kind)
	}
	if kind == database.ObjDatabase {
		if err := checkObjectName("database", name); err != nil {
			return "", err
		}
		return "DROP DATABASE " + QuoteIdent(name), nil
	}
	q, err := qualified(db, name)
	if err != nil {
		return "", err
	}
	return "DROP " + word + " " + q, nil
}

// TruncateSQL implements database.ObjectSQL.
func (d *Driver) TruncateSQL(db, table string) (string, error) {
	q, err := qualified(db, table)
	if err != nil {
		return "", err
	}
	return "TRUNCATE TABLE " + q, nil
}

// RenameSQL implements database.ObjectSQL.
func (d *Driver) RenameSQL(kind database.ObjectKind, db, name, toDB, toName string) (string, error) {
	if toDB == "" {
		toDB = db
	}
	from, err := qualified(db, name)
	if err != nil {
		return "", err
	}
	if err := checkObjectName("objeto", toName); err != nil {
		return "", err
	}
	to, err := qualified(toDB, toName)
	if err != nil {
		return "", err
	}
	switch kind {
	case database.ObjTable:
		return "RENAME TABLE " + from + " TO " + to, nil
	case database.ObjView:
		if toDB != db {
			return "", ddlError("uma view não pode ser movida para outro database")
		}
		return "RENAME TABLE " + from + " TO " + to, nil
	case database.ObjEvent:
		return "ALTER EVENT " + from + " RENAME TO " + to, nil
	}
	return "", ddlError("o servidor não permite renomear objetos do tipo %s; recrie o objeto com o novo nome", kind)
}

var (
	varNameRe  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)
	numericRe  = regexp.MustCompile(`^-?\d+(\.\d+)?([eE][-+]?\d+)?$`)
	keywordVal = map[string]bool{"ON": true, "OFF": true, "TRUE": true, "FALSE": true, "DEFAULT": true}
)

// SetVariableSQL implements database.ObjectSQL.
func (d *Driver) SetVariableSQL(global bool, name, value, valueKind string) (string, error) {
	if !varNameRe.MatchString(name) {
		return "", ddlError("nome de variável inválido: %q", name)
	}
	scope := "SESSION"
	if global {
		scope = "GLOBAL"
	}
	var v string
	switch valueKind {
	case "default":
		v = "DEFAULT"
	case "string":
		v = quoteString(value)
	case "raw":
		v = strings.TrimSpace(value)
		if v == "" || strings.ContainsAny(v, ";") {
			return "", ddlError("expressão inválida para a variável %s", name)
		}
	case "auto", "":
		t := strings.TrimSpace(value)
		switch {
		case numericRe.MatchString(t):
			v = t
		case keywordVal[strings.ToUpper(t)]:
			v = strings.ToUpper(t)
		default:
			v = quoteString(value)
		}
	default:
		return "", ddlError("tipo de valor inválido: %q", valueKind)
	}
	return fmt.Sprintf("SET %s %s = %s", scope, name, v), nil
}

// KillSQL implements database.ObjectSQL.
func (d *Driver) KillSQL(id int64, queryOnly bool) (string, error) {
	if id <= 0 {
		return "", ddlError("identificador de processo inválido")
	}
	if queryOnly {
		return "KILL QUERY " + strconv.FormatInt(id, 10), nil
	}
	return "KILL " + strconv.FormatInt(id, 10), nil
}

// AlterDatabaseSQL implements database.ObjectSQL.
func (d *Driver) AlterDatabaseSQL(db, charset, collation string) (string, error) {
	if err := checkObjectName("database", db); err != nil {
		return "", err
	}
	if err := checkOption("charset", charset); err != nil {
		return "", err
	}
	if err := checkOption("collation", collation); err != nil {
		return "", err
	}
	if charset == "" && collation == "" {
		return "", ddlError("informe o charset ou a collation")
	}
	s := "ALTER DATABASE " + QuoteIdent(db)
	if charset != "" {
		s += " CHARACTER SET " + charset
	}
	if collation != "" {
		s += " COLLATE " + collation
	}
	return s, nil
}

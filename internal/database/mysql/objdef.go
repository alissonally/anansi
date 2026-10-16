package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"anansi/internal/database"
	"anansi/internal/sqllex"
)

// definerSQL renders DEFINER=`user`@`host` from "user@host" ("" = none).
func definerSQL(def string) (string, error) {
	d := strings.TrimSpace(def)
	if d == "" {
		return "", nil
	}
	if strings.EqualFold(d, "CURRENT_USER") || strings.EqualFold(d, "CURRENT_USER()") {
		return "DEFINER=CURRENT_USER ", nil
	}
	i := strings.LastIndexByte(d, '@')
	if i <= 0 || i == len(d)-1 {
		return "", ddlError("definer inválido: use usuario@host")
	}
	unq := func(s string) string {
		s = strings.TrimSpace(s)
		if len(s) >= 2 && (s[0] == '`' && s[len(s)-1] == '`' || s[0] == '\'' && s[len(s)-1] == '\'') {
			s = s[1 : len(s)-1]
		}
		return s
	}
	return "DEFINER=" + QuoteIdent(unq(d[:i])) + "@" + QuoteIdent(unq(d[i+1:])) + " ", nil
}

// checkBody accepts statement bodies (which contain semicolons) but refuses
// unterminated strings or comments.
func checkBody(label, s string) error {
	if strings.TrimSpace(s) == "" {
		return ddlError("informe %s", label)
	}
	for _, t := range sqllex.Tokenize(s) {
		if t.Open {
			return ddlError("%s: aspas, identificador ou comentário não fechado", label)
		}
	}
	return nil
}

// ---- views --------------------------------------------------------------------

var viewHeadRe = regexp.MustCompile(`(?i)ALGORITHM=(\w+)`)

// LoadViewDef implements database.ObjectEditor.
func (d *Driver) LoadViewDef(ctx context.Context, db *sql.DB, database_, name string) (database.ViewDef, error) {
	v := database.ViewDef{Database: database_, Name: name, OrigName: name, Algorithm: "UNDEFINED", Security: "DEFINER"}
	var body, check, definer, sec sql.NullString
	err := db.QueryRowContext(ctx, `SELECT VIEW_DEFINITION, CHECK_OPTION, DEFINER, SECURITY_TYPE FROM information_schema.VIEWS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?`, database_, name).Scan(&body, &check, &definer, &sec)
	if err == sql.ErrNoRows {
		return v, ddlError("view %q não encontrada", name)
	}
	if err != nil {
		return v, err
	}
	v.Select, v.Definer, v.Security = body.String, definer.String, sec.String
	if c := strings.ToUpper(check.String); c == "CASCADED" || c == "LOCAL" {
		v.CheckOption = c
	}
	if ddl, err := newInspector(db).CreateStatement(ctx, database_, database.ObjView, name); err == nil {
		if m := viewHeadRe.FindStringSubmatch(ddl); m != nil {
			v.Algorithm = strings.ToUpper(m[1])
		}
	}
	return v, nil
}

// ViewSQL implements database.ObjectEditor: CREATE for new views, ALTER
// (plus RENAME TABLE) for existing ones.
func (d *Driver) ViewSQL(v database.ViewDef) ([]string, error) {
	if err := checkObjectName("database", v.Database); err != nil {
		return nil, err
	}
	if err := checkObjectName("view", v.Name); err != nil {
		return nil, err
	}
	sel := strings.TrimRight(strings.TrimSpace(v.Select), ";")
	if err := checkExpr("a consulta da view", sel); err != nil {
		return nil, err
	}
	def, err := definerSQL(v.Definer)
	if err != nil {
		return nil, err
	}
	var head strings.Builder
	if v.OrigName == "" {
		head.WriteString("CREATE ")
	} else {
		head.WriteString("ALTER ")
	}
	switch a := strings.ToUpper(v.Algorithm); a {
	case "", "UNDEFINED":
	case "MERGE", "TEMPTABLE":
		head.WriteString("ALGORITHM=" + a + " ")
	default:
		return nil, ddlError("algoritmo inválido: %q", v.Algorithm)
	}
	head.WriteString(def)
	switch s := strings.ToUpper(v.Security); s {
	case "":
	case "DEFINER", "INVOKER":
		head.WriteString("SQL SECURITY " + s + " ")
	default:
		return nil, ddlError("SQL SECURITY inválido: %q", v.Security)
	}
	target := v.OrigName
	if target == "" {
		target = v.Name
	}
	s := head.String() + "VIEW " + QuoteIdent(v.Database) + "." + QuoteIdent(target) + " AS\n" + sel
	switch c := strings.ToUpper(v.CheckOption); c {
	case "", "NONE":
	case "CASCADED", "LOCAL":
		s += "\nWITH " + c + " CHECK OPTION"
	default:
		return nil, ddlError("CHECK OPTION inválido: %q", v.CheckOption)
	}
	out := []string{s}
	if v.OrigName != "" && v.OrigName != v.Name {
		out = append(out, "RENAME TABLE "+QuoteIdent(v.Database)+"."+QuoteIdent(v.OrigName)+" TO "+QuoteIdent(v.Database)+"."+QuoteIdent(v.Name))
	}
	return out, nil
}

// ---- routines -------------------------------------------------------------------

var dataAccess = map[string]bool{"": true, "CONTAINS SQL": true, "NO SQL": true, "READS SQL DATA": true, "MODIFIES SQL DATA": true}

// LoadRoutineDef implements database.ObjectEditor.
func (d *Driver) LoadRoutineDef(ctx context.Context, db *sql.DB, database_, typ, name string) (database.RoutineDef, error) {
	r := database.RoutineDef{Database: database_}
	list, err := newInspector(db).RoutineDetails(ctx, database_)
	if err != nil {
		return r, err
	}
	for _, x := range list {
		if x.Name == name && strings.EqualFold(x.Type, typ) {
			return database.RoutineDef{
				Database: database_, Name: x.Name, OrigName: x.Name, Type: x.Type, OrigType: x.Type, Params: x.Params, Returns: x.Returns,
				Body: x.Body, Deterministic: x.Deterministic, DataAccess: x.DataAccess, Security: x.Security, Definer: x.Definer, Comment: x.Comment,
			}, nil
		}
	}
	return r, ddlError("rotina %q não encontrada", name)
}

// RoutineSQL implements database.ObjectEditor (always CREATE; the API
// replaces existing routines safely).
func (d *Driver) RoutineSQL(r database.RoutineDef) (string, error) {
	if err := checkObjectName("database", r.Database); err != nil {
		return "", err
	}
	if err := checkObjectName("rotina", r.Name); err != nil {
		return "", err
	}
	typ := strings.ToUpper(r.Type)
	if typ != "PROCEDURE" && typ != "FUNCTION" {
		return "", ddlError("tipo de rotina inválido: %q", r.Type)
	}
	def, err := definerSQL(r.Definer)
	if err != nil {
		return "", err
	}
	var params []string
	seen := map[string]bool{}
	for i, p := range r.Params {
		if err := checkColumnName(p.Name); err != nil {
			return "", ddlError("parâmetro %d sem nome válido", i+1)
		}
		if seen[strings.ToLower(p.Name)] {
			return "", ddlError("parâmetro duplicado: %q", p.Name)
		}
		seen[strings.ToLower(p.Name)] = true
		if err := checkExpr(fmt.Sprintf("o tipo do parâmetro %q", p.Name), p.Type); err != nil {
			return "", err
		}
		mode := strings.ToUpper(strings.TrimSpace(p.Mode))
		s := QuoteIdent(p.Name) + " " + strings.TrimSpace(p.Type)
		if typ == "PROCEDURE" {
			switch mode {
			case "", "IN":
				s = "IN " + s
			case "OUT", "INOUT":
				s = mode + " " + s
			default:
				return "", ddlError("modo inválido no parâmetro %q", p.Name)
			}
		} else if mode != "" && mode != "IN" {
			return "", ddlError("parâmetros de functions são sempre IN (%q)", p.Name)
		}
		params = append(params, s)
	}
	var b strings.Builder
	b.WriteString("CREATE " + def + typ + " " + QuoteIdent(r.Database) + "." + QuoteIdent(r.Name) + "(")
	if len(params) > 0 {
		b.WriteString("\n  " + strings.Join(params, ",\n  ") + "\n")
	}
	b.WriteString(")")
	if typ == "FUNCTION" {
		if err := checkExpr("o tipo de retorno", r.Returns); err != nil {
			return "", err
		}
		b.WriteString("\nRETURNS " + strings.TrimSpace(r.Returns))
	}
	b.WriteString("\nLANGUAGE SQL")
	if r.Deterministic {
		b.WriteString("\nDETERMINISTIC")
	} else {
		b.WriteString("\nNOT DETERMINISTIC")
	}
	da := strings.ToUpper(strings.TrimSpace(r.DataAccess))
	if !dataAccess[da] {
		return "", ddlError("acesso a dados inválido: %q", r.DataAccess)
	}
	if da != "" {
		b.WriteString("\n" + da)
	}
	switch s := strings.ToUpper(r.Security); s {
	case "":
	case "DEFINER", "INVOKER":
		b.WriteString("\nSQL SECURITY " + s)
	default:
		return "", ddlError("SQL SECURITY inválido: %q", r.Security)
	}
	if r.Comment != "" {
		b.WriteString("\nCOMMENT " + quoteString(r.Comment))
	}
	if err := checkBody("o corpo da rotina", r.Body); err != nil {
		return "", err
	}
	b.WriteString("\n" + strings.TrimSpace(r.Body))
	return b.String(), nil
}

// ---- triggers --------------------------------------------------------------------

// LoadTriggerDef implements database.ObjectEditor.
func (d *Driver) LoadTriggerDef(ctx context.Context, db *sql.DB, database_, name string) (database.TriggerDef, error) {
	t := database.TriggerDef{Database: database_}
	list, err := newInspector(db).Triggers(ctx, database_)
	if err != nil {
		return t, err
	}
	for _, x := range list {
		if x.Name == name {
			return database.TriggerDef{Database: database_, Name: x.Name, OrigName: x.Name, Table: x.Table, Timing: x.Timing, Event: x.Event, Body: x.Statement, Definer: x.Definer}, nil
		}
	}
	return t, ddlError("trigger %q não encontrado", name)
}

// TriggerSQL implements database.ObjectEditor.
func (d *Driver) TriggerSQL(t database.TriggerDef) (string, error) {
	if err := checkObjectName("database", t.Database); err != nil {
		return "", err
	}
	if err := checkObjectName("trigger", t.Name); err != nil {
		return "", err
	}
	if err := checkObjectName("tabela", t.Table); err != nil {
		return "", err
	}
	timing, event := strings.ToUpper(t.Timing), strings.ToUpper(t.Event)
	if timing != "BEFORE" && timing != "AFTER" {
		return "", ddlError("momento inválido: %q", t.Timing)
	}
	if event != "INSERT" && event != "UPDATE" && event != "DELETE" {
		return "", ddlError("evento inválido: %q", t.Event)
	}
	if err := checkBody("o corpo do trigger", t.Body); err != nil {
		return "", err
	}
	def, err := definerSQL(t.Definer)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("CREATE %sTRIGGER %s.%s %s %s ON %s.%s FOR EACH ROW\n%s", def, QuoteIdent(t.Database), QuoteIdent(t.Name), timing, event,
		QuoteIdent(t.Database), QuoteIdent(t.Table), strings.TrimSpace(t.Body)), nil
}

// ---- events ----------------------------------------------------------------------

var (
	intervalFields = map[string]bool{"YEAR": true, "QUARTER": true, "MONTH": true, "DAY": true, "HOUR": true, "MINUTE": true, "WEEK": true, "SECOND": true,
		"YEAR_MONTH": true, "DAY_HOUR": true, "DAY_MINUTE": true, "DAY_SECOND": true, "HOUR_MINUTE": true, "HOUR_SECOND": true, "MINUTE_SECOND": true}
	datetimeRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}( \d{2}:\d{2}(:\d{2}(\.\d{1,6})?)?)?$`)
)

// LoadEventDef implements database.ObjectEditor.
func (d *Driver) LoadEventDef(ctx context.Context, db *sql.DB, database_, name string) (database.EventDef, error) {
	e := database.EventDef{Database: database_}
	list, err := newInspector(db).Events(ctx, database_)
	if err != nil {
		return e, err
	}
	for _, x := range list {
		if x.Name != name {
			continue
		}
		status := "ENABLE"
		switch strings.ToUpper(x.Status) {
		case "DISABLED":
			status = "DISABLE"
		case "SLAVESIDE_DISABLED":
			status = "DISABLE ON SLAVE"
		}
		return database.EventDef{
			Database: database_, Name: x.Name, OrigName: x.Name, Definer: x.Definer, Recurring: strings.EqualFold(x.Type, "RECURRING"),
			ExecuteAt: x.ExecuteAt, IntervalValue: x.IntervalValue, IntervalField: x.IntervalField, Starts: x.Starts, Ends: x.Ends,
			Preserve: strings.EqualFold(x.OnCompletion, "PRESERVE"), Status: status, Comment: x.Comment, Body: x.Body,
		}, nil
	}
	return e, ddlError("evento %q não encontrado", name)
}

func timeExpr(label, v string) (string, error) {
	v = strings.TrimSpace(v)
	if datetimeRe.MatchString(v) {
		return quoteString(v), nil
	}
	if err := checkExpr(label, v); err != nil {
		return "", err
	}
	return v, nil
}

// EventSQL implements database.ObjectEditor.
func (d *Driver) EventSQL(e database.EventDef, cur *database.EventDef) (string, error) {
	if err := checkObjectName("database", e.Database); err != nil {
		return "", err
	}
	if err := checkObjectName("evento", e.Name); err != nil {
		return "", err
	}
	def, err := definerSQL(e.Definer)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	target := e.Name
	if cur != nil {
		target = cur.Name
		b.WriteString("ALTER " + def + "EVENT ")
	} else {
		b.WriteString("CREATE " + def + "EVENT ")
	}
	b.WriteString(QuoteIdent(e.Database) + "." + QuoteIdent(target) + "\nON SCHEDULE ")
	if e.Recurring {
		f := strings.ToUpper(strings.TrimSpace(e.IntervalField))
		if !intervalFields[f] {
			return "", ddlError("unidade de intervalo inválida: %q", e.IntervalField)
		}
		iv := strings.TrimSpace(e.IntervalValue)
		if iv == "" {
			return "", ddlError("informe o intervalo do evento")
		}
		if !plainNumber.MatchString(iv) {
			iv = quoteString(strings.Trim(iv, "'"))
		}
		b.WriteString("EVERY " + iv + " " + f)
		if strings.TrimSpace(e.Starts) != "" {
			s, err := timeExpr("o início", e.Starts)
			if err != nil {
				return "", err
			}
			b.WriteString(" STARTS " + s)
		}
		if strings.TrimSpace(e.Ends) != "" {
			s, err := timeExpr("o fim", e.Ends)
			if err != nil {
				return "", err
			}
			b.WriteString(" ENDS " + s)
		}
	} else {
		s, err := timeExpr("a data de execução", e.ExecuteAt)
		if err != nil {
			return "", err
		}
		b.WriteString("AT " + s)
	}
	if e.Preserve {
		b.WriteString("\nON COMPLETION PRESERVE")
	} else {
		b.WriteString("\nON COMPLETION NOT PRESERVE")
	}
	if cur != nil && cur.Name != e.Name {
		b.WriteString("\nRENAME TO " + QuoteIdent(e.Database) + "." + QuoteIdent(e.Name))
	}
	switch s := strings.ToUpper(strings.TrimSpace(e.Status)); s {
	case "", "ENABLE":
		b.WriteString("\nENABLE")
	case "DISABLE", "DISABLE ON SLAVE":
		b.WriteString("\n" + s)
	default:
		return "", ddlError("estado inválido: %q", e.Status)
	}
	b.WriteString("\nCOMMENT " + quoteString(e.Comment))
	if err := checkBody("o corpo do evento", e.Body); err != nil {
		return "", err
	}
	b.WriteString("\nDO " + strings.TrimSpace(e.Body))
	return b.String(), nil
}

// Definers implements database.ObjectEditor.
func (d *Driver) Definers(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, "SELECT CONCAT(User, '@', Host) FROM mysql.user WHERE User <> '' ORDER BY User, Host")
	if err != nil {
		return nil, nil // no privilege: free text only
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if rows.Scan(&s) == nil {
			out = append(out, s)
		}
	}
	return out, nil
}

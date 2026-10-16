package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ---- model -------------------------------------------------------------------------

// Account identifies a user or role.
type Account struct {
	User   string `json:"user"`
	Host   string `json:"host"`
	IsRole bool   `json:"isRole"`
	Locked bool   `json:"locked"`
	Plugin string `json:"plugin"`
	// EmptyPassword warns about accounts without password.
	EmptyPassword bool `json:"emptyPassword"`
}

// Grant is the set of privileges of an account on one object.
type Grant struct {
	// Level: global, database, table, routine.
	Level    string `json:"level"`
	Database string `json:"database"` // unescaped; may contain % and _ wildcards (database level)
	Name     string `json:"name"`     // table or routine
	// RoutineType: PROCEDURE or FUNCTION (routine level).
	RoutineType string   `json:"routineType"`
	Privileges  []string `json:"privileges"`
	// Columns holds column privileges of table grants: privilege → columns.
	Columns     map[string][]string `json:"columns"`
	GrantOption bool                `json:"grantOption"`
}

// RoleGrant is a role granted to an account.
type RoleGrant struct {
	Role        string `json:"role"` // user@host or role name
	AdminOption bool   `json:"adminOption"`
}

// UserDef is everything the user manager edits.
type UserDef struct {
	User     string `json:"user"`
	Host     string `json:"host"`
	OrigUser string `json:"origUser"`
	OrigHost string `json:"origHost"`
	IsRole   bool   `json:"isRole"`
	// Password is set only when changing it ("" keeps the current one).
	Password string `json:"password"`
	Plugin   string `json:"plugin"`
	// Require: NONE, SSL, X509 or SPECIFIED (Cipher/Issuer/Subject).
	Require            string `json:"require"`
	Cipher             string `json:"cipher"`
	Issuer             string `json:"issuer"`
	Subject            string `json:"subject"`
	MaxQueries         int    `json:"maxQueries"`
	MaxUpdates         int    `json:"maxUpdates"`
	MaxConnections     int    `json:"maxConnections"`
	MaxUserConnections int    `json:"maxUserConnections"`
	Locked             bool   `json:"locked"`
	// PasswordExpire: "" (server default), "NOW" (expired), "NEVER",
	// "INTERVAL" (with ExpireDays).
	PasswordExpire string      `json:"passwordExpire"`
	ExpireDays     int         `json:"expireDays"`
	Grants         []Grant     `json:"grants"`
	Roles          []RoleGrant `json:"roles"`
	// DefaultRoles are activated at login.
	DefaultRoles []string `json:"defaultRoles"`
}

// PrivilegeCatalog lists the privileges available at each level.
type PrivilegeCatalog struct {
	Global   []string `json:"global"`
	Database []string `json:"database"`
	Table    []string `json:"table"`
	Column   []string `json:"column"`
	Routine  []string `json:"routine"`
	Plugins  []string `json:"plugins"`
	Roles    bool     `json:"roles"`
}

// ---- loading ---------------------------------------------------------------------------

// ListAccounts lists users and roles (requires SELECT on mysql.user).
func (d *Driver) ListAccounts(ctx context.Context, db *sql.DB, mariadb bool) ([]Account, error) {
	q := "SELECT User, Host, plugin, account_locked = 'Y', authentication_string = '' FROM mysql.user ORDER BY User, Host"
	if mariadb {
		q = "SELECT User, Host, plugin, JSON_VALUE(gp.Priv, '$.account_locked') = 'true', authentication_string = '' AND Password = '', is_role = 'Y' " +
			"FROM mysql.user u LEFT JOIN mysql.global_priv gp USING (Host, User) ORDER BY User, Host"
	}
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		var a Account
		var plugin sql.NullString
		var locked, empty, role sql.NullBool
		dest := []any{&a.User, &a.Host, &plugin, &locked, &empty}
		if mariadb {
			dest = append(dest, &role)
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		a.Plugin, a.Locked, a.EmptyPassword, a.IsRole = plugin.String, locked.Bool, empty.Bool && plugin.String != "unix_socket" && plugin.String != "auth_socket", role.Bool
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !mariadb {
		// MySQL roles are locked accounts granted to someone (role_edges).
		roles := map[string]bool{}
		if r, err := db.QueryContext(ctx, "SELECT DISTINCT FROM_USER, FROM_HOST FROM mysql.role_edges"); err == nil {
			for r.Next() {
				var u, h string
				if r.Scan(&u, &h) == nil {
					roles[u+"@"+h] = true
				}
			}
			r.Close()
		}
		for i := range out {
			out[i].IsRole = roles[out[i].User+"@"+out[i].Host]
		}
	}
	return out, nil
}

var (
	grantRe   = regexp.MustCompile(`(?s)^GRANT (.+?) ON (?:(TABLE|PROCEDURE|FUNCTION) )?(\S+) TO (\S+)(.*)$`)
	roleRe    = regexp.MustCompile(`(?s)^GRANT (.+?) TO (\S+)( WITH ADMIN OPTION)?$`)
	identPart = regexp.MustCompile("^(?:`((?:[^`]|``)*)`|'((?:[^']|'')*)'|([^.`']+))")
	limitRe   = regexp.MustCompile(`(?i)\b(MAX_QUERIES_PER_HOUR|MAX_UPDATES_PER_HOUR|MAX_CONNECTIONS_PER_HOUR|MAX_USER_CONNECTIONS)\s+(\d+)`)
	pluginRe  = regexp.MustCompile("(?i)IDENTIFIED (?:WITH|VIA) '?`?([A-Za-z0-9_]+)")
	requireRe = regexp.MustCompile(`(?i)\bREQUIRE\s+(NONE|SSL|X509|(?:CIPHER|ISSUER|SUBJECT)\b.*?)(?:\s+(?:WITH|PASSWORD|ACCOUNT|$))`)
	expireRe  = regexp.MustCompile(`(?i)PASSWORD EXPIRE(?:\s+(NEVER|DEFAULT|INTERVAL\s+(\d+)\s+DAY))?`)
)

// splitPrivs splits "SELECT (`a`), UPDATE (`a`, `b`), INSERT" at top-level
// commas.
func splitPrivs(s string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
}

func parseObject(obj string) (db, name string, ok bool) {
	if obj == "*.*" {
		return "", "", true
	}
	m := identPart.FindStringSubmatch(obj)
	if m == nil {
		return "", "", false
	}
	db = m[1] + m[2] + m[3]
	db = strings.ReplaceAll(strings.ReplaceAll(db, "``", "`"), "''", "'")
	rest := strings.TrimPrefix(obj[len(m[0]):], ".")
	if rest == "*" {
		return db, "", true
	}
	n := identPart.FindStringSubmatch(rest)
	if n == nil {
		return "", "", false
	}
	name = strings.ReplaceAll(n[1]+n[2]+n[3], "``", "`")
	return db, name, true
}

// ParseGrants turns SHOW GRANTS lines into grants and role grants.
func ParseGrants(lines []string) ([]Grant, []RoleGrant) {
	var grants []Grant
	var roles []RoleGrant
	byKey := map[string]int{}
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if m := grantRe.FindStringSubmatch(l); m != nil {
			db, name, ok := parseObject(m[3])
			if !ok || strings.EqualFold(strings.TrimSpace(m[1]), "PROXY") {
				continue
			}
			// The database is kept exactly as shown (with \_ and \% escapes)
			// so REVOKE/GRANT address the same pattern.
			g := Grant{Database: db, Name: name, RoutineType: strings.ToUpper(m[2]), Columns: map[string][]string{}}
			switch {
			case m[3] == "*.*":
				g.Level = "global"
			case g.RoutineType == "PROCEDURE" || g.RoutineType == "FUNCTION":
				g.Level = "routine"
				g.Database = db
			case name == "":
				g.Level = "database"
			default:
				g.Level = "table"
				g.Database = db
			}
			g.GrantOption = strings.Contains(strings.ToUpper(m[5]), "WITH GRANT OPTION")
			key := g.Level + "\x00" + g.Database + "\x00" + g.Name + "\x00" + g.RoutineType
			if i, ok := byKey[key]; ok {
				// MySQL lists dynamic privileges on a separate line.
				g = grants[i]
				if strings.Contains(strings.ToUpper(m[5]), "WITH GRANT OPTION") {
					g.GrantOption = true
				}
			}
			for _, p := range splitPrivs(m[1]) {
				up := strings.ToUpper(strings.TrimSpace(p))
				if up == "USAGE" || up == "" {
					continue
				}
				if i := strings.IndexByte(p, '('); i > 0 {
					priv := strings.ToUpper(strings.TrimSpace(p[:i]))
					for _, c := range strings.Split(strings.Trim(p[i:], "() "), ",") {
						c = strings.Trim(strings.TrimSpace(c), "`")
						if c != "" {
							g.Columns[priv] = append(g.Columns[priv], c)
						}
					}
					continue
				}
				if up == "ALL PRIVILEGES" || up == "ALL" {
					up = "ALL PRIVILEGES"
				}
				g.Privileges = append(g.Privileges, up)
			}
			if i, ok := byKey[key]; ok {
				grants[i] = g
			} else {
				byKey[key] = len(grants)
				grants = append(grants, g)
			}
			continue
		}
		if m := roleRe.FindStringSubmatch(l); m != nil {
			for _, r := range splitPrivs(m[1]) {
				roles = append(roles, RoleGrant{Role: unquoteAccount(r), AdminOption: m[3] != ""})
			}
		}
	}
	// "GRANT USAGE ON *.*" (no privileges) is not a grant.
	kept := grants[:0]
	for _, g := range grants {
		if len(g.Privileges) == 0 && len(g.Columns) == 0 && !g.GrantOption {
			continue
		}
		sort.Strings(g.Privileges)
		kept = append(kept, g)
	}
	return kept, roles
}

// unquoteAccount turns `u`@`h` or 'r' into u@h / r.
func unquoteAccount(s string) string {
	s = strings.TrimSpace(s)
	clean := func(p string) string {
		p = strings.TrimSpace(p)
		if len(p) >= 2 && (p[0] == '`' || p[0] == '\'') {
			p = p[1 : len(p)-1]
		}
		return p
	}
	if i := strings.Index(s, "@"); i > 0 {
		return clean(s[:i]) + "@" + clean(s[i+1:])
	}
	return clean(s)
}

// accountSQL quotes an account; MariaDB roles have no host part.
func accountSQL(user, host string, mariadbRole bool) string {
	if mariadbRole {
		return quoteString(user)
	}
	return quoteString(user) + "@" + quoteString(host)
}

// LoadUser reads an account definition (SHOW CREATE USER + SHOW GRANTS).
func (d *Driver) LoadUser(ctx context.Context, db *sql.DB, user, host string, isRole, mariadb bool) (UserDef, error) {
	u := UserDef{User: user, Host: host, OrigUser: user, OrigHost: host, Require: "NONE", IsRole: isRole}
	acct := accountSQL(user, host, isRole && mariadb)
	var create string
	if !(isRole && mariadb) {
		// SHOW CREATE USER returns a single column.
		if err := db.QueryRowContext(ctx, "SHOW CREATE USER "+acct).Scan(&create); err != nil {
			return u, err
		}
	}
	if m := pluginRe.FindStringSubmatch(create); m != nil {
		u.Plugin = m[1]
	}
	if m := requireRe.FindStringSubmatch(create + " $"); m != nil {
		r := strings.TrimSpace(m[1])
		switch up := strings.ToUpper(r); {
		case up == "NONE", up == "SSL", up == "X509":
			u.Require = up
		default:
			u.Require = "SPECIFIED"
			for _, kv := range []struct {
				k string
				v *string
			}{{"CIPHER", &u.Cipher}, {"ISSUER", &u.Issuer}, {"SUBJECT", &u.Subject}} {
				if mm := regexp.MustCompile(`(?i)` + kv.k + `\s+'((?:[^']|'')*)'`).FindStringSubmatch(r); mm != nil {
					*kv.v = strings.ReplaceAll(mm[1], "''", "'")
				}
			}
		}
	}
	for _, m := range limitRe.FindAllStringSubmatch(create, -1) {
		n, _ := strconv.Atoi(m[2])
		switch strings.ToUpper(m[1]) {
		case "MAX_QUERIES_PER_HOUR":
			u.MaxQueries = n
		case "MAX_UPDATES_PER_HOUR":
			u.MaxUpdates = n
		case "MAX_CONNECTIONS_PER_HOUR":
			u.MaxConnections = n
		case "MAX_USER_CONNECTIONS":
			u.MaxUserConnections = n
		}
	}
	u.Locked = strings.Contains(strings.ToUpper(create), "ACCOUNT LOCK")
	if m := expireRe.FindStringSubmatch(create); m != nil {
		switch {
		case m[2] != "":
			u.PasswordExpire = "INTERVAL"
			u.ExpireDays, _ = strconv.Atoi(m[2])
		case strings.EqualFold(m[1], "NEVER"):
			u.PasswordExpire = "NEVER"
		case strings.EqualFold(m[1], "DEFAULT"):
			u.PasswordExpire = ""
		default:
			u.PasswordExpire = "NOW"
		}
	}
	rows, err := db.QueryContext(ctx, "SHOW GRANTS FOR "+acct)
	if err != nil {
		return u, err
	}
	var lines []string
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			rows.Close()
			return u, err
		}
		lines = append(lines, l)
	}
	rows.Close()
	u.Grants, u.Roles = ParseGrants(lines)
	// Default roles: MySQL keeps them in mysql.default_roles, MariaDB in
	// mysql.user.default_role (one role). Without privilege they stay empty.
	if !mariadb {
		if r, err := db.QueryContext(ctx, "SELECT CONCAT(DEFAULT_ROLE_USER, '@', DEFAULT_ROLE_HOST) FROM mysql.default_roles WHERE USER = ? AND HOST = ? ORDER BY 1", user, host); err == nil {
			for r.Next() {
				var s string
				if r.Scan(&s) == nil {
					u.DefaultRoles = append(u.DefaultRoles, s)
				}
			}
			r.Close()
		}
	} else {
		var role sql.NullString
		if db.QueryRowContext(ctx, "SELECT default_role FROM mysql.user WHERE User = ? AND Host = ?", user, host).Scan(&role) == nil && role.String != "" {
			u.DefaultRoles = []string{role.String}
		}
	}
	return u, nil
}

// PrivilegeCatalogFor lists privileges from SHOW PRIVILEGES.
func (d *Driver) PrivilegeCatalogFor(ctx context.Context, db *sql.DB, mariadb bool) (PrivilegeCatalog, error) {
	cat := PrivilegeCatalog{
		Database: []string{"ALTER", "ALTER ROUTINE", "CREATE", "CREATE ROUTINE", "CREATE TEMPORARY TABLES", "CREATE VIEW", "DELETE", "DROP", "EVENT", "EXECUTE", "INDEX", "INSERT", "LOCK TABLES", "REFERENCES", "SELECT", "SHOW VIEW", "TRIGGER", "UPDATE"},
		Table:    []string{"ALTER", "CREATE", "CREATE VIEW", "DELETE", "DROP", "INDEX", "INSERT", "REFERENCES", "SELECT", "SHOW VIEW", "TRIGGER", "UPDATE"},
		Column:   []string{"INSERT", "REFERENCES", "SELECT", "UPDATE"},
		Routine:  []string{"ALTER ROUTINE", "EXECUTE"},
		Roles:    true,
	}
	if mariadb {
		cat.Database = append(cat.Database, "DELETE HISTORY")
		cat.Table = append(cat.Table, "DELETE HISTORY")
	}
	rows, err := db.QueryContext(ctx, "SHOW PRIVILEGES")
	if err != nil {
		return cat, err
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	for rows.Next() {
		vals := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return cat, err
		}
		p := strings.ToUpper(vals[0].String)
		switch p {
		case "USAGE", "GRANT OPTION", "PROXY":
			continue
		}
		cat.Global = append(cat.Global, p)
	}
	sort.Strings(cat.Global)
	if r, err := db.QueryContext(ctx, "SELECT PLUGIN_NAME FROM information_schema.PLUGINS WHERE PLUGIN_TYPE = 'AUTHENTICATION' AND PLUGIN_STATUS = 'ACTIVE' ORDER BY PLUGIN_NAME"); err == nil {
		for r.Next() {
			var s string
			if r.Scan(&s) == nil {
				cat.Plugins = append(cat.Plugins, s)
			}
		}
		r.Close()
	}
	return cat, rows.Err()
}

// ---- saving ----------------------------------------------------------------------------

var privNameRe = regexp.MustCompile(`^[A-Z_][A-Z_ ]{1,60}$`)

func grantTarget(g Grant) (string, error) {
	switch g.Level {
	case "global":
		return "*.*", nil
	case "database":
		if strings.TrimSpace(g.Database) == "" {
			return "", ddlError("informe o database do privilégio")
		}
		// Underscores and percent signs act as wildcards at this level.
		return QuoteIdent(g.Database) + ".*", nil
	case "table":
		q, err := qualified(g.Database, g.Name)
		if err != nil {
			return "", err
		}
		return "TABLE " + q, nil
	case "routine":
		q, err := qualified(g.Database, g.Name)
		if err != nil {
			return "", err
		}
		t := strings.ToUpper(g.RoutineType)
		if t != "PROCEDURE" && t != "FUNCTION" {
			return "", ddlError("tipo de rotina inválido no privilégio")
		}
		return t + " " + q, nil
	}
	return "", ddlError("nível de privilégio inválido: %q", g.Level)
}

func grantKey(g Grant) string {
	return g.Level + "\x00" + strings.ToLower(g.Database) + "\x00" + strings.ToLower(g.Name) + "\x00" + strings.ToUpper(g.RoutineType)
}

func privList(g Grant) ([]string, error) {
	var out []string
	for _, p := range g.Privileges {
		p = strings.ToUpper(strings.TrimSpace(p))
		if !privNameRe.MatchString(p) {
			return nil, ddlError("privilégio inválido: %q", p)
		}
		out = append(out, p)
	}
	keys := make([]string, 0, len(g.Columns))
	for k := range g.Columns {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		cols := g.Columns[k]
		if len(cols) == 0 {
			continue
		}
		p := strings.ToUpper(k)
		if !privNameRe.MatchString(p) {
			return nil, ddlError("privilégio inválido: %q", k)
		}
		q := make([]string, len(cols))
		for i, c := range cols {
			q[i] = QuoteIdent(c)
		}
		out = append(out, p+" ("+strings.Join(q, ", ")+")")
	}
	return out, nil
}

func diffList(a, b []string) []string {
	in := map[string]bool{}
	for _, x := range b {
		in[x] = true
	}
	var out []string
	for _, x := range a {
		if !in[x] {
			out = append(out, x)
		}
	}
	return out
}

func identifiedSQL(u UserDef, mariadb bool) string {
	pw := quoteString(u.Password)
	plugin := strings.TrimSpace(u.Plugin)
	if plugin != "" && !optionNameRe.MatchString(plugin) {
		plugin = ""
	}
	switch {
	case plugin == "":
		return " IDENTIFIED BY " + pw
	case mariadb && (plugin == "unix_socket" || plugin == "gssapi"):
		return " IDENTIFIED VIA " + plugin
	case mariadb:
		return " IDENTIFIED VIA " + plugin + " USING PASSWORD(" + pw + ")"
	case plugin == "auth_socket":
		return " IDENTIFIED WITH auth_socket"
	default:
		return " IDENTIFIED WITH " + plugin + " BY " + pw
	}
}

func optionsSQLUser(u UserDef) (require, with, lock, expire string) {
	switch strings.ToUpper(u.Require) {
	case "", "NONE":
		require = " REQUIRE NONE"
	case "SSL", "X509":
		require = " REQUIRE " + strings.ToUpper(u.Require)
	default:
		var parts []string
		if u.Cipher != "" {
			parts = append(parts, "CIPHER "+quoteString(u.Cipher))
		}
		if u.Issuer != "" {
			parts = append(parts, "ISSUER "+quoteString(u.Issuer))
		}
		if u.Subject != "" {
			parts = append(parts, "SUBJECT "+quoteString(u.Subject))
		}
		if len(parts) == 0 {
			require = " REQUIRE SSL"
		} else {
			require = " REQUIRE " + strings.Join(parts, " AND ")
		}
	}
	with = fmt.Sprintf(" WITH MAX_QUERIES_PER_HOUR %d MAX_UPDATES_PER_HOUR %d MAX_CONNECTIONS_PER_HOUR %d MAX_USER_CONNECTIONS %d",
		max(u.MaxQueries, 0), max(u.MaxUpdates, 0), max(u.MaxConnections, 0), max(u.MaxUserConnections, 0))
	if u.Locked {
		lock = " ACCOUNT LOCK"
	} else {
		lock = " ACCOUNT UNLOCK"
	}
	switch strings.ToUpper(u.PasswordExpire) {
	case "NOW":
		expire = " PASSWORD EXPIRE"
	case "NEVER":
		expire = " PASSWORD EXPIRE NEVER"
	case "INTERVAL":
		expire = fmt.Sprintf(" PASSWORD EXPIRE INTERVAL %d DAY", max(u.ExpireDays, 1))
	default:
		expire = " PASSWORD EXPIRE DEFAULT"
	}
	return
}

// UserSQL returns the statements turning cur (nil for a new account) into u.
func (d *Driver) UserSQL(cur *UserDef, u UserDef, mariadb bool) ([]string, error) {
	if strings.TrimSpace(u.User) == "" && !u.IsRole {
		return nil, ddlError("informe o nome do usuário")
	}
	if len([]rune(u.User)) > 80 || len([]rune(u.Host)) > 255 || strings.ContainsRune(u.User+u.Host, 0) {
		return nil, ddlError("nome de usuário ou host longo demais")
	}
	if u.IsRole {
		u.Host = map[bool]string{true: "", false: "%"}[mariadb]
	} else if strings.TrimSpace(u.Host) == "" {
		u.Host = "%"
	}
	acct := accountSQL(u.User, u.Host, u.IsRole && mariadb)
	var out []string
	require, with, lock, expire := optionsSQLUser(u)
	if cur == nil {
		if u.IsRole {
			out = append(out, "CREATE ROLE "+acct)
		} else {
			s := "CREATE USER " + acct
			if u.Password != "" || (u.Plugin != "" && u.Plugin != "mysql_native_password" && u.Plugin != "caching_sha2_password") {
				s += identifiedSQL(u, mariadb)
			}
			out = append(out, s+require+with+lock+expire)
		}
	} else {
		if cur.User != u.User || cur.Host != u.Host {
			out = append(out, "RENAME USER "+accountSQL(cur.User, cur.Host, cur.IsRole && mariadb)+" TO "+acct)
		}
		if !u.IsRole {
			if u.Password != "" || (u.Plugin != "" && u.Plugin != cur.Plugin) {
				out = append(out, "ALTER USER "+acct+identifiedSQL(u, mariadb))
			}
			cr, cw, cl, ce := optionsSQLUser(*cur)
			if cr != require || cw != with || cl != lock || ce != expire {
				out = append(out, "ALTER USER "+acct+require+with+lock+expire)
			}
		}
	}

	// Privileges: revoke what was removed, grant what was added, per object.
	curGrants := map[string]Grant{}
	if cur != nil {
		for _, g := range cur.Grants {
			curGrants[grantKey(g)] = g
		}
	}
	seen := map[string]bool{}
	for _, g := range u.Grants {
		k := grantKey(g)
		seen[k] = true
		target, err := grantTarget(g)
		if err != nil {
			return nil, err
		}
		want, err := privList(g)
		if err != nil {
			return nil, err
		}
		var have []string
		haveOpt := false
		if c, ok := curGrants[k]; ok {
			if have, err = privList(c); err != nil {
				return nil, err
			}
			haveOpt = c.GrantOption
		}
		if rv := diffList(have, want); len(rv) > 0 {
			out = append(out, "REVOKE "+strings.Join(rv, ", ")+" ON "+target+" FROM "+acct)
		}
		if haveOpt && !g.GrantOption {
			out = append(out, "REVOKE GRANT OPTION ON "+target+" FROM "+acct)
		}
		add := diffList(want, have)
		if len(add) == 0 && g.GrantOption && !haveOpt {
			add = []string{"USAGE"}
		}
		if len(add) > 0 {
			s := "GRANT " + strings.Join(add, ", ") + " ON " + target + " TO " + acct
			if g.GrantOption && !haveOpt {
				s += " WITH GRANT OPTION"
			}
			out = append(out, s)
		}
	}
	if cur != nil {
		for k, c := range curGrants {
			if seen[k] {
				continue
			}
			target, err := grantTarget(c)
			if err != nil {
				return nil, err
			}
			have, err := privList(c)
			if err != nil {
				return nil, err
			}
			if c.GrantOption {
				have = append(have, "GRANT OPTION")
			}
			if len(have) > 0 {
				out = append(out, "REVOKE "+strings.Join(have, ", ")+" ON "+target+" FROM "+acct)
			}
		}
	}

	// Roles.
	curRoles := map[string]RoleGrant{}
	if cur != nil {
		for _, r := range cur.Roles {
			curRoles[r.Role] = r
		}
	}
	roleSQL := func(r string) string {
		if i := strings.LastIndexByte(r, '@'); i > 0 {
			return quoteString(r[:i]) + "@" + quoteString(r[i+1:])
		}
		return quoteString(r)
	}
	newRoles := map[string]bool{}
	for _, r := range u.Roles {
		newRoles[r.Role] = true
		c, had := curRoles[r.Role]
		if !had || c.AdminOption != r.AdminOption {
			if had {
				out = append(out, "REVOKE "+roleSQL(r.Role)+" FROM "+acct)
			}
			s := "GRANT " + roleSQL(r.Role) + " TO " + acct
			if r.AdminOption {
				s += " WITH ADMIN OPTION"
			}
			out = append(out, s)
		}
	}
	for name := range curRoles {
		if !newRoles[name] {
			out = append(out, "REVOKE "+roleSQL(name)+" FROM "+acct)
		}
	}
	if !u.IsRole && cur != nil && strings.Join(cur.DefaultRoles, ",") != strings.Join(u.DefaultRoles, ",") || cur == nil && len(u.DefaultRoles) > 0 {
		switch {
		case len(u.DefaultRoles) == 0:
			out = append(out, "SET DEFAULT ROLE NONE "+map[bool]string{true: "FOR ", false: "TO "}[mariadb]+acct)
		case mariadb:
			out = append(out, "SET DEFAULT ROLE "+roleSQL(u.DefaultRoles[0])+" FOR "+acct)
		default:
			var rs []string
			for _, r := range u.DefaultRoles {
				rs = append(rs, roleSQL(r))
			}
			out = append(out, "SET DEFAULT ROLE "+strings.Join(rs, ", ")+" TO "+acct)
		}
	}
	return out, nil
}

// DropUserSQL drops an account or role.
func (d *Driver) DropUserSQL(user, host string, isRole, mariadb bool) string {
	if isRole {
		return "DROP ROLE " + accountSQL(user, host, mariadb)
	}
	return "DROP USER " + accountSQL(user, host, false)
}

// MaskPasswords hides password literals of user statements for display.
func MaskPasswords(stmt string) string {
	return regexp.MustCompile(`(?i)((?:BY|PASSWORD\()\s*)'(?:[^']|'')*'`).ReplaceAllString(stmt, "$1'***'")
}

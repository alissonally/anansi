package mysql

import (
	"strings"
	"testing"
)

func TestParseGrants(t *testing.T) {
	g, r := ParseGrants([]string{
		"GRANT PROCESS ON *.* TO `x`@`%`",
		"GRANT BACKUP_ADMIN ON *.* TO `x`@`%`",
		"GRANT SELECT, INSERT ON `studio\\_it`.* TO `x`@`%`",
		"GRANT SELECT (`a`), UPDATE (`a`, `b`) ON `studio_it`.`tt` TO `x`@`%` WITH GRANT OPTION",
		"GRANT EXECUTE ON PROCEDURE `db`.`p` TO `x`@`%`",
		"GRANT USAGE ON *.* TO `y`@`%` IDENTIFIED BY PASSWORD '*667F' WITH MAX_QUERIES_PER_HOUR 5",
		"GRANT `leitor`@`%` TO `x`@`%`",
		"GRANT `papel` TO `x`@`%` WITH ADMIN OPTION",
		"GRANT PROXY ON ''@'' TO 'root'@'localhost' WITH GRANT OPTION",
	})
	if len(g) != 4 {
		t.Fatalf("%+v", g)
	}
	if g[0].Level != "global" || strings.Join(g[0].Privileges, ",") != "BACKUP_ADMIN,PROCESS" {
		t.Fatalf("global merged: %+v", g[0])
	}
	if g[1].Level != "database" || g[1].Database != `studio\_it` {
		t.Fatalf("db %+v", g[1])
	}
	if g[2].Level != "table" || !g[2].GrantOption || strings.Join(g[2].Columns["UPDATE"], ",") != "a,b" || len(g[2].Privileges) != 0 {
		t.Fatalf("table %+v", g[2])
	}
	if g[3].Level != "routine" || g[3].RoutineType != "PROCEDURE" || g[3].Name != "p" {
		t.Fatalf("routine %+v", g[3])
	}
	if len(r) != 2 || r[0].Role != "leitor@%" || r[1].Role != "papel" || !r[1].AdminOption {
		t.Fatalf("roles %+v", r)
	}
}

func TestUserSQLDiff(t *testing.T) {
	d := New()
	u := UserDef{User: "ana", Host: "%", Password: "s3gr'edo", Require: "SSL", MaxQueries: 10, Locked: true, PasswordExpire: "INTERVAL", ExpireDays: 90,
		Grants: []Grant{{Level: "database", Database: "loja", Privileges: []string{"SELECT", "INSERT"}}, {Level: "table", Database: "loja", Name: "t", Columns: map[string][]string{"UPDATE": {"a"}}, GrantOption: true}},
		Roles:  []RoleGrant{{Role: "leitor@%"}}, DefaultRoles: []string{"leitor@%"}}
	stmts, err := d.UserSQL(nil, u, false)
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(stmts, "\n")
	for _, want := range []string{
		"CREATE USER 'ana'@'%' IDENTIFIED BY 's3gr''edo' REQUIRE SSL WITH MAX_QUERIES_PER_HOUR 10 MAX_UPDATES_PER_HOUR 0 MAX_CONNECTIONS_PER_HOUR 0 MAX_USER_CONNECTIONS 0 ACCOUNT LOCK PASSWORD EXPIRE INTERVAL 90 DAY",
		"GRANT SELECT, INSERT ON `loja`.* TO 'ana'@'%'",
		"GRANT UPDATE (`a`) ON TABLE `loja`.`t` TO 'ana'@'%' WITH GRANT OPTION",
		"GRANT 'leitor'@'%' TO 'ana'@'%'",
		"SET DEFAULT ROLE 'leitor'@'%' TO 'ana'@'%'",
	} {
		if !strings.Contains(all, want) {
			t.Fatalf("missing %q in\n%s", want, all)
		}
	}
	if !strings.Contains(MaskPasswords(stmts[0]), "BY '***'") || strings.Contains(MaskPasswords(stmts[0]), "s3gr") {
		t.Fatal(MaskPasswords(stmts[0]))
	}
	// Same definition → nothing to do.
	cur := u
	cur.Password = ""
	same, err := d.UserSQL(&cur, cur, false)
	if err != nil || len(same) != 0 {
		t.Fatalf("%v %q", err, same)
	}
	// Changes: rename, drop a privilege, revoke grant option, remove role.
	next := cur
	next.User = "ana2"
	next.Grants = []Grant{{Level: "database", Database: "loja", Privileges: []string{"SELECT"}}, {Level: "table", Database: "loja", Name: "t", Columns: map[string][]string{"UPDATE": {"a"}}}}
	next.Roles, next.DefaultRoles = nil, nil
	stmts, _ = d.UserSQL(&cur, next, false)
	all = strings.Join(stmts, "\n")
	for _, want := range []string{"RENAME USER 'ana'@'%' TO 'ana2'@'%'", "REVOKE INSERT ON `loja`.* FROM 'ana2'@'%'", "REVOKE GRANT OPTION ON TABLE `loja`.`t` FROM 'ana2'@'%'", "REVOKE 'leitor'@'%' FROM 'ana2'@'%'", "SET DEFAULT ROLE NONE TO 'ana2'@'%'"} {
		if !strings.Contains(all, want) {
			t.Fatalf("missing %q in\n%s", want, all)
		}
	}
	if _, err := d.UserSQL(nil, UserDef{User: "x", Grants: []Grant{{Level: "global", Privileges: []string{"SELECT; DROP"}}}}, false); err == nil {
		t.Fatal("bad privilege accepted")
	}
	// MariaDB roles have no host.
	stmts, _ = d.UserSQL(nil, UserDef{User: "leitor", IsRole: true}, true)
	if stmts[0] != "CREATE ROLE 'leitor'" {
		t.Fatal(stmts)
	}
}

func TestParseGrantsUsageOnly(t *testing.T) {
	g, _ := ParseGrants([]string{"GRANT USAGE ON *.* TO `u`@`localhost`"})
	if len(g) != 0 {
		t.Fatalf("USAGE is not a grant: %+v", g)
	}
	if s := accountSQL("r", "%", true); s != "'r'" {
		t.Fatalf("MariaDB role has no host: %s", s)
	}
}

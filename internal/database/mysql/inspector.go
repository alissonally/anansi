package mysql

import (
	"context"
	"database/sql"
	"sort"
	"strings"

	"dbstudio/internal/database"
)

type inspector struct{ db *sql.DB }

var systemSchemas = map[string]bool{
	"information_schema": true,
	"performance_schema": true,
	"mysql":              true,
	"sys":                true,
}

func (i *inspector) Databases(ctx context.Context) ([]database.DatabaseInfo, error) {
	rows, err := i.db.QueryContext(ctx, "SELECT SCHEMA_NAME FROM information_schema.SCHEMATA ORDER BY SCHEMA_NAME")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []database.DatabaseInfo
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, database.DatabaseInfo{Name: name, System: systemSchemas[strings.ToLower(name)]})
	}
	return out, rows.Err()
}

func (i *inspector) Tables(ctx context.Context, db string) ([]database.TableInfo, error) {
	rows, err := i.db.QueryContext(ctx, `
SELECT TABLE_NAME, TABLE_TYPE, COALESCE(ENGINE, ''), TABLE_ROWS, COALESCE(TABLE_COMMENT, '')
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = ?
ORDER BY TABLE_NAME`, db)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []database.TableInfo
	for rows.Next() {
		var t database.TableInfo
		var typ string
		var n sql.NullInt64
		if err := rows.Scan(&t.Name, &typ, &t.Engine, &n, &t.Comment); err != nil {
			return nil, err
		}
		switch strings.ToUpper(typ) {
		case "VIEW":
			t.Type = "view"
		case "SYSTEM VIEW":
			t.Type = "system view"
		default:
			t.Type = "table"
		}
		if n.Valid {
			v := n.Int64
			t.Rows = &v
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

const columnsSQL = `
SELECT TABLE_NAME, COLUMN_NAME, ORDINAL_POSITION, DATA_TYPE, COLUMN_TYPE, IS_NULLABLE,
       COLUMN_DEFAULT, COLUMN_KEY, EXTRA, COALESCE(COLUMN_COMMENT, '')
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = ?`

func scanColumns(rows *sql.Rows) (map[string][]database.ColumnInfo, error) {
	defer rows.Close()
	out := map[string][]database.ColumnInfo{}
	for rows.Next() {
		var table, nullable string
		var c database.ColumnInfo
		var def sql.NullString
		if err := rows.Scan(&table, &c.Name, &c.Position, &c.DataType, &c.ColumnType, &nullable, &def, &c.Key, &c.Extra, &c.Comment); err != nil {
			return nil, err
		}
		c.Nullable = strings.EqualFold(nullable, "YES")
		if def.Valid {
			v := def.String
			c.Default = &v
		}
		out[table] = append(out[table], c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, cols := range out {
		sort.Slice(cols, func(a, b int) bool { return cols[a].Position < cols[b].Position })
	}
	return out, nil
}

func (i *inspector) Columns(ctx context.Context, db, table string) ([]database.ColumnInfo, error) {
	rows, err := i.db.QueryContext(ctx, columnsSQL+" AND TABLE_NAME = ? ORDER BY ORDINAL_POSITION", db, table)
	if err != nil {
		return nil, err
	}
	m, err := scanColumns(rows)
	if err != nil {
		return nil, err
	}
	return m[table], nil
}

func (i *inspector) AllColumns(ctx context.Context, db string) (map[string][]database.ColumnInfo, error) {
	rows, err := i.db.QueryContext(ctx, columnsSQL+" ORDER BY TABLE_NAME, ORDINAL_POSITION", db)
	if err != nil {
		return nil, err
	}
	return scanColumns(rows)
}

func (i *inspector) Indexes(ctx context.Context, db, table string) ([]database.IndexInfo, error) {
	rows, err := i.db.QueryContext(ctx, `
SELECT INDEX_NAME, COLUMN_NAME, NON_UNIQUE, INDEX_TYPE
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
ORDER BY INDEX_NAME = 'PRIMARY' DESC, INDEX_NAME, SEQ_IN_INDEX`, db, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []database.IndexInfo
	idx := map[string]int{}
	for rows.Next() {
		var name, typ string
		var col sql.NullString // NULL for functional indexes
		var nonUnique int
		if err := rows.Scan(&name, &col, &nonUnique, &typ); err != nil {
			return nil, err
		}
		pos, ok := idx[name]
		if !ok {
			out = append(out, database.IndexInfo{Name: name, Unique: nonUnique == 0, Primary: name == "PRIMARY", Type: typ})
			pos = len(out) - 1
			idx[name] = pos
		}
		c := col.String
		if !col.Valid {
			c = "(expressão)"
		}
		out[pos].Columns = append(out[pos].Columns, c)
	}
	return out, rows.Err()
}

func (i *inspector) Routines(ctx context.Context, db string) ([]database.RoutineInfo, error) {
	rows, err := i.db.QueryContext(ctx, `
SELECT ROUTINE_NAME, ROUTINE_TYPE FROM information_schema.ROUTINES
WHERE ROUTINE_SCHEMA = ? ORDER BY ROUTINE_NAME`, db)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []database.RoutineInfo
	for rows.Next() {
		var r database.RoutineInfo
		if err := rows.Scan(&r.Name, &r.Type); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

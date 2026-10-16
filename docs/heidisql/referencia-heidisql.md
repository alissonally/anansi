# HeidiSQL feature inventory for a MySQL/MariaDB re-implementation

All paths are under `/home/alisson/dev/HeidiSQL/source/`. Line numbers come from the current tree. Several `.pas` files (notably `dbconnection.pas` and `apphelpers.pas`) are Windows-1252 encoded, so use `grep -a` on them.

Abbreviations: **PK** = primary key, **IS** = INFORMATION_SCHEMA, `Q(x)` = identifier quoting (backticks on MySQL), `E(x)` = string escaping plus single quotes.

---

## 0. Core architecture you need to copy (SQL provider, version gates, data types)

### Query templates
- **Query IDs** are defined at `dbstructures.pas:40-55`.
- **Generic defaults** are at `dbstructures.pas:209-221`:
  - `qEmptyTable='DELETE FROM %s'`
  - `qForeignKeyEventAction='RESTRICT,CASCADE,SET NULL,NO ACTION'`
  - `qGetRowCountExact='SELECT COUNT(*) FROM :QuotedDbAndTableName'`
  - `qAutoInc='AUTO_INCREMENT'`
  - `qOrderAsc/Desc`
- **MySQL provider** is at `dbstructures.mysql.pas:3247-3447`. `IsMariaDB` means version >= 100000. The templates are:

| ID | SQL |
|---|---|
| qDatabaseDrop | `DROP DATABASE %s` |
| qEmptyTable | `TRUNCATE %s` |
| qRenameTable / qRenameView | `RENAME TABLE %s TO %s` |
| qCurrentUserHost | `SELECT CURRENT_USER()` |
| qLikeCompare | `%s LIKE %s` |
| qAddColumn / qChangeColumn | `ADD COLUMN %s` / `CHANGE COLUMN %s %s` |
| qGlobalStatus | `SHOW /*!50002 GLOBAL */ STATUS` (ProxySQL: `SELECT * FROM stats_mysql_global`) |
| qCommandsCounters | `SHOW /*!50002 GLOBAL */ STATUS LIKE 'Com\_%'` |
| qSessionVariables / qGlobalVariables | `SHOW VARIABLES` / `SHOW GLOBAL VARIABLES` (>=4.0) |
| qUSEQuery | `USE %s` |
| qKillQuery | `KILL QUERY %d` (>=5.0, else `KILL %d`; RDS: `CALL mysql.rds_kill_query(%d)`) |
| qKillProcess | `KILL %d` (RDS: `CALL mysql.rds_kill(%d)`) |
| qFuncLeft / qFuncNow / qFuncLastAutoIncNumber | `LEFT(%s, %d)` / `NOW()` / `LAST_INSERT_ID()` |
| qLockedTables | `SHOW OPEN TABLES FROM %s WHERE in_use!=0` (>=5.1.24) |
| qDisableForeignKeyChecks | `SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0` |
| qEnableForeignKeyChecks | `SET FOREIGN_KEY_CHECKS=IFNULL(@OLD_FOREIGN_KEY_CHECKS, 1)` |
| qForeignKeyDrop | `DROP FOREIGN KEY %s` |
| qGetCollations | `SHOW COLLATION` |
| qGetCollationsExtended (MariaDB >=10.10.1) | `SELECT FULL_COLLATION_NAME AS Collation, CHARACTER_SET_NAME AS Charset, ID AS Id, IS_DEFAULT AS Default, 0 AS Sortlen FROM INFORMATION_SCHEMA.COLLATION_CHARACTER_SET_APPLICABILITY ORDER BY Collation` |
| qGetCharsets | `SHOW CHARSET` |
| qGetRowCountApprox | `SHOW TABLE STATUS FROM :QuotedDatabase LIKE :EscapedName` (reads column index 4 = Rows) |
| qGetReverseForeignKeys | `SELECT DISTINCT k.TABLE_SCHEMA, k.TABLE_NAME FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE k WHERE REFERENCED_TABLE_SCHEMA=:EscapedDatabase AND REFERENCED_TABLE_NAME=:EscapedName` |
| qExplain | `EXPLAIN %s` (MySQL 8.4+: `EXPLAIN FORMAT=TRADITIONAL %s`) |
| qSetTimezone | `SET time_zone=%s` |
| qShowFunctionStatus / qShowProcedureStatus | `SHOW FUNCTION STATUS WHERE LOWER(Db) = LOWER(%s)` / same with `PROCEDURE` |
| qShowTriggers | `SHOW TRIGGERS FROM %s` |
| qShowEvents | `SELECT *, EVENT_SCHEMA AS Db, EVENT_NAME AS Name FROM INFORMATION_SCHEMA.EVENTS WHERE EVENT_SCHEMA=%s` (fallback `SHOW EVENTS FROM db`) |
| qHelpKeyword | `HELP %s` |
| qShowCreateTrigger | `SHOW CREATE TRIGGER :QuotedDatabase.:QuotedName` (>=5.1.21) |
| qShowWarnings | `SHOW WARNINGS` |
| qDropUser | `DROP USER %s@%s` (<4.1.1: `DELETE FROM mysql.user WHERE User=%s AND Host=%s`) |
| qCreateRole / qDropRole | `CREATE ROLE %s` / `DROP ROLE %s` |
| qGrantRole / qRevokeRole / qSetDefaultRole | `GRANT %s TO %s%s` / `REVOKE %s FROM %s` / `SET DEFAULT ROLE %s FOR %s` |
| qReloadPrivileges | `FLUSH PRIVILEGES` |
| qIndexVisible / qIndexInvisible | MariaDB >=10.6: `NOT IGNORED` / `IGNORED`; MySQL >=8.0: `VISIBLE` / `INVISIBLE` |
| qGetAuthPlugins | `SELECT PLUGIN_NAME FROM INFORMATION_SCHEMA.PLUGINS WHERE PLUGIN_TYPE='AUTHENTICATION' AND PLUGIN_STATUS='ACTIVE'` |
| qCastAsText | `CAST(%s AS CHAR)` |
| qIndexSize | `SELECT index_name, stat_value * @@innodb_page_size AS bytes FROM mysql.innodb_index_stats WHERE database_name=%s AND table_name=%s AND stat_name='size'` |

### Feature flags
`TDBConnection.Has` at `dbconnection.pas:6816-6840`:

| Flag | Condition |
|---|---|
| frSrid | MySQL >= 8.0 |
| frTemporalTypesFraction | MariaDB >= 5.3 or MySQL >= 5.6.4 |
| frIntegerDisplayWidth | not (MySQL >= 8.0.17) |
| frColumnDefaultParentheses | MySQL >= 8.0.13; expressions are emitted as `DEFAULT (expr)` |
| frEditVariables | >= 4.0.3 |
| frCreateView / Procedure / Function / Trigger / Event | 5.0.1 / 5.0.3 / 5.0.3 / 5.0.2 / 5.1 |
| frInvisibleColumns | MariaDB >= 10.3.3 or MySQL >= 8.0.23 |
| frCompressedColumns | MariaDB >= 10.3.1 |

### Data types and server variables
- **Data type list** (`dbstructures.mysql.pas:334`): TINYINT, BOOLEAN, SMALLINT, MEDIUMINT, INT, BIGINT, FLOAT, DOUBLE, DECIMAL, DATE, TIME, YEAR, DATETIME, TIMESTAMP, VARCHAR, CHAR, TINYTEXT, TEXT, MEDIUMTEXT, LONGTEXT, JSON, UUID, INET4, INET6, BINARY, VARBINARY, TINYBLOB, BLOB, MEDIUMBLOB, LONGBLOB, VECTOR, ENUM, SET, BIT, POINT, LINESTRING, POLYGON, GEOMETRY, MULTIPOINT, MULTILINESTRING, MULTIPOLYGON, GEOMETRYCOLLECTION.
  - Each type has: HasLength, RequiresLength, MaxSize, HasBinary, HasDefault, LoadPart (whether the grid truncates it), Category (Integer/Real/Text/Binary/Temporal/Spatial/Other), DefLengthSet and a description.
- **Server variable metadata** `MySQLVariables` (`dbstructures.mysql.pas:1036`): a table of {Name, IsDynamic, VarScope (Global/Session/Both), EnumValues}. The variable editor depends on it.
- **SQL functions list** is loaded from per-version files. The lookup order is `mariadb<maj.min>,mariadb<maj>,mariadb,mysql`, or `mysql<maj.min>,...` (`dbconnection.pas:3485-3504`).

### Escaping (`dbconnection.pas:5276-5400`)
- Numeric values are emitted unquoted when they are numeric or hex.
- BIT values are emitted as `b'...'`.
- Binary values are emitted as `0x<HEX>`. The "Lowercase hex" preference applies.
- The grid emits binary values as `_binary 0x…` in the SQL export.

---

## 1. Session manager (`connections.pas`, `connections.dfm`, `dbconnection.pas` TConnectionParameters 306-412)

### Session list (left pane)
- Tree of sessions and **folders**. Folders are stored as a registry key with a `Folder` value.
- Actions: rename inline, drag & drop into folders, New (session/folder in root or in the selected folder), Save, "Duplicate / save as", Delete.
- "Folders at top" option and a text Filter box.
- Columns: Session name, Host, Port, User, Version, Last connect, Counter, Comment (`connections.dfm:956-1036`).
- Row background uses the session color.
- "More" menu: Preferences, Check for updates, Import settings file, Export settings file, Help, About.

### Settings tab
- **Network type**: `NetTypeName` (`dbconnection.pas:1654-1712`):
  - MySQL family: "MariaDB or MySQL (TCP/IP)", "(named pipe)", "(SSH tunnel)", "ProxySQL Admin (Experimental)", "MySQL on RDS".
  - Other DBs: MSSQL (named pipe/TCP/SPX/VINES/RPC), PostgreSQL (TCP/IP/SSH), SQLite and SQLite encrypted, Interbase/Firebird (TCP/local, experimental).
- **Library**: client DLL. Default is `libmariadb.dll`.
- **Hostname / IP**:
  - For named pipe, this field holds the pipe/socket name; the connection uses host `.` and socket = hostname (`dbconnection.pas:2523-2526`).
- **User**, **Password**, **Port** (default 3306; ProxySQL 6032).
- **Prompt for credentials**: a login dialog at connect time (`dbconnection.pas:3276-3291`).
- **Use Windows authentication**: sends the OS user name and an empty password.
- **Compressed client/server protocol**: sets `CLIENT_COMPRESS`.
- **Databases**: semicolon-separated list. Its "..." button connects and offers a checklist from `SHOW DATABASES` (`connections.pas:1288-1355`). When set, only these DBs appear in the tree (`GetAllDatabases`, `dbconnection.pas:4910-4943`).
- **Comment** (memo).

### SSH tunnel tab
Fields:
- Use SSH tunnel (checkbox)
- SSH executable (plink or OpenSSH)
- SSH host + port (default 22)
- Username, Password
- Private key file
- SSH timeout (seconds, default 4)
- Local port (default = port+1)

The command is built at `dbconnection.pas:1104-1160`:
`<exe> [-ssh [-legacy-stdio-prompts]] user@sshhost [-pw "pass"] -P|-p port [-i "key"] [-o StrictHostKeyChecking=no] -N -L <localport>:<dbhost>:<dbport>`

- If the local port is busy, it increments the port up to 20 times.
- The DB connection then goes to `127.0.0.1:<localport>`.

### SSL tab (`dbconnection.pas:2474-2517`)
- Use SSL, SSL private key, SSL certificate, SSL CA certificate, SSL cipher.
- **Certificate verification** combo:
  - 0 = No verification (insecure)
  - 1 = Verify CA (insecure)
  - 2 = Verify CA and host name identity (default 2)
- libmysql maps these to SSL_MODE_PREFERRED / VERIFY_CA / VERIFY_IDENTITY; libmariadb uses the MYSQL_OPT_SSL_VERIFY_SERVER_CERT bool.
- TLS versions allowed: `TLSv1,TLSv1.1,TLSv1.2,TLSv1.3`.
- After connecting, it warns "SSL not used" if the `Ssl_cipher` status value is empty (`main.pas:4448-4455`).

### Advanced tab
- **Startup script**: a file of SQL run after connect. It is split with the batch splitter, and errors are only logged (`dbconnection.pas:3525-3541`).
- **Query timeout**: default 30. Used as `MYSQL_OPT_CONNECT_TIMEOUT`.
- **Ping every X seconds (keep-alive)**: default 20. A timer calls `mysql_ping` without reconnecting (`dbconnection.pas:3738-3743`).
- **Background color** (session color).
- **Hide database pattern**: a regex that removes matching DBs from the list (`dbconnection.pas:5025-5041`).
- **Log queries to file**: DDL and/or DML checkboxes plus a path. The default path is `<AppData>\Logs\%session\%db\%y%m%d.sql`.
- **Use own client time zone**: runs `SET time_zone='+hh:mm'` computed from the OS timezone (`dbconnection.pas:3506-3523`).
- **Get full table status**: default on. When on, the object list uses `SHOW TABLE STATUS`; when off, a lighter IS query is used (see §2).
- **Enable cleartext authentication**: `MYSQL_ENABLE_CLEARTEXT_PLUGIN`.
- **Force Unicode**: default on. When off, the IsUnicode flag follows the connection charset.

### Statistics tab
Created, Last connect, Successful connects (counter), Unsuccessful connects.

### Connect sequence (MySQL) (`dbconnection.pas:2449-2697`, `3418-3567`)
- **Client flags**: `CLIENT_LOCAL_FILES | INTERACTIVE | PROTOCOL_41 | MULTI_STATEMENTS | CAN_HANDLE_EXPIRED_PASSWORDS | PLUGIN_AUTH_LENENC_CLIENT_DATA` (+COMPRESS, +SSL).
- Connect attribute `program_name`=HeidiSQL. `LOCAL_INFILE` is enabled.
- After connecting it pings to catch an `init_connect` failure.
- **Expired password (error 1820)**: shows a dialog (`change_password.pas`), then runs `ALTER USER USER() IDENTIFIED BY '…'` or `SET PASSWORD=PASSWORD('…')`.
- Version string: `SHOW VARIABLES` → `version` + `version_comment`.
- Charset: tries `mysql_set_character_set('utf8mb4')`, then `utf8`, then `SET NAMES utf8`.
- Global status provides `Uptime` and `Ssl_cipher`. `SELECT NOW()` stores the server time at connect, used for the status-bar clock.
- `version_compile_os`, `hostname`, `lower_case_table_names` set identifier case sensitivity.
- `SHOW TABLES FROM information_schema` caches the list of available IS tables.
- `SELECT CONNECTION_ID()` provides the thread id.

### Other connection behavior
- **Auto-reconnect**: every `Query()` first calls `Ping(True)`, which reconnects if the connection is dead (`dbconnection.pas:3749-3765`, `3609-3633`).
- **USE detection**: user SQL starting with `USE x` updates the current database (`dbconnection.pas:4530-4554`).
- **Last used DB** is saved per session (`asLastUsedDB`) and restored on connect if the "Restore last used database" preference is on (`main.pas:4428-4441`).
- Connect and refused counters are kept per session.

---

## 2. Database tree (`main.pas`)

### Node types
Session (`lntNone`), Database, Group (when "Group objects by type" is on: Tables, Views, Procedures, Functions, Triggers, Events), Table, View, Function, Procedure, Trigger, Event, Column (when "Columns below tables in tree" is on).

### Size column
- Session row: sum over all DBs, only once all are cached.
- DB row: sum of `Data_length + Index_length` for its objects.
- Table row: its own size.
- Toggle: "Display size of objects" (`main.pas:9747-9827`).

### Object loading per DB (`FetchDbObjects`, `dbconnection.pas:7201-7393`)
1. Collation:
   `SELECT DEFAULT_COLLATION_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME='db'`
2. Tables and views. With full table status on (or for information_schema):
   `SHOW TABLE STATUS FROM db`
   Otherwise the light query:
   `SELECT TABLE_NAME AS Name, ENGINE AS Engine, VERSION AS Version, TABLE_COLLATION AS Collation, TABLE_COMMENT AS Comment, NULL AS Create_time, NULL AS Update_time, NULL AS Data_length, NULL AS Index_length, NULL AS Rows, NULL AS Auto_increment, NULL AS Row_format, NULL AS Avg_row_length, NULL AS Max_data_length, NULL AS Data_free, NULL AS Check_time, NULL AS Checksum, NULL AS Create_options FROM information_schema.TABLES WHERE TABLE_SCHEMA='db' AND TABLE_TYPE IN('BASE TABLE','VIEW')`
   - An object is a view when Engine and Version are NULL.
   - The InnoDB "free:" suffix is stripped from the comment.
3. Then functions, procedures, triggers and events, using the provider templates above.
   - Trigger comment is synthesized as `"<Timing> <Event> in table `t`"`.
4. Results are cached per DB. Columns, keys, FKs and check constraints are cached per table.

### Tree filters
- **Database filter** and **Table filter** boxes: regex; `;` acts as OR; case-insensitive.
- Each box keeps a history dropdown with "Clear" and "Empty recent filters" (`main.pas:13175-13330`).

### Favorites
- Clicking the icon area of a table/view/routine/trigger/event node toggles it as a favorite.
- Favorites are stored per session (`asFavoriteObjects`) as object paths (`main.pas:14108-14131`).
- "Show only favorites" hides non-favorites.

### popupDB context menu (`main.dfm`)
- Edit (opens the object editor; for a DB node it opens the Alter Database dialog)
- Drop …, Empty table(s) …, "Select top 1000 rows", Run routine(s) …
- Create new → Database / Table / Table copy / View / Stored procedure / Stored function / Trigger / Event
- Clear data tab filter and sort order
- Export database as SQL, Maintenance, Find text on server, Bulk table editor, Generate data
- Expand all / Collapse all
- Group objects by type, Display size of objects, Auto expand on click, Doubleclick inserts node text, Show only favorites, Columns below tables in tree
- Print…, Refresh, Full status refresh (Shift+F5), Connection properties, Disconnect
- SQLite only: Attach database / Detach database (`ATTACH DATABASE 'file' AS alias`, `DETACH DATABASE alias`, `main.pas:5728-5800`)

### Drop (`main.pas:3872-3966`)
- **Database**: `DROP DATABASE `db`` after a critical confirmation. Focus first moves to a sibling DB.
- **Objects**: sorted, then run as
  `SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0;` + `DROP TABLE|VIEW|FUNCTION|PROCEDURE|TRIGGER|EVENT `name`` per object + restore FK checks.

### Empty tables (`main.pas:4583-4657`)
`TRUNCATE `t`` for each table. An optional "Disable foreign key checks" checkbox wraps the run with the FK-check toggles.

### Run routines (`main.pas:4698-4765`)
- Parses parameters from `SHOW CREATE` and prompts for each value.
- Generates `CALL `proc`('v1', …)` or `SELECT `func`(…)` in a new query tab and runs it.

### Select top rows (`main.pas:5140-5167`)
- Generates `SELECT * FROM `t` LIMIT 1000` in a new tab (count = "rows per step" setting) and runs it.
- With Shift held, the table name is qualified as `db.t`.

### Rename
- Inline rename in the DB tab object list: `RENAME TABLE `old` TO `new`` (`main.pas:7452-7490`).

### Refresh
- F5 refresh. "Full status refresh" temporarily forces full table status.

---

## 3. Host tab (`main.pas:11965-12212`, `9609-9705`)

Every list has a filter edit (`editFilterVT`, with history) and an auto-refresh option in the Refresh button dropdown with "Set interval …" (`main.pas:7650-7675`).

### Databases list
- Columns: Database, Size, Items, Last modification, Tables, Views, Functions, Procedures, Triggers, Events, Default collation.
- Values are computed only from the cached object lists (`main.pas:11908-11960`).
- Bars are painted in the numeric columns.
- "Fetch database items" loads the object list for every DB. Double-click selects the DB.

### Variables list
- Columns: Variable, Session, Global. Sources: `SHOW VARIABLES` and `SHOW GLOBAL VARIABLES`.
- A cell is highlighted when the session value differs from the global value.
- Double-click or "Edit …" opens `editvar.pas`:
  - Rejected when the variable is non-dynamic or not in the internal list.
  - Editor type is detected as String / Number (up-down) / Enum (combo from EnumValues) / Boolean (On/Off).
  - Scope radio: This session / Global, enabled per VarScope.
  - SQL: `SET @@session.<name> = <val>` or `SET @@global.<name> = <val>`.
    - Strings that look numeric are not quoted. Booleans become 1/0. Enums are quoted.
  - `SHOW WARNINGS` runs afterwards (`editvar.pas:155-200`).

### Status list
- Columns: Variable, Value, Avg per hour, Avg per second. Source: `SHOW GLOBAL STATUS`.
- Avg per hour = value / (uptime/3600); per second divides by a further 3600.
- `Bytes_*` values are byte-formatted (`main.pas:9630-9662`).

### Process list
- Query when IS.PROCESSLIST exists:
  `SELECT ID, USER, HOST, DB, COMMAND, TIME, STATE, LEFT(INFO, 51200) AS Info [, extra IS columns from index 8 on] FROM information_schema.PROCESSLIST`
- Otherwise: `SHOW FULL PROCESSLIST`.
- Columns: id, User, Host, DB, Command, Time, State, Info, plus extra columns.
- Icons: idle (<60s), long idle, running, own thread overlay, Killed overlay. A bar is painted on Time.
- A lower panel shows the focused process SQL with an **EXPLAIN** link. It opens a new tab with `USE `db`;` + `EXPLAIN <sql>;` and runs it (`main.pas:14265-14283`).
- **Kill Process(es)…** runs `KILL QUERY <id>` per selected row (`main.pas:6791-6824`).
  - Your own thread id is skipped. ER_NO_SUCH_THREAD is ignored.
  - Note: the menu says "Kill" but the code uses qKillQuery, not KILL CONNECTION.

### Command statistics
- Columns: Command-type (with `Com_` stripped and `_` → space), Total count, Average per hour, Average per second, Percentage (with bar).
- Source: `SHOW GLOBAL STATUS LIKE 'Com\_%'`.

### Flush menu (Tools) (`main.pas:1605-1636`)
- `FLUSH HOSTS|LOGS|PRIVILEGES|TABLES|STATUS`.
- `FLUSH TABLES WITH READ LOCK` shows a modal "perform backup now", then runs `UNLOCK TABLES`.

Other DBs: variables and processes are also available on MSSQL/PostgreSQL; Status and Command statistics are MySQL-only.

---

## 4. Database tab (object list `ListTables`) (`main.dfm`, `main.pas:6474-6650`)

- Columns: Name, Rows, Size, Created, Updated, Engine, Comment, Version, Row format, Avg row length, Max data length, Index length, Data free, Auto increment, Check time, Collation, Checksum, Create options, Type. Bars are painted on Rows and Size.
- Status bar text example: `db: N objects (x tables, y views …)`. With multiple rows selected: "Selected: N".
- Inline rename for tables and views. Double-click opens the editor.
- Column visibility, width, order and sort are persisted (`asListCol*`).
- The header context menu toggles columns. "Copy all lines from listing in CSV" exists.

---

## 5. Table editor (`table_editor.pas`, `table_editor.dfm`)

### Tabs
Basic (Name, Comment), Options, Indexes, Foreign keys, Check constraints, Partitions, CREATE code, ALTER code. Below the tabs: the column grid and Save / Discard / Help buttons.

### Options tab (`table_editor.pas:285-286, 383-445`)
- Auto increment, Default collation plus a **Convert data** checkbox.
- Engine: from `SHOW ENGINES` where Support is YES or DEFAULT, plus "<Server default>" (`dbconnection.pas:5615-5661`).
- Average row length, Maximum row count, Checksum for rows.
- Row format: DEFAULT, DYNAMIC, FIXED, COMPRESSED, REDUNDANT, COMPACT.
- Union tables (MERGE engine) and INSERT method (NO, FIRST, LAST), both MERGE-only.
- When editing, these values are parsed from `SHOW CREATE TABLE` via the regex `\s(\S+)\s*=\s*(\S+)`.
- The comment is taken from table status. The comment max length is 60 on servers older than 5.5.3, otherwise 2048.

### Column grid
Columns: #, Name, Datatype, Length/Set, Unsigned, Allow NULL, Zerofill, Default, Comment, Collation, Expression, Virtuality, SRID, Invisible, Compressed.
- Expression, Virtuality and Compressed are visible only on MariaDB.

Editing rules (`table_editor.pas:1461-1534`):
- Unsigned and Zerofill only for integer/real types except BIT.
- Allow NULL is locked off for PK columns.
- Collation is locked when "Convert data" is checked.
- SRID only for spatial types with frSrid.
- Invisible and Compressed follow the feature flags.

Editors:
- Datatype: dropdown with descriptions.
- Collation: list from `SHOW COLLATION`.
- Virtuality: MariaDB `,VIRTUAL,PERSISTENT`; MySQL `,VIRTUAL,STORED`.
- **Default editor** (`grideditlinks.pas:1310-1401`): radio choices "No default value", "Custom text: …", "NULL", "Expression: …" (combo of SQL functions), "AUTO_INCREMENT", plus "On update: …" (expression).

Changing the data type (`table_editor.pas:1690-1740`):
- Resets the length if the type has none.
- Clears index subparts.
- Suggests the default length (`DefLengthSet`).
- Auto-fixes the default type.

Toolbar and context menu:
- Add (copies the type, length, unsigned and nullability of the focused column; an AUTO_INCREMENT source becomes default '0'), Remove, Up, Down, drag-reorder.
- Copy selected columns / Paste columns (serialized).
- Create new index (PRIMARY/KEY/UNIQUE/FULLTEXT/SPATIAL) from selected columns, with a duplicate-key warning; the name is the column names joined with `_`.
- Add to index → existing index (`table_editor.pas:2860-2920`).

### Column SQL (`TTableColumn.SQLCode`, `dbconnection.pas:10936-11050`)
Order of parts:
`Q(name) TYPE[(len)] [UNSIGNED] [ZEROFILL] [/*!100301 COMPRESSED*/] [NOT NULL|NULL] [SRID n] [DEFAULT 'text'|DEFAULT NULL|AUTO_INCREMENT|DEFAULT expr|DEFAULT (expr)] [ON UPDATE expr] [AS (expr) VIRTUAL|STORED|PERSISTENT] [INVISIBLE] [COMMENT '…'] [COLLATE 'coll']`

- Virtual columns omit NULL/DEFAULT/COLLATE.
- JSON columns omit COLLATE.

Reading columns uses `SELECT * FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=… AND TABLE_NAME=… ORDER BY ORDINAL_POSITION` (`dbconnection.pas:5846-5977`):
- Virtuality comes from EXTRA `(\w+) generated`. Invisible comes from EXTRA. ON UPDATE comes from EXTRA.
- `auto_increment` in EXTRA marks the column as AutoInc.
- Literal text vs expression default is decided by `IsTextDefault` (`dbconnection.pas:5806-5843`):
  - MariaDB >=10.2.7 quotes literal defaults.
  - MySQL >=8.0.13 heuristic: a value containing `(` is an expression.
- Fallback for pre-5.0 servers: `SHOW FULL COLUMNS FROM db.t`.

### Indexes
Reading:
- `SHOW KEYS FROM `t` FROM `db``.
- Index size from `mysql.innodb_index_stats`.
- Visible/Ignored columns are read when present.
- Functional key parts (the `Expression` column) are wrapped in `(…)` (`dbconnection.pas:6206-6320`).

Grid columns: Name, Type / Length, Algorithm, Comment, Direction, Visibility, Size.
- Types: PRIMARY, KEY, UNIQUE, FULLTEXT, SPATIAL; VECTOR is detected on read.
- Algorithm: blank, BTREE, HASH, RTREE.
- Visibility: blank / VISIBLE|NOT IGNORED / INVISIBLE|IGNORED.
- Part-level: column name with an optional `col(20)` prefix length (text/binary only) and Direction ASC/DESC.
- Up/Down, drag & drop from the column list.

SQL (`dbconnection.pas:11301-11364`):
`PRIMARY KEY (…)` | `[UNIQUE|FULLTEXT|SPATIAL ]INDEX `name` (`c1`(len) DESC, (expr)…) [USING BTREE] [COMMENT '…']`

### Foreign keys
Reading (`dbconnection.pas:6450-6513`):
- `SELECT * FROM information_schema.REFERENTIAL_CONSTRAINTS WHERE CONSTRAINT_SCHEMA=db AND TABLE_NAME=t AND REFERENCED_TABLE_NAME IS NOT NULL`
- Plus `KEY_COLUMN_USAGE` with the same filter. The two are joined client-side.

Grid columns: Key name, Columns (multi-select), Reference table (combo of DB tables, custom `db.table` allowed), Foreign columns (multi-select), On UPDATE, On DELETE.
- Actions: RESTRICT, CASCADE, SET NULL, NO ACTION. Default is NO ACTION.
- Auto name: `FK_<table25>_<reftable25>[_n]`.
- Validates column counts and type/unsigned match.
- **Reverse** button lists referencing tables (qGetReverseForeignKeys).

SQL (`dbconnection.pas:11445-11478`):
`CONSTRAINT `name` FOREIGN KEY (`c`) REFERENCES [`refdb`.]`reftable` (`rc`) ON UPDATE x ON DELETE y`

### Check constraints
Grid: Name, Check clause.

Reading:
- MariaDB: `SELECT CONSTRAINT_NAME, CHECK_CLAUSE FROM information_schema.CHECK_CONSTRAINTS WHERE CONSTRAINT_SCHEMA=db AND TABLE_NAME=t`
- MySQL: join of `CHECK_CONSTRAINTS cc` and `TABLE_CONSTRAINTS tc` on schema and name, with `tc.CONSTRAINT_TYPE='CHECK'` (`dbconnection.pas:6719-6768`).

SQL: `CONSTRAINT `n` CHECK (clause)`.

### Partitions
- Free-text memo, prefilled from `PARTITION BY …` in the CREATE code.
- Only used in CREATE as `/*!50100 <text> */`. ALTER does not apply partition changes.

### CREATE (new table) (`table_editor.pas:976-1063`)
```
CREATE TABLE `name` (
  <col>, …, <keys>, <fks with CONSTRAINT>, <checks>
)
COMMENT='…'
COLLATE='…'
ENGINE=…
ROW_FORMAT=… CHECKSUM=1 AUTO_INCREMENT=… AVG_ROW_LENGTH=… MAX_ROWS=… UNION=(…) INSERT_METHOD=…
/*!50100 PARTITION BY … */;
```

### ALTER generation (`table_editor.pas:638-973`), in order
1. Modified FKs are dropped in a separate statement: `ALTER TABLE t DROP FOREIGN KEY `old`;`
2. Columns whose default was removed: `ALTER TABLE t ALTER `old` DROP DEFAULT;`
3. A combined `ALTER TABLE `t`` with clauses joined by `,\n  `:
   - Table options when changed: `COMMENT='…'`, `COLLATE='…'`, `ENGINE=…`, `ROW_FORMAT=…`, `CHECKSUM=0|1`, `AUTO_INCREMENT=n`, `AVG_ROW_LENGTH=n`, `MAX_ROWS=n`, `UNION=(…)`, `INSERT_METHOD=…`.
   - With "Convert data": `CONVERT TO CHARSET <cs> COLLATE '<coll>'`. Every column collation is overridden.
   - Each non-untouched column, in grid order:
     - Modified: `CHANGE COLUMN `old` <colsql> FIRST|AFTER `prev``. This handles renames and reordering in one clause.
     - Added: `ADD COLUMN <colsql> FIRST|AFTER `prev``.
   - Deleted columns: `DROP COLUMN `old``.
   - Deleted indexes: `DROP PRIMARY KEY` / `DROP INDEX `old``.
   - Modified indexes: drop as above, then `ADD <keysql>`. Added indexes: `ADD <keysql>`.
   - Deleted FKs: `DROP FOREIGN KEY `n``. Added or modified FKs: `ADD CONSTRAINT … FOREIGN KEY …`.
   - Deleted checks: `DROP CONSTRAINT `n``. Added or modified checks: `ADD CONSTRAINT … CHECK (…)`.
4. When visibility is supported, a separate statement for each modified index: `ALTER TABLE t ALTER INDEX `n` VISIBLE|INVISIBLE|IGNORED|NOT IGNORED`.
5. Table rename runs after the batch: `RENAME TABLE `old` TO `new`` (`table_editor.pas:546-553`).

Errors: on "constraint is incorrectly formed", the dialog appends the matching line from `SHOW ENGINE INNODB STATUS` (`table_editor.pas:578-585`).

---

## 6. Other object editors

### View (`view.pas:224-256`)
- Fields:
  - Name.
  - Definer: combo of `SELECT CONCAT(User,'@',Host) FROM mysql.user WHERE User!='' ORDER BY User, Host` (`dbconnection.pas:6971-6985`).
  - SQL security: Definer / Invoker.
  - Algorithm: UNDEFINED / MERGE / TEMPTABLE.
  - Check option: None / CASCADED / LOCAL.
  - Select statement memo, CREATE code tab.
- SQL:
  `CREATE|ALTER [ALGORITHM = X] [DEFINER=`u`@`h`] [SQL SECURITY X] VIEW `name` AS <select> [WITH CASCADED|LOCAL CHECK OPTION];` + `RENAME TABLE `old` TO `new`;` when renamed.
- Loading:
  - `SHOW CREATE VIEW`. Fallback: `information_schema.VIEWS`.
  - Also tries `LOAD_FILE(datadir/db/view.frm)` to get the original formatting (`dbconnection.pas:4247-4302`).
  - Parsed with a regex (`dbconnection.pas:7879-7918`).

### Stored routine (`routine_editor.pas`)
- Fields:
  - Name.
  - Type: Procedure / Function.
  - Returns: datatype combo, functions only.
  - Data access: Contains SQL / No SQL / Reads SQL data / Modifies SQL data.
  - SQL security: Definer / Invoker. Deterministic checkbox. Comment.
  - Definer: empty means the current user.
  - Parameters grid: #, Name, Datatype, Context IN/OUT/INOUT (functions are IN only). Add / Remove / Clear / Up / Down.
  - Body memo (default `BEGIN\n\nEND`), CREATE code tab, Run button.
- SQL (`routine_editor.pas:538-573`):
  ```
  CREATE [DEFINER=`u`@`h`] PROCEDURE|FUNCTION `name`(
    [IN|OUT|INOUT] `p` TYPE, …
  )
  RETURNS type
  LANGUAGE SQL
  [NOT ]DETERMINISTIC
  CONTAINS SQL
  SQL SECURITY DEFINER
  COMMENT '…'
  <body>
  ```
- Save of an existing routine (`routine_editor.pas:464-536`):
  1. Lists names with `SELECT ROUTINE_NAME FROM information_schema.ROUTINES WHERE ROUTINE_SCHEMA=… AND ROUTINE_TYPE=…`.
  2. Creates a temp routine `HeidiSQL_temproutine_N` as a syntax check, then `DROP … IF EXISTS` it.
  3. `DROP <oldtype> IF EXISTS `old``, plus a confirmed overwrite of the target, then `CREATE`.
- Parsing: regex on the CREATE code (`dbconnection.pas:7921-8027`).

### Trigger (`trigger_editor.pas`)
- Fields: Name, Definer, On table (combo of tables), Timing (BEFORE/AFTER), Event (INSERT/UPDATE/DELETE), Statement body.
- SQL: `CREATE [DEFINER=…] TRIGGER `n` BEFORE INSERT ON `tbl` FOR EACH ROW <body>`.
- Save: `DROP TRIGGER `old``, then `CREATE`. On failure it re-runs the old CREATE code to restore the trigger.

### Event (`event_editor.pas:76-310`)
- Fields:
  - Name, Comment, Definer.
  - Body (default `BEGIN END`).
  - Timing: "Once, at" (date + time) or "Every N <interval>". Interval values: YEAR, QUARTER, MONTH, DAY, HOUR, MINUTE, WEEK, SECOND, YEAR_MONTH, DAY_HOUR, DAY_MINUTE, DAY_SECOND, HOUR_MINUTE, HOUR_SECOND, MINUTE_SECOND. Composite intervals use a quoted text quantity.
  - Optional Starts at / Ends at.
  - "Drop event after expiration".
  - State: Enable / Disable / Disable on slave.
  - CREATE and ALTER code tabs.
- SQL:
  ```
  CREATE|ALTER [DEFINER=…] EVENT `n`
    ON SCHEDULE
      AT '<dt>' | EVERY n UNIT [STARTS '…'] [ENDS '…']
    ON COMPLETION [NOT] PRESERVE
    [RENAME TO `new`]
    ENABLE|DISABLE|DISABLE ON SLAVE
    COMMENT '…'
    DO <body>
  ```

### Create / alter database (`createdatabase.pas`)
- Fields: Name, Collation (with the server default shown from `collation_server`), CREATE code preview.
- Create: `CREATE DATABASE `n` /*!40100 COLLATE 'x' */`.
- Alter: `ALTER DATABASE `n` COLLATE 'x'`.
- **Rename** is emulated:
  1. Create the target DB.
  2. `RENAME TABLE `old`.`t` TO `new`.`t`, …` (refused if the DB has routines or name clashes).
  3. `DROP DATABASE old` if it is now empty.
- Current settings are read via `SHOW CREATE DATABASE`.

---

## 7. Data tab (grid)

### SELECT construction (`main.pas:5992-6277`)
- Key columns (PK, else a UNIQUE key with all NOT NULL columns, `dbconnection.pas:7733-7777`) are always selected.
- When a sufficient key exists, long "LoadPart" columns (TEXT/BLOB etc. with no length or length > 256) are fetched as `LEFT(CAST(col AS CHAR)?, 256)`. The full row is fetched on demand:
  `SELECT cols FROM `db`.`t` WHERE <key> [LIMIT 1]` (`EnsureFullRow`, `dbconnection.pas:9780-9817`).
- Uses `*` when all columns are full and none are INVISIBLE.
- Query shape: `SELECT … FROM `db`.`t` [WHERE <filter>] [ORDER BY …] LIMIT [offset,] n`.
- **Row limit**: initial "rows per step" (default 1000). "Next" (Ctrl+PgDn) loads the next step using an offset. "Show all" (Ctrl+End) loads up to "maximum rows" (default 100000).
- Label above the grid: "db.t: N rows total (approximately|exact), limited to X / Y rows match to filter". "Query exact row count" switches to `SELECT COUNT(*)` (`main.pas:6292-6369`).

### Filter panel
- WHERE memo; a leading `WHERE` is stripped.
- "Apply filter" keeps 20 recent filters per table.
- A "search text" box generates `CAST(col AS CHAR) LIKE '%txt%' OR …` over all columns (`main.pas:10684-10739`). "Always generate filter" option.
- Filter, sort and hidden columns are remembered per table (`asFilter`, `asSort`, `asHiddenColumns`) when "Remember filters, sorting and column selection across sessions" is on.

### Sorting
- **Header click** cycles ASC → DESC → removed. Shift starts with DESC (`main.pas:10997-11047`).
- The Sorting dialog (`data_sorting.pas`) supports multiple columns with direction toggle and delete.
- ORDER BY: `` `c` ASC, `d` DESC `` (`apphelpers.pas:1903-1919`).
- "Reset sorting" action. Query-result grids sort locally in memory, with a warning above N rows (`asQueryGridLongSortRowNum`).

### Column selection (`column_selection.pas`)
Checklist with All toggle, filter, "Sort alphabetically" and "Show static row id column". Right-clicking the header opens it.

### Quick filters (`main.pas:8185-8360`, `7524-7601`)
The menu captions show the generated clause. Shift+click ANDs the clause with the existing filter.
- **Focused values** (multi-row selection): `col IN (…)`, `col NOT IN (…)`, `col LIKE 'v%'`, `LIKE '%v'`, `LIKE '%v%'`, `col > v`, `col < v`, each OR-joined, plus `IS NULL` handling.
- **Prompt**: `=`, `!=`, `>`, `<`, `LIKE %…%`.
- `IS NULL`, `IS NOT NULL`.
- **Clipboard** (when <1KB): `=`, `!=`, `>`, `<`, `LIKE %…%`, `IN (clipboard)`.
- **More values**:
  `SELECT `c`, COUNT(*) AS c FROM `t` [WHERE filter] GROUP BY `c` ORDER BY c DESC, `c` LIMIT 50`
  Skipped for tables over 2GB (10GB if indexed); ENUM/SET values are offered instead.
- Remove filter.

### Editing (`dbconnection.pas:9643-10311`)
- **Editability**: requires a PK, or a UNIQUE key whose columns are all NOT NULL. Otherwise all columns are used in WHERE plus `LIMIT 1`.
- **WHERE** (`GetWhereClause`): `CAST(col AS CHAR)` for date/time/JSON/unknown types, otherwise the plain column, then:
  - `IS NULL`
  - `=num` (empty becomes 0)
  - `=b'…'` for BIT
  - `='datetime'`
  - `=0xHEX` for binary
  - `='text'` otherwise
- **UPDATE**: `UPDATE `db`.`t` SET `c`=val, … WHERE <where> [LIMIT 1 if no key]`. An error is raised if 0 rows were affected.
- **INSERT**: `INSERT INTO `db`.`t` (`c`,…) VALUES (…)`. If an auto-increment column is 0 or empty, it reads `SELECT LAST_INSERT_ID()`.
  - New rows are pre-filled with text defaults and simple integer defaults.
- **DELETE**: `DELETE FROM `db`.`t` WHERE <where> [LIMIT 1]`, per selected row, after confirmation (`main.pas:4486-4541`).
- Value formatting: NULL; a function value is raw (e.g. `NOW()`); int/real unquoted when numeric; binary/spatial as hex; temporal quoted.
- **Actions**:
  - Insert row (Ins), Duplicate row without keys (Ctrl+Ins), Duplicate row with keys (copies values, skips PK and virtual columns as applicable) (`main.pas:5490-5541`).
  - Post (Ctrl+Enter), Cancel (Esc), Delete (Ctrl+Del).
  - Set NULL (Ctrl+Shift+N).
  - "SQL function" (Ctrl+F12) switches the cell editor to a function list, so the value is sent unquoted.
  - "Edit value without foreign key lookup".
- **Insert value** submenu: default value; local and UTC datetime, date, time, year, unix timestamp (from server `NOW()`); GUID variants (`main.pas:8473-8532`).
- Other: "This is a UNIX timestamp column" (displays and edits as a date, stored per table), Save BLOB to file, Insert files, Image preview panel, "View binary data as text (instead of HEX)", Open URL, Follow foreign key, Copy, "Copy as <export format>", Paste.
- **Follow foreign key**: jumps to the referenced table with filter `` `refcol`='value' `` (`main.pas:12215-12268`).

### Cell editors (`main.pas:11285-11452`, `grideditlinks.pas`)
- **FK dropdown**, when the "Show values in foreign key columns" preference is on (`main.pas:11316-11366`). It uses the first text column of the referenced table as the label:
  `SELECT `fk`, LEFT(`textcol`, 256) FROM `db`.`reftable` GROUP BY `fk`, `textcol` ORDER BY `textcol` LIMIT 10000`
  It is skipped when 10000 or more rows come back.
- ENUM/BOOL dropdown (preference). SET checkbox editor (preference).
- Date/time inplace editor (preference), optionally prefilled with "now" including microseconds.
- Hex popup editor for binary (`bineditor.pas`: wrap, load binary file).
- Text inplace editor with a "…" button for the popup text editor (`texteditor.pas`: wrap; linebreak style Windows/UNIX/Mac/Unicode/Mixed; load textfile; syntax highlighter selection; format code once/always; max length).

### Insert files (`insertfiles.pas:600-660`)
- Pick target DB, table and files.
- Per column, set a value expression with placeholders: `%filecontent%` (as `_binary 0x…` or text), `%filename%`, `%filepath%`, `%filesize%`, `%filedate%`, `%filedatetime%`, `%filetime%`, e.g. `UNIX_TIMESTAMP('%filedatetime%')`.
- Generates one `INSERT INTO … (…) VALUES (…)` per file.

---

## 8. Query tab

### Running queries
- Multiple query tabs: new, new in background, close, close all, close to the right, rename. Tabs are restored on startup. Each tab can be bound to a file (Load / Save / Save as / Save selection / snippets).
- Actions: Run (F9), Run selection (Ctrl+F9), Run current query (Ctrl+Shift+F9, the statement at the cursor), Explain current query (wraps the statement with qExplain).
- Run modes: "Send queries one by one" (default) or "Send batch in one go". The batch mode concatenates statements with `; ` up to `max_allowed_packet` (`apphelpers.pas:3302-3384`).
- "Stop on errors in batch mode".
- **Batch splitter** (`apphelpers.pas:3457-3556`):
  - Honors `--`, `#` and `/* */` comments, quotes and backslash escapes.
  - Supports a `DELIMITER xx` line and a custom delimiter (action "Set delimiter").
- **Unsafe query warning**: UPDATE/DELETE without WHERE asks for confirmation (preference) (`main.pas:3310-3334`).
- **Results**:
  - One result tab per result set, up to "Maximum result sets" (default 10).
  - Caption from `-- names: a,b,c` in the batch head, else the table name, plus "(Nr × Mc)".
  - Header icons show PK/UNIQUE/KEY; the header hint shows the source table. Previous/next result tab navigation.
- **Editable query results**: the same editing machinery applies when a result maps to one table with a usable key (`CheckEditable`).
- **Log line** after a run: "Affected rows: … Found rows: … Warnings: … Duration for N queries: x sec. (+ y sec. network)". Then `SHOW WARNINGS` output is logged.
- **Error navigation**: on "near '…' at line N" and "Duplicate entry" errors, the cursor moves to the offending SQL.

### Query helpers panel (`main.pas:76-82`, `14672-14790`, `14009-14087`)
- **Columns in <table>** (or parameters of a routine). Context menu: Generate SELECT / INSERT / UPDATE / DELETE using the selected columns and key-based WHERE with placeholder values.
- **SQL functions**: name plus declaration, grouped by category.
- **SQL keywords**.
- **Snippets**: `.sql` files in the snippets directory. Insert at cursor, Load, Rename, Delete, Explore folder.
- **Query history**:
  - Per session. Stores date, db, duration and SQL, grouped by day.
  - Keep N days (default 30); only successful batches of 5 or fewer statements and up to 1MB; deduplicated.
  - "Clear query history" (`main.pas:3560-3610`).
- **Query profile** (checkbox):
  - Before the run: `SET profiling=1`. After: `SHOW PROFILE` (Status, Duration). Then `SET profiling=0` (`main.pas:3328-3336`, `3547-3561`).
- **Bind parameters** (checkbox):
  - Detects `:name` with the regex `([^:\w]|^):\w+`. Values are entered in the tree.
  - Values are text-substituted before running, processed in descending name order (`main.pas:15805-15870`, `3288-3307`).

### Editor features
- Autocompletion (interval, mid-match, auto-uppercase keywords).
- Code folding with region markers. Toggle comment. Move line up/down.
- Word wrap. Tabs-to-spaces. Ctrl+wheel zoom.
- Insert function submenu. Find text (Ctrl+F), Replace (Ctrl+R), Find again (F3).
- "SQL help" (F1) and "Sequal Suggest" (AI prompt to `https://api.sequal.dev/simple-suggest`, with recent prompts).

**Find / replace** (`searchreplace.dfm`):
- Text to find, Replace with (`\n` and `\t` allowed).
- Options: Case sensitive, Whole words, Regular expression, Prompt on replace.
- Direction: Forward/Backward. Origin: From cursor/Entire scope. Scope: Global/Selection.
- "Search in" (which editor). Replace all.

**Reformatter** (`reformatter.pas`):
- Internal: uppercases keywords, functions and data types; inserts newlines before SELECT, FROM, JOIN, WHERE, GROUP, ORDER, HAVING, LIMIT, CREATE, DROP, UPDATE, INSERT, REPLACE, TRUNCATE, DELETE, but not between pairs like LEFT JOIN; collapses spaces.
- Online: `heidisql.com/sql-formatter.php` (indent, input) or `https://sqlformat.org/api/v1/format` (`reindent=1`, `keyword_case=upper`, `indent_width`).
- "Keep asking this question" checkbox.

**SQL help** (`sqlhelp.pas`): browses the server's `HELP 'CONTENTS'` / `HELP 'topic'` tree, showing the `description` and `example` columns.

### Run / load SQL file (`main.pas:4090-4350`)
- Files over 5MB prompt "Run file(s) directly" vs "Load into editor".
- Direct run: streams 20MB chunks, splits them, executes each statement with a progress dialog (queries, position, affected rows) and counts errors. Honors "Stop on errors".
- Load SQL keeps a recent-files menu with encoding selection.

### SQL log panel
- Categories: errors, user SQL, internal SQL, info, debug, script.
- Settings: last N lines (default 300), snip to N chars (default 2000), timestamps, horizontal scrollbar.
- Context menu: "Copy line to new query tab", Clear, Save as textfile, "Log to file" (session logs dir), Open log folder.
- Per-session DDL/DML log files.

---

## 9. Export grid rows (`exportgrid.pas`, `exportgrid.dfm`)

- **Row selection**: Selected rows / All loaded rows.
- **Output**: Copy to clipboard, or File (recent files, encoding, "Open file after creation").
- **Formats** (`exportgrid.pas:11-27`, `107-145`):
  - Excel CSV, Delimited text (CSV), HTML table (full XHTML doc with style), XML (`<table_data name>` with `<row><field name>`; or `<resultset statement>`), LaTeX (`tabular`), Textile, Jira Textile, PHP Array, Markdown Here (with `---:` alignment for numeric columns), JSON (`{"table"|"query": …, "rows":[{…}]}`), JSON Lines.
  - SQL INSERTs, SQL INSERT IGNOREs, SQL REPLACEs, SQL DELETEs/INSERTs (`DELETE FROM t WHERE <key>;` before each INSERT), SQL UPDATEs (`UPDATE t SET … WHERE <key>;`).
- **Options**:
  - Field separator (presets Tab/Comma/Semicolon; default `;`), Encloser (presets single/double quote; default empty), Line terminator (`\r\n`, `\n`, `\r`), NULL value (default `\N`; presets NUL/backspace/Ctrl+Z).
  - Include column names, Include auto increment column, Include SQL query, Remove linebreaks from contents, "Only focused column (%s)", "Save clipboard settings" (separate clipboard settings).
- "Copy as" in the grid context menu uses the same exporter.

---

## 10. Table tools (`tabletools.pas`)

Shared UI:
- A left tree of sessions → DBs → objects with checkboxes, database/table regex filters, Check all/none/invert/"Check …".
- Shows the size of the selected objects. A result grid and a progress bar.

### Maintenance (`tabletools.pas:1053-1071`, `641-670`)
- `<OP> TABLE `db`.`t` [QUICK] [FAST] [MEDIUM] [EXTENDED] [CHANGED] [USE_FRM] [FOR UPGRADE]`
- OP is one of CHECK, ANALYZE, CHECKSUM, OPTIMIZE, REPAIR. The result set is shown.
- Option availability:
  - QUICK: check / checksum / repair.
  - FAST, MEDIUM, CHANGED, FOR UPGRADE: check only.
  - EXTENDED: check / checksum / repair.
  - USE_FRM: repair only.
  - For CHECKSUM, QUICK and EXTENDED are mutually exclusive.

### Find text on server (`tabletools.pas:1160-1318`)
- Simple text tab:
  - Data type category filter, match type (both wildcards / exact / left / right / regex), case sensitive.
  - Case-insensitive MySQL: `LOWER(CONVERT(`c` USING <charset>)) LIKE '%x%'` or `REGEXP`.
  - Case-sensitive MySQL: `LIKE BINARY` / `REGEXP BINARY`.
  - Exact match on numeric columns: `=n`.
- SQL tab: free condition such as `> NOW()`, applied to every column (`c <cond> OR …`).
- Per table:
  `SELECT 'db' AS Database, 't' AS Table, COUNT(*) AS Found rows, CONCAT(ROUND(100 / <rows> * COUNT(*), 1), '%') AS Relevance FROM db.t WHERE …`
- Routines: occurrence count in `information_schema.routines.routine_definition`.
- "See results" opens the matching `SELECT * … WHERE …` in query tabs.

### SQL export (`tabletools.pas:1753-2203`, `473-544`)
- **Databases**: Drop (`DROP DATABASE IF EXISTS`), Create (`SHOW CREATE DATABASE` with `IF NOT EXISTS` injected, then `USE`).
- **Tables**: Drop (`DROP TABLE|VIEW|… IF EXISTS`), Create (`SHOW CREATE` with `IF NOT EXISTS`).
- **Data** modes: No data / Delete + insert (`DELETE FROM t` first, optionally `ALTER TABLE AUTO_INCREMENT=1`) / Insert / Insert ignore / Replace existing data (REPLACE INTO).
- **Max INSERT size** in KB (default 1024; 0 = single-row INSERTs). Rows are packed into multi-row `INSERT … VALUES\n (…),\n (…)` up to 90% of that size.
- **Output** targets: Single .sql file, ZIP-compressed .sql file, Directory (one file per object in DB subdirs: `<dir>/<db>/<type>-<name>.sql`), Clipboard, another Database on the same server, or another Server/session (with optional target DB). Direct DB/server output executes the statements.
- **Filename placeholders**: `%session %host %u %db %date %d %m %y %h %i %s` (`apphelpers.pas:2873-2921`).
- **Options menu**: Add comments, Wrap data DML in transactions (`BEGIN`/`COMMIT`), Remove AUTO_INCREMENT clauses, Remove DEFINER clauses, Copy mysqldump command.
- **File header**: comment block (Host, Server version, Server OS, HeidiSQL version), then:
  `/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */; SET NAMES …; /*!40103 SET @OLD_TIME_ZONE=@@TIME_ZONE */; /*!40103 SET TIME_ZONE='+00:00' */; /*!40014 SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0 */; /*!40101 SET @OLD_SQL_MODE=@@SQL_MODE, SQL_MODE='NO_AUTO_VALUE_ON_ZERO' */; /*!40111 SET @OLD_SQL_NOTES=@@SQL_NOTES, SQL_NOTES=0 */;`
- **Footer**: restores those values (`tabletools.pas:213-218`).
- Read session: `SQL_MODE=''` and `TIME_ZONE='+00:00'`.
- **Data reading**: `SELECT /* HeidiSQL ver */ * FROM db.t ORDER BY <pk cols> LIMIT offset, n`, with n sized to about 100MB per chunk using avg_row_length.
  - Binary values are written as `_binary 0x…`. Virtual columns are skipped. MRG_MYISAM and FEDERATED tables have no data exported.
- **Ordering**:
  - Views first become a placeholder `CREATE TABLE` with the view's columns (no defaults, VARCHAR(1)). A second pass drops it and writes the real view.
  - Triggers are exported last, wrapped in `SET @OLDTMP_SQL_MODE=…, SQL_MODE='<trigger sql_mode>'; DELIMITER // … // DELIMITER ;`.
  - Routines and events are wrapped in `DELIMITER //`.
- **mysqldump command**: `--add-drop-database`, `--no-create-db`, `--add-drop-table`, `--no-create-info`, `--no-data|--replace|--insert-ignore`, `--result-file|--tab`, `--databases …`, plus connection args (`--ssl* --host --port --user --password --compress --pipe --socket`) (`dbconnection.pas:1981-2027`).

### Bulk table editor (`tabletools.pas:2221-2294`)
- One statement per table: `ALTER TABLE db.t <specs joined by ", ">`.
- Options:
  - Move to database: `RENAME `newdb`.`t``. Views are recreated in the new DB, then dropped.
  - Change engine: `ENGINE X`.
  - Convert to charset: `CONVERT TO CHARSET cs [COLLATE 'x']`.
  - Change default collation: `COLLATE 'x'`.
  - Reset auto increment: `AUTO_INCREMENT=0`.

### Generate data (`tabletools.pas:2323+`)
- Settings: number of rows (default 1000), NULL percentage (default 10).
- Random values per data type. Auto-increment columns and NOW/CURRENT_TIMESTAMP-default columns are skipped.
- Runs with FK checks disabled.

---

## 11. CSV import (`loaddata.pas`, `csv_detector.pas`)

### Dialog fields
- Filename, Encoding.
- Fields terminated by (default `;`), enclosed by (default `"`) plus "optionally" (default on), escaped by (default `"`), Lines terminated by (default `\r\n`).
- Ignore first N lines (default 1).
- Low priority, Local number format (converts thousand/decimal separators), Truncate destination table before import, Keep dialog open.
- Duplicates: INSERT / INSERT IGNORE / REPLACE.
- Method: "Server parses file contents (LOAD DATA)" or "Client parses file contents".
- Destination DB, table, and a column checklist with ordering (up/down, all).

### Truncate
`TRUNCATE TABLE db.t`; falls back to `DELETE FROM`.

### Server parse (`loaddata.pas:381-455`)
```
LOAD DATA [LOW_PRIORITY] LOCAL INFILE '<path>' [IGNORE|REPLACE] INTO TABLE `db`.`t` [CHARACTER SET cs]
FIELDS TERMINATED BY '…' [OPTIONALLY] ENCLOSED BY '…' ESCAPED BY '…'
LINES TERMINATED BY '…' IGNORE n LINES
(`c1`, @ColVar1, …) [SET `c`=REPLACE(REPLACE(@ColVar1, '<thousands>', ''), '<decimal>', '.')]
```

### Client parse (`loaddata.pas:457-660`)
- A char-by-char parser.
- Builds `INSERT [LOW_PRIORITY] [IGNORE] INTO … (cols) VALUES (…),(…)` in packets of about 0.5MB, also capped by max rows per insert.
- Runs under `SET SQL_MODE=''`, then restores it.

### CSV layout detector (`csv_detector.pas`)
- "Scan file…" parses the file and infers column types:
  - Integer types sized by max value; float/decimal; `YYYY-MM-DD` → DATE; `YYYY-MM-DD hh:mm:ss` → DATETIME; text → VARCHAR rounded to 10 or a larger TEXT type.
  - NULL / empty / `\N` makes a column nullable. Header names are taken from the first row.
- Shows an editable `CREATE TABLE `db`.`<filename>` (…)` and runs it with "Ok, create table".

### Run SQL file
See §8.

---

## 12. User manager (`usermanager.pas`, `usermanager.dfm`)

### User list
- Source query:
  `SELECT user, host, <password|authentication_string|IF(LENGTH(password)>0,password,authentication_string)> AS password, is_role, default_role, plugin FROM mysql.user`
  Columns are probed first with `SHOW COLUMNS FROM mysql.user` (`usermanager.pas:396-432`).
- Warning icons:
  - Empty password.
  - Invalid hash length (native/old password plugin and length not 0/16/41).
  - `skip_name_resolve` is ON but the host needs name resolution.
  - Unknown grant.
- Filter box. Add user (defaults `Unnamed@localhost`), Add role, Clone, Delete.

### Credentials tab
- User name.
- From host, with presets: localhost, `192.168.%`, `10.122.%`, `%`.
- Password + repeat, with a generator for 6/8/10/12/30 characters.
- Plugin: None or active auth plugins. Warns "Empty password after plugin change!".
- Default role.

### Limitations tab
Queries/hour, Updates/hour, Connections/hour (>=4.0.2), Simultaneous connections (>=5.0.3).

### SSL tab
Require SSL: none / SSL / X509 / "Specify requirements…" (Cipher, Issuer, Subject).

### Privileges tab
- Tree of objects (global `*.*`, db, table/view, column, procedure/function) with privilege checkboxes. Colors: read = green, write = maroon, admin = navy. "Add object" uses `selectdbobject.pas` (tree or wildcard DB with `%` / `_`).
- Privilege sets by level (`usermanager.pas:312-390`):
  - Global: FILE, PROCESS, RELOAD, SHUTDOWN, REPLICATION CLIENT, REPLICATION SLAVE, SHOW DATABASES, SUPER, CREATE USER, CREATE TABLESPACE.
    - MariaDB >10.5.2 adds BINLOG ADMIN (replacing REPLICATION CLIENT), BINLOG REPLAY, CONNECTION ADMIN, FEDERATED ADMIN, READ_ONLY ADMIN, REPLICATION MASTER ADMIN, REPLICATION SLAVE ADMIN, SET USER.
    - MariaDB 10.5.9+ adds SLAVE MONITOR. MariaDB 10.3.4+ adds DELETE HISTORY.
  - DB: CREATE TEMPORARY TABLES, LOCK TABLES, CREATE ROUTINE, TRIGGER, EVENT, plus all table and routine privileges.
  - Table: ALTER, CREATE, DELETE, DROP, GRANT, INDEX, CREATE VIEW, SHOW VIEW, plus column privileges.
  - Routine: GRANT, EXECUTE, ALTER ROUTINE.
  - Column: INSERT, SELECT, UPDATE, REFERENCES.
  - Each higher level includes all lower-level privileges.

### Roles tab
Visible if `is_role` exists. Role name → No / Yes / Yes with ADMIN OPTION.

### Load
- `SHOW GRANTS FOR 'u'@'h'`, parsed with the regex at `usermanager.pas:731`.
- `SHOW CREATE USER 'u'@'h'` for REQUIRE and WITH limits.
- The password hash is read from the grant on pre-5.7.6 MySQL and on MariaDB.

### Save (`usermanager.pas:1370-1628`)
1. New user (>=5.0.2):
   `CREATE USER 'u'@'h' [IDENTIFIED BY 'pw' | IDENTIFIED BY PASSWORD '*hash' | IDENTIFIED WITH|VIA plugin BY|USING 'pw']`
   MariaDB uses `VIA` / `USING`; MySQL uses `WITH` / `BY`.
2. Per object, revokes then grants:
   - `REVOKE p1, GRANT OPTION, SELECT(`col`) ON <obj> FROM 'u'@'h'`
   - `GRANT p1, … | USAGE ON *.* | db.* | [TABLE|FUNCTION|PROCEDURE] db.obj TO 'u'@'h' [WITH GRANT OPTION]`
3. Global options:
   - `ALTER USER 'u'@'h' IDENTIFIED WITH|VIA plugin` (on plugin change)
   - `ALTER USER 'u'@'h' REQUIRE NONE|SSL|X509|CIPHER '…' AND ISSUER '…' AND SUBJECT '…'`
   - `ALTER USER 'u'@'h' WITH MAX_QUERIES_PER_HOUR n MAX_UPDATES_PER_HOUR n MAX_CONNECTIONS_PER_HOUR n MAX_USER_CONNECTIONS n`
4. Password: MySQL >=5.7.6 uses `SET PASSWORD FOR 'u'@'h' = 'pw'`; otherwise `= PASSWORD('pw')`.
5. Roles: `GRANT 'role' TO 'u'@'h'[ WITH ADMIN OPTION]`, `REVOKE 'role' FROM …`, `SET DEFAULT ROLE 'role'|NONE FOR 'u'@'h'`.
6. Rename: `RENAME USER 'old'@'h' TO 'new'@'h'`. Pre-5.0.2 servers instead get UPDATEs on `mysql.user/db/tables_priv/columns_priv`.
7. `FLUSH PRIVILEGES`.

### Delete, create role, clone
- Delete: `DROP USER 'u'@'h'` or `DROP ROLE 'r'`, then `FLUSH PRIVILEGES`. Pre-5.0.2 servers first run `REVOKE ALL PRIVILEGES ON *.* FROM …` and `REVOKE GRANT OPTION …`.
- Create role: `CREATE ROLE 'name'` from an input prompt.
- Clone: copies the grant lines (and password hash) to a new user.

### Not present
Password expiry and account locking: no `EXPIRE` or `ACCOUNT LOCK` UI or SQL exists anywhere in `usermanager.pas`.

### Change password dialog (`change_password.pas`)
- Shown only on the "expired password" connect error.
- Fields: new + repeat, generator for 6–30 characters, copy button.

---

## 13. Copy table, select object, sync

### Table copy (`copytable.pas:348-506`)
- Fields:
  - Target `db.newtable`.
  - Checkable element tree: Columns (each), Indexes (each), Foreign keys (each), Data.
  - "WHERE clause for data copying" with recent filters.
- If the target exists: confirm, then `DROP TABLE`.
- SQL:
  ```
  CREATE TABLE `db`.`new` (<cols>, <keys>, <fks without CONSTRAINT name>) COLLATE '…' ENGINE=… ROW_FORMAT=… AUTO_INCREMENT=… COMMENT='…'
  INSERT INTO `db`.`new` (cols) SELECT cols FROM `old` [WHERE …]
  ```
  - AUTO_INCREMENT is dropped from a column if its key is not selected. Virtual columns are left out of the data copy.

### Select database object (`selectdbobject.pas`)
Picker used by the user manager: tree selection, or a wildcard DB (`%`, `_`).

### Sync DB (`syncdb.pas`)
- Unfinished and not wired up: no reference from `main.pas`.
- Analyze only lists "Create missing <type>" with the CREATE code. Apply is an empty stub. Treat as non-existent.

---

## 14. Preferences (`preferences.dfm`, setting defaults at `apphelpers.pas:3852-4170`)

### General
- MySQL command line programs path (used for the mysql.exe launcher and mysqldump).
- Language, Custom snippets directory, GUI font and size, Style theme, Icon pack.
- Web search base URL (presets: Ecosia, Startpage, DDG, Baidu, …).
- Automatically reconnect to previously opened sessions on startup.
- Restore last used database on startup.
- Check for updates every N days (default 3) + also check nightly builds.
- Send usage statistics, Allow multiple instances, Display bars in list columns, Ctrl+wheel zoom.

### Logging
- Log last N lines (300). Snip lines to N chars (2000).
- Event types: Errors, User SQL, Internal SQL, Info, Debug, Import/script queries.
- Write SQL log to file. Horizontal scrollbar. Timestamps.
- Enable query history + keep days (30).

### SQL
- Editor font and size, Tab width (3), Maximum result sets (10).
- Highlighter element colors (foreground/background/bold/italic) with presets.
- Auto completion (enable, delay 500ms, find matches in the middle).
- Auto uppercase. Tabs to spaces.

### Grid formatting
- Maximum column width (300), data font.
- Rows per page (1000) and maximum (100000). Lines of text per grid row (1).
- Per-category text colors: numeric, real, text, binary, datetime, spatial, other.
- NULL background, alternating row background, same-text highlight.
- Sort warning row count (10000). Max decimal zeros for floats (-1 keeps all).
- Local number format, popup SQL text over result tabs, lowercase hex, show static row id column.

### Data editors
- Default linebreak style.
- Enable: BLOB/HEX editor, inplace date/time editor, prefill empty date/time, ENUM dropdown, SET checkbox editor.
- Remember filters, sorting and column selection across sessions.
- Show values in foreign key columns.
- Click on column headers toggles sorting. Incremental search.

### Shortcuts
Primary and secondary shortcut per action.

### Files and tabs
- Tab icon grayscale mode.
- Reformatter choice (Internal / heidisql.com / sqlformat.org / always ask).
- Prompt to save modified files on tab close.
- Reopen previous files and unsaved content.
- Close tab on double-click / middle-click.

Buttons: Restore defaults, Apply.

---

## 15. Miscellaneous

### Status bar panels (`main.pas:1463-1597`, `7399-7505`, `10153`)
| Panel | Content |
|---|---|
| 0 | Object counts / function description |
| 1 | Selection count or editor `rN : cN` |
| 2 | Connected time |
| 3 | Server type + version. Hover shows connection info: host, net type, real hostname, OS, version, port, compression, unicode, SSL, max_allowed_packet, client version, plus `mysql_stat()` values (Uptime, Threads, Questions…). Click opens connection properties. |
| 4 | Server uptime |
| 5 | Server time (computed client-side from startup NOW + elapsed) |
| 6 | Busy/idle message |

### Startup and settings
- **Command line** (`apphelpers.pas:2679-2850`):
  `-d/--description=<session>`, `-n/--nettype`, `-h/--host`, `-l/--library`, `-u/--user`, `-p/--password`, `-cte/--cleartextenabled`, `-S/--socket` (implies named pipe), `-P/--port`, `-db/--databases`, `-W/--winauth`, `--ssl`, `--sslpk`, `--sslca`, `--sslcert`, `--sslcip`, `--sslvrf`, `--ssh-executable/-se`, `-sh`, `-sP`, `-sLP`, `-su`, `-sp`, `-sk`, `-st`, `--runfrom`, `--psettings=<file>` (portable settings file; default `portable_settings.txt`). `.sql` filenames are opened.
- **Settings export/import** (`apphelpers.pas:4621-4720`): text file with lines `regpath<DELIM>type<DELIM>value`. Import does not overwrite existing values.
- **Auto-reconnect on startup** (`main.pas:2328-2345`) reopens `LastSessions` and focuses `LastActiveSession`.
- **Keep-alive ping** and auto-reconnect are described in §1.

### Other features
- **Update check** (`updatecheck.pas:152`): GET `heidisql.com/updatecheck.php?r=<rev>&bits=..` returns an ini; can schedule an update task.
- **Launch command line** (`main.pas:3968-4008`): runs `mysql.exe` with the generated CLI args (password masked in the log).
- **Print list** (`printlist.pas`): prints the current list/grid (brief).
- **JumpList** (`JumpList.pas`): Windows taskbar recent sessions (Windows-only, skip).
- **New window**, Exit, About, Help, web links (download, forum, changelog), donate check.
- **Clipboard**: Copy, Copy formatted, Copy with tabs to spaces, Copy column names, "Copy all lines from listing or tree in CSV format".
- **Navigation**: Go to DB tree (Ctrl+D), table filter (Ctrl+E), query/results (F6), multi-column filter (F7), tabs 1–5 (Ctrl+1..5), previous/next tab. "Reset panel dimensions".

### Other-DB-only, mentioned briefly
- SQLite: attach/detach DB, cipher list (sqlite3mc).
- PostgreSQL: schemas instead of DBs, SERIAL, `COMMENT ON`.
- MSSQL: `sp_helptext`, TOP / OFFSET-FETCH.
- Interbase/Firebird: mostly stubs.
- ProxySQL Admin: uses `stats_*` tables and `pragma_table_info`.
- RDS: kill via `mysql.rds_kill*`.
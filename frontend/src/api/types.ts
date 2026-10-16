// Types mirroring the Go structures exchanged through Wails bindings.

export type DbKind = 'mysql' | 'mariadb'
export type TLSMode = 'disabled' | 'preferred' | 'required' | 'verify-ca' | 'verify-full'
export type Environment = 'local' | 'development' | 'staging' | 'production'

export interface TLSOptions {
  mode: TLSMode
  caFile?: string
  certFile?: string
  keyFile?: string
  serverName?: string
}

export interface Profile {
  id: string
  name: string
  kind: DbKind
  host: string
  port: number
  user: string
  database: string
  tls: TLSOptions
  params: Record<string, string>
  allowCleartextPasswords: boolean
  environment: Environment
  color: string
  readOnly: boolean
  savePassword: boolean
  connectTimeoutSeconds: number
  /** Unix socket path used instead of host:port. */
  socket: string
  ssh: SSHOptions
  /** Statements run on every new connection. */
  initSQL: string
  keepAliveSeconds: number
  /** "A/B" nests folders in the tree. */
  folder: string
  comment: string
  /** Names or patterns (* ?) separated by ";". */
  databaseFilter: string
  stats?: ConnStats
  createdAt?: string
  updatedAt?: string
}

export type SSHAuth = 'password' | 'key' | 'agent'

export interface SSHOptions {
  enabled: boolean
  host: string
  port: number
  user: string
  auth: SSHAuth
  keyFile: string
  saveSecret: boolean
}

export interface ConnStats {
  connects: number
  failures: number
  lastConnectedAt: string
}

export interface ProfileView extends Profile {
  hasStoredPassword: boolean
  hasStoredSSHSecret: boolean
}

export interface UnknownHost {
  host: string
  keyType: string
  fingerprint: string
}

export interface ImportResult {
  imported: number
  problems: string[] | null
}

export interface SaveResult {
  profile: ProfileView
  warning?: string
}

export interface ServerInfo {
  version: string
  flavor: string
  tlsActive: boolean
  tlsCipher?: string
  user: string
}

export interface SessionState {
  profileId: string
  status: 'connecting' | 'connected'
  server: ServerInfo
  openedAt: string
}

export interface DatabaseInfo {
  name: string
  system: boolean
}

export interface TableInfo {
  name: string
  type: 'table' | 'view' | 'system view'
  engine?: string
  rows?: number
  comment?: string
}

export interface ColumnInfo {
  name: string
  position: number
  dataType: string
  columnType: string
  nullable: boolean
  default?: string
  key?: string
  extra?: string
  comment?: string
}

export interface IndexInfo {
  name: string
  columns: string[]
  unique: boolean
  primary: boolean
  type: string
}

export interface RoutineInfo {
  name: string
  type: string
}

export interface DBError {
  code?: number
  sqlState?: string
  message: string
  line?: number
  near?: string
  category: string
}

export type Category = 'read' | 'write' | 'ddl' | 'transaction' | 'session' | 'other'

export interface ResultColumn {
  name: string
  type: string
  nullable?: boolean
  binary: boolean
  numeric: boolean
}

/** A cell: null is SQL NULL; large integers arrive as strings. */
export type Cell = string | number | boolean | null

export interface StatementResult {
  index: number
  sql: string
  line: number
  kind: Category
  columns: ResultColumn[] | null
  rows: Cell[][] | null
  hasRows: boolean
  hasMore: boolean
  limitReached: boolean
  cursorId?: string
  truncatedCells?: [number, number][]
  rowsAffected?: number
  lastInsertId?: number
  durationMs: number
  error?: DBError
  warnings?: SqlWarning[] | null
  /** Extra result with the statement profile (SHOW PROFILE[S]). */
  profile?: boolean
}

export interface SqlWarning {
  level: string
  code: number
  message: string
}

export interface ExecuteRequest {
  tabId: string
  connectionId: string
  sql: string
  cursorOffset: number
  mode: 'statement' | 'script'
  database: string
  confirmed: boolean
  source?: 'editor' | 'data'
  profile?: boolean
}

export interface ExecuteResponse {
  executionId: string
  results: StatementResult[] | null
  database: string
  inTransaction: boolean
  cancelled: boolean
  durationMs: number
  notice?: string
}

export interface FetchResponse {
  rows: Cell[][] | null
  hasMore: boolean
  limitReached: boolean
  truncatedCells?: [number, number][]
  offset: number
  durationMs: number
  error?: DBError
}

export interface ConfirmStatement {
  sql: string
  kind: Category
  reason: string
}

export interface ConfirmationDetails {
  production: boolean
  statements: ConfirmStatement[]
  environment: Environment
  connection: string
}

export interface CompletionItem {
  label: string
  kind: string
  detail?: string
  insertText: string
  sortText: string
  snippet?: boolean
}

export interface CompletionResponse {
  items: CompletionItem[] | null
  replaceLen: number
  clause: string
  metadataNote?: string
  incomplete: boolean
}

export interface Preferences {
  theme: 'system' | 'light' | 'dark'
  editorFontSize: number
  maxRows: number
  queryTimeoutSeconds: number
  historyEnabled: boolean
  historyLimit: number
  confirmDestructive: boolean
  showSystemSchemas: boolean
  hideWarnings: boolean
  reconnectOnStartup: boolean
  editorTabSize: number
  editorUseTabs: boolean
  editorWordWrap: boolean
  editorMinimap: boolean
  autocompleteOff: boolean
  uppercaseKeywords: boolean
  gridMaxColumnWidth: number
  gridThousands: boolean
  /** "<connection id>|<database>|<table>|<column>" shown as UNIX timestamps. */
  timestampColumns: string[] | null
  sqlLogLimit: number
  /** action id -> key combo ("" disables). */
  shortcuts: Record<string, string> | null
}

export interface AppInfo {
  version: string
  keyringAvailable: boolean
  configDir: string
  warnings: string[] | null
}

export interface HistoryEntry {
  id: number
  connectionId: string
  connectionName: string
  database: string
  sql: string
  executedAt: string
  durationMs: number
  success: boolean
  rowCount: number
  error?: string
}

export type TabKind = 'query' | 'host' | 'database' | 'table' | 'view' | 'routine' | 'trigger' | 'event' | 'users'

export interface PersistedTab {
  id: string
  title: string
  connectionId: string
  database: string
  sql: string
  filePath?: string
  savedQueryId?: string
  savedContent?: string | null
  kind?: TabKind
  objectName?: string
  objectType?: string
  view?: string
}

export interface WorkspaceState {
  tabs: PersistedTab[] | null
  activeTabId: string
  sidebarWidth: number
  resultsHeight: number
  logVisible?: boolean
  logHeight?: number
  helpersVisible?: boolean
  helpersWidth?: number
  openConnections?: string[] | null
}

export interface SavedQuery {
  id: string
  name: string
  sql: string
  connectionId?: string
  updatedAt?: string
}

export interface FileContent {
  path: string
  name: string
  content: string
}

// ---- DDL dialogs ----

export interface CreateDatabaseSpec {
  name: string
  charset: string
  collation: string
  ifNotExists: boolean
}

export type DefaultKind = '' | 'null' | 'value' | 'current_timestamp' | 'expression'

export interface ColumnSpec {
  name: string
  type: string
  length: string
  values: string[]
  unsigned: boolean
  nullable: boolean
  autoIncrement: boolean
  primaryKey: boolean
  unique: boolean
  defaultKind: DefaultKind
  default: string
  onUpdateCurrentTimestamp: boolean
  comment: string
}

export interface IndexSpec {
  name: string
  columns: string[]
  unique: boolean
}

export interface CreateTableSpec {
  database: string
  name: string
  columns: ColumnSpec[]
  indexes: IndexSpec[]
  engine: string
  charset: string
  collation: string
  comment: string
  ifNotExists: boolean
}

export interface CharsetInfo {
  name: string
  defaultCollation: string
  description: string
  collations: string[] | null
}

export interface DDLOptions {
  charsets: CharsetInfo[] | null
  engines: string[] | null
  defaultCharset: string
  defaultCollation: string
  defaultEngine: string
  types: string[] | null
}

// ---- catalog and server (HeidiSQL parity, stage 1) ----

export type ObjectKind = 'database' | 'table' | 'view' | 'procedure' | 'function' | 'trigger' | 'event'

export interface ObjectRef {
  kind: ObjectKind
  database: string
  name: string
}

export interface DatabaseSummary {
  name: string
  charset: string
  collation: string
  tables: number
  views: number
  size: number
  lastUpdate?: string
  system: boolean
}

export interface TableStatus {
  name: string
  type: 'table' | 'view' | 'system view' | 'sequence'
  engine?: string
  version?: number
  rowFormat?: string
  rows?: number
  avgRowLength?: number
  dataLength?: number
  maxDataLength?: number
  indexLength?: number
  dataFree?: number
  autoIncrement?: string
  createTime?: string
  updateTime?: string
  checkTime?: string
  collation?: string
  checksum?: string
  createOptions?: string
  comment?: string
}

export interface ForeignKeyInfo {
  name: string
  database: string
  table: string
  columns: string[]
  refDatabase: string
  refTable: string
  refColumns: string[]
  onUpdate: string
  onDelete: string
}

export interface CheckInfo {
  name: string
  clause: string
}

export interface TriggerInfo {
  name: string
  table: string
  timing: string
  event: string
  statement: string
  definer: string
  created?: string
  sqlMode?: string
  order: number
}

export interface EventInfo {
  name: string
  definer: string
  type: string
  executeAt?: string
  intervalValue?: string
  intervalField?: string
  starts?: string
  ends?: string
  status: string
  onCompletion: string
  body: string
  comment?: string
  lastExecuted?: string
  created?: string
}

export interface RoutineParam {
  mode: string
  name: string
  type: string
}

export interface RoutineDetail {
  name: string
  type: 'PROCEDURE' | 'FUNCTION'
  params: RoutineParam[] | null
  returns?: string
  body: string
  deterministic: boolean
  dataAccess: string
  security: string
  definer: string
  comment?: string
  created?: string
  modified?: string
}

export interface ServerVariable {
  name: string
  session?: string
  global?: string
}

export interface StatusValue {
  name: string
  value: string
}

export interface Process {
  id: number
  user: string
  host: string
  db?: string
  command: string
  time: number
  state?: string
  info?: string
  progress?: number
}

export interface AdminResult {
  sql: string
  rowsAffected?: number
  lastInsertId?: number
  durationMs: number
  error?: DBError
}

export interface SQLLogEntry {
  id: number
  at: string
  connectionId: string
  sql: string
  durationMs: number
  error?: string
}

// ---- table data grid (stage 2) ----

export interface SortSpec {
  column: string
  desc: boolean
}

export interface BrowseSpec {
  database: string
  table: string
  columns: string[]
  filter: string
  sort: SortSpec[]
  offset: number
  limit: number
  partialLength: number
  noLimit?: boolean
}

export type CellValueKind = 'value' | 'null' | 'default' | 'expression' | 'hex'

export interface CellValue {
  kind: CellValueKind
  text: string
}

export interface ColumnValue {
  column: string
  value: CellValue
}

export interface RowEdit {
  op: 'update' | 'insert' | 'delete'
  where: ColumnValue[]
  values: ColumnValue[]
}

export interface TableKey {
  kind: 'primary' | 'unique' | 'none'
  name?: string
  columns: string[] | null
}

export interface TableData {
  columns: ResultColumn[]
  partial: boolean[]
  key: TableKey
  rows: Cell[][] | null
  truncatedCells?: [number, number][]
  hasMore: boolean
  sql: string
  durationMs: number
  info: ColumnInfo[]
}

export interface RowEditResult {
  results: AdminResult[] | null
  applied: boolean
}

export interface DistinctValue {
  value: Cell
  count: number
}

export interface LookupValue {
  value: Cell
  label?: string
}

// ---- object editors (stage 3) ----

export interface ColumnDef {
  origName: string
  name: string
  type: string
  length: string
  unsigned: boolean
  zerofill: boolean
  nullable: boolean
  defaultKind: '' | 'null' | 'value' | 'expression'
  default: string
  autoIncrement: boolean
  onUpdate: string
  comment: string
  charset: string
  collation: string
  generated: string
  generatedKind: string
  invisible: boolean
  srid: string
}

export interface IndexPart {
  column: string
  length: number
  desc: boolean
  expression: string
}

export interface IndexDef {
  origName: string
  name: string
  kind: 'PRIMARY' | 'KEY' | 'UNIQUE' | 'FULLTEXT' | 'SPATIAL'
  parts: IndexPart[] | null
  algorithm: string
  comment: string
  invisible: boolean
}

export interface ForeignKeyDef {
  origName: string
  name: string
  columns: string[] | null
  refDatabase: string
  refTable: string
  refColumns: string[] | null
  onUpdate: string
  onDelete: string
}

export interface CheckDef {
  origName: string
  name: string
  clause: string
}

export interface TableOptions {
  engine: string
  collation: string
  comment: string
  rowFormat: string
  autoIncrement: string
  avgRowLength: string
  maxRows: string
  checksum: boolean
  partition: string
  convertData: boolean
}

export interface TableDef {
  database: string
  name: string
  origName: string
  columns: ColumnDef[] | null
  indexes: IndexDef[] | null
  foreignKeys: ForeignKeyDef[] | null
  checks: CheckDef[] | null
  options: TableOptions
  mariadb: boolean
}

export interface ViewDef {
  database: string
  name: string
  origName: string
  definer: string
  security: string
  algorithm: string
  checkOption: string
  select: string
}

export interface RoutineDef {
  database: string
  name: string
  origName: string
  type: 'PROCEDURE' | 'FUNCTION'
  origType: string
  params: RoutineParam[] | null
  returns: string
  body: string
  deterministic: boolean
  dataAccess: string
  security: string
  definer: string
  comment: string
}

export interface TriggerDef {
  database: string
  name: string
  origName: string
  table: string
  timing: string
  event: string
  body: string
  definer: string
}

export interface EventDef {
  database: string
  name: string
  origName: string
  definer: string
  recurring: boolean
  executeAt: string
  intervalValue: string
  intervalField: string
  starts: string
  ends: string
  preserve: boolean
  status: string
  comment: string
  body: string
}

// ---- query helpers (stage 4) ----

export interface EditableInfo {
  database: string
  table: string
  key: TableKey
  columns: string[] | null
  info: ColumnInfo[] | null
}

export interface HelpItem {
  name: string
  isCategory: boolean
}

export interface HelpResult {
  kind: 'topic' | 'list' | 'none'
  name: string
  description?: string
  example?: string
  items?: HelpItem[] | null
}

export interface Snippet {
  name: string
  size: number
  modified: string
}

export interface Vocabulary {
  keywords: string[] | null
  functions: { name: string; signature: string }[] | null
}

// ---- export / import / tasks (stage 5) ----

export interface ExportColumn {
  name: string
  type: string
  numeric: boolean
  binary: boolean
  key: boolean
  autoIncrement: boolean
}

export type ExportFormat = 'csv' | 'excel' | 'tsv' | 'json' | 'jsonl' | 'xml' | 'html' | 'markdown' | 'latex' | 'textile' | 'jira' | 'php'
  | 'sql-insert' | 'sql-insert-ignore' | 'sql-replace' | 'sql-delete-insert' | 'sql-update'

export interface ExportOptions {
  format: ExportFormat
  separator: string
  encloser: string
  lineTerminator: string
  nullText: string
  header: boolean
  removeLineBreaks: boolean
  table: string
  query: string
  skipAutoIncrement: boolean
  rowsPerInsert: number
}

export interface Task {
  id: string
  kind: string
  title: string
  status: 'running' | 'done' | 'error' | 'cancelled'
  done: number
  total: number
  unit: string
  message?: string
  error?: string
  warnings: number
  startedAt: string
  finishedAt?: string
  result?: string
  path?: string
  /** Structured result (maintenance messages, search hits…). */
  data?: unknown
}

export interface DumpSelection {
  database: string
  tables: string[]
  routines: boolean
  events: boolean
  triggers: boolean
  allObjects: boolean
}

export interface DumpOptions {
  selections: DumpSelection[]
  dropDatabase: boolean
  createDatabase: boolean
  dropObjects: boolean
  createObjects: boolean
  data: 'none' | 'insert' | 'insert-ignore' | 'replace' | 'delete-insert'
  maxInsertKB: number
  comments: boolean
  wrapTransactions: boolean
  removeAutoIncrement: boolean
  removeDefiner: boolean
  targetDatabase: string
}

export interface DumpTarget {
  kind: 'file' | 'gzip' | 'directory' | 'database'
  connectionId: string
  database: string
  continueOnError: boolean
}

export interface FileInfo {
  path: string
  name: string
  size: number
}

export interface CsvFormat {
  separator: string
  encloser: string
  escape: string
  encoding: string
  skipLines: number
  nullMarker: string
  emptyNull: boolean
}

export interface ColumnGuess {
  name: string
  type: string
  length: string
  unsigned: boolean
  nullable: boolean
}

export interface CsvPreview extends FileInfo {
  detected: {
    format: CsvFormat
    header: string[] | null
    sample: (string | null)[][] | null
    columns: ColumnGuess[] | null
  }
}

export interface CsvTarget {
  database: string
  table: string
  columns: string[]
  numeric: boolean[]
  mode: 'insert' | 'ignore' | 'replace'
  batchRows: number
  atomic: boolean
  thousands: string
  decimal: string
}

export interface CsvImport {
  path: string
  format: CsvFormat
  target: CsvTarget
  truncate: boolean
  serverSide: boolean
}

export interface CellFile {
  name: string
  size: number
  hex: string
}

// ---- tools (etapa 6) ----

export interface TableRef {
  database: string
  table: string
}

export interface MaintenanceRow {
  table: string
  op: string
  msgType: string
  message: string
}

export type SearchMatch = 'contains' | 'exact' | 'starts' | 'ends' | 'regex'
export type SearchTypes = 'all' | 'text' | 'numeric' | 'date' | 'binary'

export interface SearchSpec {
  text: string
  match: SearchMatch
  caseSensitive: boolean
  types: SearchTypes
}

export interface SearchHit {
  database: string
  table: string
  rows: number
  total: number
  filter: string
}

export interface BulkSpec {
  moveTo: string
  engine: string
  convertCharset: string
  convertCollation: string
  defaultCollation: string
  resetAutoIncrement: boolean
}

export interface CopyTableSpec {
  database: string
  table: string
  targetDatabase: string
  targetTable: string
  columns: string[]
  indexes: boolean
  foreignKeys: boolean
  checks: boolean
  data: boolean
  filter: string
  dropExisting: boolean
}

export interface GenerateSpec {
  database: string
  table: string
  rows: number
  nullPercent: number
}

export interface Account {
  user: string
  host: string
  isRole: boolean
  locked: boolean
  plugin: string
  emptyPassword: boolean
}

export type GrantLevel = 'global' | 'database' | 'table' | 'routine'

export interface Grant {
  level: GrantLevel
  database: string
  name: string
  routineType: string
  privileges: string[]
  columns: Record<string, string[]>
  grantOption: boolean
}

export interface RoleGrant {
  role: string
  adminOption: boolean
}

export interface UserDef {
  user: string
  host: string
  origUser: string
  origHost: string
  isRole: boolean
  password: string
  plugin: string
  require: '' | 'NONE' | 'SSL' | 'X509' | 'SPECIFIED'
  cipher: string
  issuer: string
  subject: string
  maxQueries: number
  maxUpdates: number
  maxConnections: number
  maxUserConnections: number
  locked: boolean
  passwordExpire: '' | 'NOW' | 'NEVER' | 'INTERVAL'
  expireDays: number
  grants: Grant[]
  roles: RoleGrant[]
  defaultRoles: string[]
}

export interface PrivilegeCatalog {
  global: string[]
  database: string[]
  table: string[]
  column: string[]
  routine: string[]
  plugins: string[]
  roles: boolean
}

export type DiffStatus = 'create' | 'drop' | 'alter' | 'same'

export interface SchemaDiffItem {
  kind: 'table' | 'view' | 'procedure' | 'function' | 'trigger' | 'event'
  name: string
  status: DiffStatus
  statements: string[]
  sourceSQL: string
  targetSQL: string
}

export interface SchemaDiff {
  items: SchemaDiffItem[]
  warnings: string[] | null
}

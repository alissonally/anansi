// Typed facade over the Wails bindings. Every call rejects with AppError.
import * as App from '../../wailsjs/go/api/App'
import { toAppError } from './errors'
import type {
  AppInfo, ColumnInfo, CompletionResponse, DatabaseInfo, ExecuteRequest, ExecuteResponse, FetchResponse,
  FileContent, HistoryEntry, IndexInfo, Preferences, Profile, ProfileView, RoutineInfo, SaveResult, SavedQuery,
  ServerInfo, SessionState, TableInfo, WorkspaceState, Cell, CreateDatabaseSpec, CreateTableSpec, DDLOptions,
  AdminResult, CheckInfo, DatabaseSummary, EventInfo, ForeignKeyInfo, ObjectKind, ObjectRef, Process, RoutineDetail,
  ServerVariable, SQLLogEntry, StatusValue, TableStatus, TriggerInfo,
  BrowseSpec, ColumnValue, DistinctValue, LookupValue, RowEdit, RowEditResult, TableData,
  EventDef, RoutineDef, TableDef, TriggerDef, ViewDef,
  EditableInfo, HelpResult, Snippet, Vocabulary,
  ImportResult, Account, BulkSpec, CopyTableSpec, GenerateSpec, PrivilegeCatalog, SchemaDiff, SearchSpec, TableRef, UserDef,
  CellFile, CsvImport, CsvFormat, CsvPreview, DumpOptions, DumpTarget, ExportColumn, ExportOptions, FileInfo, Task,
} from './types'

// The generated bindings use classes with converter methods; the payloads we
// send are plain objects with the same shape.
type AnyFn = (...args: never[]) => Promise<unknown>
const bindings = App as unknown as Record<string, AnyFn>

async function call<T>(name: string, ...args: unknown[]): Promise<T> {
  const fn = bindings[name]
  if (!fn) throw toAppError(`função do backend indisponível: ${name}`)
  try {
    return (await (fn as (...a: unknown[]) => Promise<unknown>)(...args)) as T
  } catch (e) {
    throw toAppError(e)
  }
}

export const backend = {
  appInfo: () => call<AppInfo>('GetAppInfo'),
  getPreferences: () => call<Preferences>('GetPreferences'),
  savePreferences: (p: Preferences) => call<Preferences>('SavePreferences', p),

  listConnections: async () => (await call<ProfileView[] | null>('ListConnections')) ?? [],
  saveConnection: (profile: Profile, password: string | null, sshSecret: string | null = null) => call<SaveResult>('SaveConnection', { profile, password, sshSecret }),
  duplicateConnection: (id: string) => call<ProfileView>('DuplicateConnection', id),
  deleteConnection: (id: string) => call<void>('DeleteConnection', id),
  reorderConnections: (ids: string[]) => call<void>('ReorderConnections', ids),
  testConnection: (profile: Profile, password: string | null, sshSecret: string | null = null) => call<ServerInfo>('TestConnection', { profile, password, sshSecret }),
  setSshSecret: (id: string, secret: string, remember: boolean) => call<void>('SetSSHSecret', id, secret, remember),
  trustSshHost: (host: string, fingerprint: string) => call<void>('TrustSSHHost', host, fingerprint),
  knownSshHosts: async () => (await call<string[] | null>('KnownSSHHosts')) ?? [],
  pickKeyFile: () => call<string>('PickKeyFile'),
  exportConnections: (ids: string[]) => call<string>('ExportConnections', ids),
  importConnections: () => call<ImportResult | null>('ImportConnections'),
  connect: (id: string, password: string | null, remember: boolean) => call<SessionState>('Connect', id, password, remember),
  disconnect: (id: string) => call<void>('Disconnect', id),
  sessionStates: async () => (await call<SessionState[] | null>('SessionStates')) ?? [],

  listDatabases: async (id: string) => (await call<DatabaseInfo[] | null>('ListDatabases', id)) ?? [],
  listTables: async (id: string, db: string) => (await call<TableInfo[] | null>('ListTables', id, db)) ?? [],
  listColumns: async (id: string, db: string, t: string) => (await call<ColumnInfo[] | null>('ListColumns', id, db, t)) ?? [],
  listIndexes: async (id: string, db: string, t: string) => (await call<IndexInfo[] | null>('ListIndexes', id, db, t)) ?? [],
  listRoutines: async (id: string, db: string) => (await call<RoutineInfo[] | null>('ListRoutines', id, db)) ?? [],
  refreshMetadata: (id: string, db: string) => call<void>('RefreshMetadata', id, db),
  warmMetadata: (id: string, db: string) => call<void>('WarmMetadata', id, db),
  complete: (id: string, sql: string, offset: number, database: string) =>
    call<CompletionResponse>('Complete', id, { sql, offset, database }),

  execute: (req: ExecuteRequest) => call<ExecuteResponse>('Execute', req),
  fetchMore: (tabId: string, cursorId: string) => call<FetchResponse>('FetchMore', { tabId, cursorId }),
  cancel: (tabId: string) => call<void>('CancelExecution', tabId),
  closeResult: (tabId: string) => call<void>('CloseResult', tabId),
  releaseTab: (tabId: string, force: boolean) => call<void>('ReleaseTab', tabId, force),

  listHistory: async (connectionId: string, search: string, limit = 200) =>
    (await call<HistoryEntry[] | null>('ListHistory', { connectionId, search, limit })) ?? [],
  clearHistory: (connectionId: string) => call<void>('ClearHistory', connectionId),

  loadWorkspace: () => call<WorkspaceState>('LoadWorkspace'),
  saveWorkspace: (s: WorkspaceState) => call<void>('SaveWorkspace', s),
  listSavedQueries: async () => (await call<SavedQuery[] | null>('ListSavedQueries')) ?? [],
  saveQuery: (q: SavedQuery) => call<SavedQuery>('SaveQuery', q),
  deleteSavedQuery: (id: string) => call<void>('DeleteSavedQuery', id),
  openSqlFile: () => call<FileContent | null>('OpenSQLFile'),
  saveSqlFile: (path: string, content: string) => call<void>('SaveSQLFile', path, content),
  saveSqlFileAs: (name: string, content: string) => call<FileContent | null>('SaveSQLFileAs', name, content),
  ddlOptions: (id: string) => call<DDLOptions>('DDLOptions', id),
  previewCreateDatabase: (id: string, spec: CreateDatabaseSpec) => call<string>('PreviewCreateDatabase', id, spec),
  createDatabase: (id: string, spec: CreateDatabaseSpec, confirmed: boolean) => call<void>('CreateDatabase', id, spec, confirmed),
  previewCreateTable: (id: string, spec: CreateTableSpec) => call<string>('PreviewCreateTable', id, spec),
  createTable: (id: string, spec: CreateTableSpec, confirmed: boolean) => call<void>('CreateTable', id, spec, confirmed),

  // catalog
  databaseSummaries: async (id: string) => (await call<DatabaseSummary[] | null>('DatabaseSummaries', id)) ?? [],
  tableStatus: async (id: string, db: string) => (await call<TableStatus[] | null>('TableStatus', id, db)) ?? [],
  foreignKeys: async (id: string, db: string, t: string) => (await call<ForeignKeyInfo[] | null>('ForeignKeys', id, db, t)) ?? [],
  referencingKeys: async (id: string, db: string, t: string) => (await call<ForeignKeyInfo[] | null>('ReferencingKeys', id, db, t)) ?? [],
  checkConstraints: async (id: string, db: string, t: string) => (await call<CheckInfo[] | null>('CheckConstraints', id, db, t)) ?? [],
  triggers: async (id: string, db: string) => (await call<TriggerInfo[] | null>('Triggers', id, db)) ?? [],
  events: async (id: string, db: string) => (await call<EventInfo[] | null>('Events', id, db)) ?? [],
  routineDetails: async (id: string, db: string) => (await call<RoutineDetail[] | null>('RoutineDetails', id, db)) ?? [],
  createStatement: (id: string, db: string, kind: ObjectKind, name: string) => call<string>('CreateStatement', id, db, kind, name),

  // server
  serverVariables: async (id: string) => (await call<ServerVariable[] | null>('ServerVariables', id)) ?? [],
  serverStatus: async (id: string) => (await call<StatusValue[] | null>('ServerStatus', id)) ?? [],
  processList: async (id: string) => (await call<Process[] | null>('ProcessList', id)) ?? [],

  // object actions (confirmed=false may reject with confirmation_required)
  dropObjects: async (id: string, objects: ObjectRef[], confirmed: boolean) => (await call<AdminResult[] | null>('DropObjects', id, objects, confirmed)) ?? [],
  truncateTables: async (id: string, db: string, tables: string[], noFKChecks: boolean, confirmed: boolean) => (await call<AdminResult[] | null>('TruncateTables', id, db, tables, noFKChecks, confirmed)) ?? [],
  renameObject: (id: string, obj: ObjectRef, toDatabase: string, toName: string, confirmed: boolean) => call<void>('RenameObject', id, obj, toDatabase, toName, confirmed),
  alterDatabase: (id: string, db: string, charset: string, collation: string, confirmed: boolean) => call<void>('AlterDatabase', id, db, charset, collation, confirmed),
  setVariable: (id: string, global: boolean, name: string, value: string, valueKind: string, confirmed: boolean) => call<void>('SetVariable', id, global, name, value, valueKind, confirmed),
  killProcesses: async (id: string, ids: number[], queryOnly: boolean, confirmed: boolean) => (await call<AdminResult[] | null>('KillProcesses', id, ids, queryOnly, confirmed)) ?? [],

  // table data grid
  browseTable: (token: string, id: string, spec: BrowseSpec) => call<TableData>('BrowseTable', token, id, spec),
  cancelBrowse: (token: string) => call<void>('CancelBrowse', token),
  countTableRows: (token: string, id: string, db: string, table: string, filter: string) => call<number>('CountTableRows', token, id, db, table, filter),
  fetchRowValues: async (id: string, db: string, table: string, where: ColumnValue[], columns: string[]) => (await call<Cell[] | null>('FetchRowValues', id, db, table, where, columns)) ?? [],
  previewRowEdits: async (id: string, db: string, table: string, edits: RowEdit[]) => (await call<string[] | null>('PreviewRowEdits', id, db, table, edits)) ?? [],
  applyRowEdits: (id: string, db: string, table: string, edits: RowEdit[], confirmed: boolean) => call<RowEditResult>('ApplyRowEdits', id, db, table, edits, confirmed),
  distinctValues: async (id: string, db: string, table: string, column: string, filter: string) => (await call<DistinctValue[] | null>('DistinctValues', id, db, table, column, filter)) ?? [],
  lookupValues: async (id: string, db: string, table: string, column: string, search: string) => (await call<LookupValue[] | null>('LookupValues', id, db, table, column, search)) ?? [],

  // object editors (save* may reject with confirmation_required)
  loadTableDef: (id: string, db: string, table: string) => call<TableDef>('LoadTableDef', id, db, table),
  previewTableDef: async (id: string, def: TableDef) => (await call<string[] | null>('PreviewTableDef', id, def)) ?? [],
  saveTableDef: async (id: string, def: TableDef, confirmed: boolean) => (await call<AdminResult[] | null>('SaveTableDef', id, def, confirmed)) ?? [],
  loadViewDef: (id: string, db: string, name: string) => call<ViewDef>('LoadViewDef', id, db, name),
  previewViewDef: async (id: string, def: ViewDef) => (await call<string[] | null>('PreviewViewDef', id, def)) ?? [],
  saveViewDef: async (id: string, def: ViewDef, confirmed: boolean) => (await call<AdminResult[] | null>('SaveViewDef', id, def, confirmed)) ?? [],
  loadRoutineDef: (id: string, db: string, type: string, name: string) => call<RoutineDef>('LoadRoutineDef', id, db, type, name),
  previewRoutineDef: (id: string, def: RoutineDef) => call<string>('PreviewRoutineDef', id, def),
  saveRoutineDef: async (id: string, def: RoutineDef, confirmed: boolean) => (await call<AdminResult[] | null>('SaveRoutineDef', id, def, confirmed)) ?? [],
  loadTriggerDef: (id: string, db: string, name: string) => call<TriggerDef>('LoadTriggerDef', id, db, name),
  previewTriggerDef: (id: string, def: TriggerDef) => call<string>('PreviewTriggerDef', id, def),
  saveTriggerDef: async (id: string, def: TriggerDef, confirmed: boolean) => (await call<AdminResult[] | null>('SaveTriggerDef', id, def, confirmed)) ?? [],
  loadEventDef: (id: string, db: string, name: string) => call<EventDef>('LoadEventDef', id, db, name),
  previewEventDef: (id: string, def: EventDef) => call<string>('PreviewEventDef', id, def),
  saveEventDef: async (id: string, def: EventDef, confirmed: boolean) => (await call<AdminResult[] | null>('SaveEventDef', id, def, confirmed)) ?? [],
  definers: async (id: string) => (await call<string[] | null>('Definers', id)) ?? [],

  // query helpers
  editableResult: (id: string, sql: string, db: string, columns: string[]) => call<EditableInfo | null>('EditableResult', id, sql, db, columns),
  sqlHelp: (id: string, topic: string) => call<HelpResult>('SQLHelp', id, topic),
  sqlVocabulary: () => call<Vocabulary>('SQLVocabulary'),
  listSnippets: async () => (await call<Snippet[] | null>('ListSnippets')) ?? [],
  readSnippet: (name: string) => call<string>('ReadSnippet', name),
  saveSnippet: (name: string, content: string) => call<void>('SaveSnippet', name, content),
  renameSnippet: (from: string, to: string) => call<void>('RenameSnippet', from, to),
  deleteSnippet: (name: string) => call<void>('DeleteSnippet', name),
  openSnippetsFolder: () => call<void>('OpenSnippetsFolder'),

  // export / import / tasks (start* return a task id, "" when the dialog was cancelled)
  listTasks: async () => (await call<Task[] | null>('ListTasks')) ?? [],
  cancelTask: (id: string) => call<void>('CancelTask', id),
  dismissTask: (id: string) => call<void>('DismissTask', id),
  openTaskFolder: (id: string) => call<void>('OpenTaskFolder', id),
  formatRows: (cols: ExportColumn[], rows: Cell[][], opts: ExportOptions) => call<string>('FormatRows', cols, rows, opts),
  exportRowsToFile: (cols: ExportColumn[], rows: Cell[][], opts: ExportOptions, name: string) => call<string>('ExportRowsToFile', cols, rows, opts, name),
  exportQueryToFile: (id: string, db: string, sql: string, opts: ExportOptions, name: string) => call<string>('ExportQueryToFile', id, db, sql, opts, name),
  exportTableToFile: (id: string, spec: BrowseSpec, opts: ExportOptions) => call<string>('ExportTableToFile', id, spec, opts),
  startDump: (id: string, opts: DumpOptions, target: DumpTarget, confirmed: boolean) => call<string>('StartDump', id, opts, target, confirmed),
  pickSqlFile: () => call<FileInfo | null>('PickSQLFile'),
  runSqlFile: (id: string, db: string, path: string, stopOnError: boolean, confirmed: boolean) => call<string>('RunSQLFile', id, db, path, stopOnError, confirmed),
  pickCsvFile: () => call<CsvPreview | null>('PickCSVFile'),
  previewCsv: (path: string, format: CsvFormat | null) => call<CsvPreview>('PreviewCSV', path, format),
  importCsv: (id: string, req: CsvImport, confirmed: boolean) => call<string>('ImportCSV', id, req, confirmed),
  loadFileForCell: () => call<CellFile | null>('LoadFileForCell'),
  saveCellToFile: (name: string, data: string, isHex: boolean) => call<string>('SaveCellToFile', name, data, isHex),

  // tools (start* return a task id)
  startMaintenance: (id: string, op: string, options: string[], tables: TableRef[], confirmed: boolean) => call<string>('StartMaintenance', id, op, options, tables, confirmed),
  startSearch: (id: string, databases: string[], tables: TableRef[], spec: SearchSpec) => call<string>('StartSearch', id, databases, tables, spec),
  previewBulkEdit: async (id: string, tables: TableRef[], spec: BulkSpec) => (await call<string[] | null>('PreviewBulkEdit', id, tables, spec)) ?? [],
  applyBulkEdit: async (id: string, tables: TableRef[], spec: BulkSpec, confirmed: boolean) => (await call<AdminResult[] | null>('ApplyBulkEdit', id, tables, spec, confirmed)) ?? [],
  previewCopyTable: async (id: string, spec: CopyTableSpec) => (await call<string[] | null>('PreviewCopyTable', id, spec)) ?? [],
  copyTable: async (id: string, spec: CopyTableSpec, confirmed: boolean) => (await call<AdminResult[] | null>('CopyTable', id, spec, confirmed)) ?? [],
  startGenerateData: (id: string, spec: GenerateSpec, confirmed: boolean) => call<string>('StartGenerateData', id, spec, confirmed),
  flush: (id: string, what: string, confirmed: boolean) => call<void>('Flush', id, what, confirmed),
  listAccounts: async (id: string) => (await call<Account[] | null>('ListAccounts', id)) ?? [],
  privilegeCatalog: (id: string) => call<PrivilegeCatalog>('PrivilegeCatalog', id),
  loadAccount: (id: string, user: string, host: string, isRole: boolean) => call<UserDef>('LoadAccount', id, user, host, isRole),
  previewAccount: async (id: string, u: UserDef) => (await call<string[] | null>('PreviewAccount', id, u)) ?? [],
  saveAccount: async (id: string, u: UserDef, confirmed: boolean) => (await call<AdminResult[] | null>('SaveAccount', id, u, confirmed)) ?? [],
  dropAccount: (id: string, user: string, host: string, isRole: boolean, confirmed: boolean) => call<void>('DropAccount', id, user, host, isRole, confirmed),
  changeOwnPassword: (id: string, pw: string) => call<void>('ChangeOwnPassword', id, pw),
  randomPassword: (len: number) => call<string>('RandomPassword', len),
  compareSchemas: (srcId: string, srcDb: string, dstId: string, dstDb: string) => call<SchemaDiff>('CompareSchemas', srcId, srcDb, dstId, dstDb),
  applySchemaDiff: async (dstId: string, dstDb: string, stmts: string[], continueOnError: boolean, confirmed: boolean) =>
    (await call<AdminResult[] | null>('ApplySchemaDiff', dstId, dstDb, stmts, continueOnError, confirmed)) ?? [],

  // SQL log
  listSqlLog: async (since: number, limit: number) => (await call<SQLLogEntry[] | null>('ListSQLLog', since, limit)) ?? [],
  clearSqlLog: () => call<void>('ClearSQLLog'),
}

export type Backend = typeof backend

export namespace api {
	
	export class AppInfo {
	    version: string;
	    keyringAvailable: boolean;
	    configDir: string;
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.keyringAvailable = source["keyringAvailable"];
	        this.configDir = source["configDir"];
	        this.warnings = source["warnings"];
	    }
	}
	export class CSVImport {
	    path: string;
	    format: csvimport.Format;
	    target: csvimport.Target;
	    truncate: boolean;
	    serverSide: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CSVImport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.format = this.convertValues(source["format"], csvimport.Format);
	        this.target = this.convertValues(source["target"], csvimport.Target);
	        this.truncate = source["truncate"];
	        this.serverSide = source["serverSide"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CSVPreview {
	    path: string;
	    name: string;
	    size: number;
	    detected: csvimport.Detected;
	
	    static createFrom(source: any = {}) {
	        return new CSVPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.size = source["size"];
	        this.detected = this.convertValues(source["detected"], csvimport.Detected);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CellFile {
	    name: string;
	    size: number;
	    hex: string;
	
	    static createFrom(source: any = {}) {
	        return new CellFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.size = source["size"];
	        this.hex = source["hex"];
	    }
	}
	export class CopyTableSpec {
	    database: string;
	    table: string;
	    targetDatabase: string;
	    targetTable: string;
	    columns: string[];
	    indexes: boolean;
	    foreignKeys: boolean;
	    checks: boolean;
	    data: boolean;
	    filter: string;
	    dropExisting: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CopyTableSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.database = source["database"];
	        this.table = source["table"];
	        this.targetDatabase = source["targetDatabase"];
	        this.targetTable = source["targetTable"];
	        this.columns = source["columns"];
	        this.indexes = source["indexes"];
	        this.foreignKeys = source["foreignKeys"];
	        this.checks = source["checks"];
	        this.data = source["data"];
	        this.filter = source["filter"];
	        this.dropExisting = source["dropExisting"];
	    }
	}
	export class DistinctValue {
	    value: any;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new DistinctValue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value = source["value"];
	        this.count = source["count"];
	    }
	}
	export class DumpTarget {
	    kind: string;
	    connectionId: string;
	    database: string;
	    continueOnError: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DumpTarget(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.connectionId = source["connectionId"];
	        this.database = source["database"];
	        this.continueOnError = source["continueOnError"];
	    }
	}
	export class EditableInfo {
	    database: string;
	    table: string;
	    key: database.TableKey;
	    columns: string[];
	    info: database.ColumnInfo[];
	
	    static createFrom(source: any = {}) {
	        return new EditableInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.database = source["database"];
	        this.table = source["table"];
	        this.key = this.convertValues(source["key"], database.TableKey);
	        this.columns = source["columns"];
	        this.info = this.convertValues(source["info"], database.ColumnInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FileContent {
	    path: string;
	    name: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new FileContent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.content = source["content"];
	    }
	}
	export class FileInfo {
	    path: string;
	    name: string;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new FileInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.size = source["size"];
	    }
	}
	export class GenerateSpec {
	    database: string;
	    table: string;
	    rows: number;
	    nullPercent: number;
	
	    static createFrom(source: any = {}) {
	        return new GenerateSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.database = source["database"];
	        this.table = source["table"];
	        this.rows = source["rows"];
	        this.nullPercent = source["nullPercent"];
	    }
	}
	export class HelpItem {
	    name: string;
	    isCategory: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HelpItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.isCategory = source["isCategory"];
	    }
	}
	export class HelpResult {
	    kind: string;
	    name: string;
	    description?: string;
	    example?: string;
	    items?: HelpItem[];
	
	    static createFrom(source: any = {}) {
	        return new HelpResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.example = source["example"];
	        this.items = this.convertValues(source["items"], HelpItem);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ImportResult {
	    imported: number;
	    problems: string[];
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.imported = source["imported"];
	        this.problems = source["problems"];
	    }
	}
	export class LookupValue {
	    value: any;
	    label?: string;
	
	    static createFrom(source: any = {}) {
	        return new LookupValue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value = source["value"];
	        this.label = source["label"];
	    }
	}
	export class ObjectRef {
	    kind: string;
	    database: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new ObjectRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.database = source["database"];
	        this.name = source["name"];
	    }
	}
	export class RowEditResult {
	    results: query.AdminResult[];
	    applied: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RowEditResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.results = this.convertValues(source["results"], query.AdminResult);
	        this.applied = source["applied"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SQLLogEntry {
	    id: number;
	    // Go type: time
	    at: any;
	    connectionId: string;
	    sql: string;
	    durationMs: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new SQLLogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.at = this.convertValues(source["at"], null);
	        this.connectionId = source["connectionId"];
	        this.sql = source["sql"];
	        this.durationMs = source["durationMs"];
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SchemaDiffItem {
	    kind: string;
	    name: string;
	    status: string;
	    statements: string[];
	    sourceSQL: string;
	    targetSQL: string;
	
	    static createFrom(source: any = {}) {
	        return new SchemaDiffItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.statements = source["statements"];
	        this.sourceSQL = source["sourceSQL"];
	        this.targetSQL = source["targetSQL"];
	    }
	}
	export class SchemaDiff {
	    items: SchemaDiffItem[];
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new SchemaDiff(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], SchemaDiffItem);
	        this.warnings = source["warnings"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class Snippet {
	    name: string;
	    size: number;
	    // Go type: time
	    modified: any;
	
	    static createFrom(source: any = {}) {
	        return new Snippet(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.size = source["size"];
	        this.modified = this.convertValues(source["modified"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TableData {
	    columns: query.Column[];
	    partial: boolean[];
	    key: database.TableKey;
	    rows: any[][];
	    truncatedCells?: number[][];
	    hasMore: boolean;
	    sql: string;
	    durationMs: number;
	    info: database.ColumnInfo[];
	
	    static createFrom(source: any = {}) {
	        return new TableData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.columns = this.convertValues(source["columns"], query.Column);
	        this.partial = source["partial"];
	        this.key = this.convertValues(source["key"], database.TableKey);
	        this.rows = source["rows"];
	        this.truncatedCells = source["truncatedCells"];
	        this.hasMore = source["hasMore"];
	        this.sql = source["sql"];
	        this.durationMs = source["durationMs"];
	        this.info = this.convertValues(source["info"], database.ColumnInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TableRef {
	    database: string;
	    table: string;
	
	    static createFrom(source: any = {}) {
	        return new TableRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.database = source["database"];
	        this.table = source["table"];
	    }
	}
	export class Vocabulary {
	    keywords: string[];
	    functions: autocomplete.Function[];
	
	    static createFrom(source: any = {}) {
	        return new Vocabulary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.keywords = source["keywords"];
	        this.functions = this.convertValues(source["functions"], autocomplete.Function);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace autocomplete {
	
	export class Function {
	    name: string;
	    signature: string;
	
	    static createFrom(source: any = {}) {
	        return new Function(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.signature = source["signature"];
	    }
	}
	export class Item {
	    label: string;
	    kind: string;
	    detail?: string;
	    insertText: string;
	    sortText: string;
	    snippet?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Item(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.kind = source["kind"];
	        this.detail = source["detail"];
	        this.insertText = source["insertText"];
	        this.sortText = source["sortText"];
	        this.snippet = source["snippet"];
	    }
	}
	export class Request {
	    sql: string;
	    offset: number;
	    database: string;
	
	    static createFrom(source: any = {}) {
	        return new Request(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sql = source["sql"];
	        this.offset = source["offset"];
	        this.database = source["database"];
	    }
	}
	export class Response {
	    items: Item[];
	    replaceLen: number;
	    clause: string;
	    metadataNote?: string;
	    incomplete: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Response(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], Item);
	        this.replaceLen = source["replaceLen"];
	        this.clause = source["clause"];
	        this.metadataNote = source["metadataNote"];
	        this.incomplete = source["incomplete"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace config {
	
	export class Preferences {
	    theme: string;
	    editorFontSize: number;
	    maxRows: number;
	    queryTimeoutSeconds: number;
	    historyEnabled: boolean;
	    historyLimit: number;
	    confirmDestructive: boolean;
	    showSystemSchemas: boolean;
	    hideWarnings: boolean;
	    reconnectOnStartup: boolean;
	    editorTabSize: number;
	    editorUseTabs: boolean;
	    editorWordWrap: boolean;
	    editorMinimap: boolean;
	    autocompleteOff: boolean;
	    uppercaseKeywords: boolean;
	    gridMaxColumnWidth: number;
	    gridThousands: boolean;
	    timestampColumns: string[];
	    sqlLogLimit: number;
	    shortcuts: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new Preferences(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.theme = source["theme"];
	        this.editorFontSize = source["editorFontSize"];
	        this.maxRows = source["maxRows"];
	        this.queryTimeoutSeconds = source["queryTimeoutSeconds"];
	        this.historyEnabled = source["historyEnabled"];
	        this.historyLimit = source["historyLimit"];
	        this.confirmDestructive = source["confirmDestructive"];
	        this.showSystemSchemas = source["showSystemSchemas"];
	        this.hideWarnings = source["hideWarnings"];
	        this.reconnectOnStartup = source["reconnectOnStartup"];
	        this.editorTabSize = source["editorTabSize"];
	        this.editorUseTabs = source["editorUseTabs"];
	        this.editorWordWrap = source["editorWordWrap"];
	        this.editorMinimap = source["editorMinimap"];
	        this.autocompleteOff = source["autocompleteOff"];
	        this.uppercaseKeywords = source["uppercaseKeywords"];
	        this.gridMaxColumnWidth = source["gridMaxColumnWidth"];
	        this.gridThousands = source["gridThousands"];
	        this.timestampColumns = source["timestampColumns"];
	        this.sqlLogLimit = source["sqlLogLimit"];
	        this.shortcuts = source["shortcuts"];
	    }
	}

}

export namespace connections {
	
	export class ConnStats {
	    connects: number;
	    failures: number;
	    // Go type: time
	    lastConnectedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new ConnStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connects = source["connects"];
	        this.failures = source["failures"];
	        this.lastConnectedAt = this.convertValues(source["lastConnectedAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SSHOptions {
	    enabled: boolean;
	    host: string;
	    port: number;
	    user: string;
	    auth: string;
	    keyFile: string;
	    saveSecret: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SSHOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.auth = source["auth"];
	        this.keyFile = source["keyFile"];
	        this.saveSecret = source["saveSecret"];
	    }
	}
	export class Profile {
	    id: string;
	    name: string;
	    kind: string;
	    host: string;
	    port: number;
	    user: string;
	    database: string;
	    tls: database.TLSOptions;
	    params: Record<string, string>;
	    allowCleartextPasswords: boolean;
	    environment: string;
	    color: string;
	    readOnly: boolean;
	    savePassword: boolean;
	    connectTimeoutSeconds: number;
	    socket: string;
	    ssh: SSHOptions;
	    initSQL: string;
	    keepAliveSeconds: number;
	    folder: string;
	    comment: string;
	    databaseFilter: string;
	    stats: ConnStats;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Profile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.database = source["database"];
	        this.tls = this.convertValues(source["tls"], database.TLSOptions);
	        this.params = source["params"];
	        this.allowCleartextPasswords = source["allowCleartextPasswords"];
	        this.environment = source["environment"];
	        this.color = source["color"];
	        this.readOnly = source["readOnly"];
	        this.savePassword = source["savePassword"];
	        this.connectTimeoutSeconds = source["connectTimeoutSeconds"];
	        this.socket = source["socket"];
	        this.ssh = this.convertValues(source["ssh"], SSHOptions);
	        this.initSQL = source["initSQL"];
	        this.keepAliveSeconds = source["keepAliveSeconds"];
	        this.folder = source["folder"];
	        this.comment = source["comment"];
	        this.databaseFilter = source["databaseFilter"];
	        this.stats = this.convertValues(source["stats"], ConnStats);
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ProfileView {
	    id: string;
	    name: string;
	    kind: string;
	    host: string;
	    port: number;
	    user: string;
	    database: string;
	    tls: database.TLSOptions;
	    params: Record<string, string>;
	    allowCleartextPasswords: boolean;
	    environment: string;
	    color: string;
	    readOnly: boolean;
	    savePassword: boolean;
	    connectTimeoutSeconds: number;
	    socket: string;
	    ssh: SSHOptions;
	    initSQL: string;
	    keepAliveSeconds: number;
	    folder: string;
	    comment: string;
	    databaseFilter: string;
	    stats: ConnStats;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    hasStoredPassword: boolean;
	    hasStoredSSHSecret: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ProfileView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.database = source["database"];
	        this.tls = this.convertValues(source["tls"], database.TLSOptions);
	        this.params = source["params"];
	        this.allowCleartextPasswords = source["allowCleartextPasswords"];
	        this.environment = source["environment"];
	        this.color = source["color"];
	        this.readOnly = source["readOnly"];
	        this.savePassword = source["savePassword"];
	        this.connectTimeoutSeconds = source["connectTimeoutSeconds"];
	        this.socket = source["socket"];
	        this.ssh = this.convertValues(source["ssh"], SSHOptions);
	        this.initSQL = source["initSQL"];
	        this.keepAliveSeconds = source["keepAliveSeconds"];
	        this.folder = source["folder"];
	        this.comment = source["comment"];
	        this.databaseFilter = source["databaseFilter"];
	        this.stats = this.convertValues(source["stats"], ConnStats);
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.hasStoredPassword = source["hasStoredPassword"];
	        this.hasStoredSSHSecret = source["hasStoredSSHSecret"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class SaveInput {
	    profile: Profile;
	    password?: string;
	    sshSecret?: string;
	
	    static createFrom(source: any = {}) {
	        return new SaveInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profile = this.convertValues(source["profile"], Profile);
	        this.password = source["password"];
	        this.sshSecret = source["sshSecret"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SaveResult {
	    profile: ProfileView;
	    warning?: string;
	
	    static createFrom(source: any = {}) {
	        return new SaveResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profile = this.convertValues(source["profile"], ProfileView);
	        this.warning = source["warning"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SessionState {
	    profileId: string;
	    status: string;
	    server: database.ServerInfo;
	    // Go type: time
	    openedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new SessionState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	        this.status = source["status"];
	        this.server = this.convertValues(source["server"], database.ServerInfo);
	        this.openedAt = this.convertValues(source["openedAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TestInput {
	    profile: Profile;
	    password?: string;
	    sshSecret?: string;
	
	    static createFrom(source: any = {}) {
	        return new TestInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profile = this.convertValues(source["profile"], Profile);
	        this.password = source["password"];
	        this.sshSecret = source["sshSecret"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace csvimport {
	
	export class ColumnGuess {
	    name: string;
	    type: string;
	    length: string;
	    unsigned: boolean;
	    nullable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ColumnGuess(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.length = source["length"];
	        this.unsigned = source["unsigned"];
	        this.nullable = source["nullable"];
	    }
	}
	export class Format {
	    separator: string;
	    encloser: string;
	    escape: string;
	    encoding: string;
	    skipLines: number;
	    nullMarker: string;
	    emptyNull: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Format(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.separator = source["separator"];
	        this.encloser = source["encloser"];
	        this.escape = source["escape"];
	        this.encoding = source["encoding"];
	        this.skipLines = source["skipLines"];
	        this.nullMarker = source["nullMarker"];
	        this.emptyNull = source["emptyNull"];
	    }
	}
	export class Detected {
	    format: Format;
	    header: string[];
	    sample: string[][];
	    columns: ColumnGuess[];
	
	    static createFrom(source: any = {}) {
	        return new Detected(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.format = this.convertValues(source["format"], Format);
	        this.header = source["header"];
	        this.sample = source["sample"];
	        this.columns = this.convertValues(source["columns"], ColumnGuess);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class Target {
	    database: string;
	    table: string;
	    columns: string[];
	    numeric: boolean[];
	    mode: string;
	    batchRows: number;
	    atomic: boolean;
	    thousands: string;
	    decimal: string;
	
	    static createFrom(source: any = {}) {
	        return new Target(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.database = source["database"];
	        this.table = source["table"];
	        this.columns = source["columns"];
	        this.numeric = source["numeric"];
	        this.mode = source["mode"];
	        this.batchRows = source["batchRows"];
	        this.atomic = source["atomic"];
	        this.thousands = source["thousands"];
	        this.decimal = source["decimal"];
	    }
	}

}

export namespace database {
	
	export class SortSpec {
	    column: string;
	    desc: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SortSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.column = source["column"];
	        this.desc = source["desc"];
	    }
	}
	export class BrowseSpec {
	    database: string;
	    table: string;
	    columns: string[];
	    filter: string;
	    sort: SortSpec[];
	    offset: number;
	    limit: number;
	    partialLength: number;
	    noLimit: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BrowseSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.database = source["database"];
	        this.table = source["table"];
	        this.columns = source["columns"];
	        this.filter = source["filter"];
	        this.sort = this.convertValues(source["sort"], SortSpec);
	        this.offset = source["offset"];
	        this.limit = source["limit"];
	        this.partialLength = source["partialLength"];
	        this.noLimit = source["noLimit"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CellValue {
	    kind: string;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new CellValue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.text = source["text"];
	    }
	}
	export class CharsetInfo {
	    name: string;
	    defaultCollation: string;
	    description: string;
	    collations: string[];
	
	    static createFrom(source: any = {}) {
	        return new CharsetInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.defaultCollation = source["defaultCollation"];
	        this.description = source["description"];
	        this.collations = source["collations"];
	    }
	}
	export class CheckDef {
	    origName: string;
	    name: string;
	    clause: string;
	
	    static createFrom(source: any = {}) {
	        return new CheckDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.origName = source["origName"];
	        this.name = source["name"];
	        this.clause = source["clause"];
	    }
	}
	export class CheckInfo {
	    name: string;
	    clause: string;
	
	    static createFrom(source: any = {}) {
	        return new CheckInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.clause = source["clause"];
	    }
	}
	export class ColumnDef {
	    origName: string;
	    name: string;
	    type: string;
	    length: string;
	    unsigned: boolean;
	    zerofill: boolean;
	    nullable: boolean;
	    defaultKind: string;
	    default: string;
	    autoIncrement: boolean;
	    onUpdate: string;
	    comment: string;
	    charset: string;
	    collation: string;
	    generated: string;
	    generatedKind: string;
	    invisible: boolean;
	    srid: string;
	
	    static createFrom(source: any = {}) {
	        return new ColumnDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.origName = source["origName"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.length = source["length"];
	        this.unsigned = source["unsigned"];
	        this.zerofill = source["zerofill"];
	        this.nullable = source["nullable"];
	        this.defaultKind = source["defaultKind"];
	        this.default = source["default"];
	        this.autoIncrement = source["autoIncrement"];
	        this.onUpdate = source["onUpdate"];
	        this.comment = source["comment"];
	        this.charset = source["charset"];
	        this.collation = source["collation"];
	        this.generated = source["generated"];
	        this.generatedKind = source["generatedKind"];
	        this.invisible = source["invisible"];
	        this.srid = source["srid"];
	    }
	}
	export class ColumnInfo {
	    name: string;
	    position: number;
	    dataType: string;
	    columnType: string;
	    nullable: boolean;
	    default?: string;
	    key?: string;
	    extra?: string;
	    comment?: string;
	
	    static createFrom(source: any = {}) {
	        return new ColumnInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.position = source["position"];
	        this.dataType = source["dataType"];
	        this.columnType = source["columnType"];
	        this.nullable = source["nullable"];
	        this.default = source["default"];
	        this.key = source["key"];
	        this.extra = source["extra"];
	        this.comment = source["comment"];
	    }
	}
	export class ColumnSpec {
	    name: string;
	    type: string;
	    length: string;
	    values: string[];
	    unsigned: boolean;
	    nullable: boolean;
	    autoIncrement: boolean;
	    primaryKey: boolean;
	    unique: boolean;
	    defaultKind: string;
	    default: string;
	    onUpdateCurrentTimestamp: boolean;
	    comment: string;
	
	    static createFrom(source: any = {}) {
	        return new ColumnSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.length = source["length"];
	        this.values = source["values"];
	        this.unsigned = source["unsigned"];
	        this.nullable = source["nullable"];
	        this.autoIncrement = source["autoIncrement"];
	        this.primaryKey = source["primaryKey"];
	        this.unique = source["unique"];
	        this.defaultKind = source["defaultKind"];
	        this.default = source["default"];
	        this.onUpdateCurrentTimestamp = source["onUpdateCurrentTimestamp"];
	        this.comment = source["comment"];
	    }
	}
	export class ColumnValue {
	    column: string;
	    value: CellValue;
	
	    static createFrom(source: any = {}) {
	        return new ColumnValue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.column = source["column"];
	        this.value = this.convertValues(source["value"], CellValue);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CreateDatabaseSpec {
	    name: string;
	    charset: string;
	    collation: string;
	    ifNotExists: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CreateDatabaseSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.charset = source["charset"];
	        this.collation = source["collation"];
	        this.ifNotExists = source["ifNotExists"];
	    }
	}
	export class IndexSpec {
	    name: string;
	    columns: string[];
	    unique: boolean;
	
	    static createFrom(source: any = {}) {
	        return new IndexSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.columns = source["columns"];
	        this.unique = source["unique"];
	    }
	}
	export class CreateTableSpec {
	    database: string;
	    name: string;
	    columns: ColumnSpec[];
	    indexes: IndexSpec[];
	    engine: string;
	    charset: string;
	    collation: string;
	    comment: string;
	    ifNotExists: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CreateTableSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.database = source["database"];
	        this.name = source["name"];
	        this.columns = this.convertValues(source["columns"], ColumnSpec);
	        this.indexes = this.convertValues(source["indexes"], IndexSpec);
	        this.engine = source["engine"];
	        this.charset = source["charset"];
	        this.collation = source["collation"];
	        this.comment = source["comment"];
	        this.ifNotExists = source["ifNotExists"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DBError {
	    code?: number;
	    sqlState?: string;
	    message: string;
	    line?: number;
	    near?: string;
	    category: string;
	
	    static createFrom(source: any = {}) {
	        return new DBError(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.sqlState = source["sqlState"];
	        this.message = source["message"];
	        this.line = source["line"];
	        this.near = source["near"];
	        this.category = source["category"];
	    }
	}
	export class DDLOptions {
	    charsets: CharsetInfo[];
	    engines: string[];
	    defaultCharset: string;
	    defaultCollation: string;
	    defaultEngine: string;
	    types: string[];
	
	    static createFrom(source: any = {}) {
	        return new DDLOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.charsets = this.convertValues(source["charsets"], CharsetInfo);
	        this.engines = source["engines"];
	        this.defaultCharset = source["defaultCharset"];
	        this.defaultCollation = source["defaultCollation"];
	        this.defaultEngine = source["defaultEngine"];
	        this.types = source["types"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DatabaseInfo {
	    name: string;
	    system: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DatabaseInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.system = source["system"];
	    }
	}
	export class DatabaseSummary {
	    name: string;
	    charset: string;
	    collation: string;
	    tables: number;
	    views: number;
	    size: number;
	    lastUpdate?: string;
	    system: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DatabaseSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.charset = source["charset"];
	        this.collation = source["collation"];
	        this.tables = source["tables"];
	        this.views = source["views"];
	        this.size = source["size"];
	        this.lastUpdate = source["lastUpdate"];
	        this.system = source["system"];
	    }
	}
	export class EventDef {
	    database: string;
	    name: string;
	    origName: string;
	    definer: string;
	    recurring: boolean;
	    executeAt: string;
	    intervalValue: string;
	    intervalField: string;
	    starts: string;
	    ends: string;
	    preserve: boolean;
	    status: string;
	    comment: string;
	    body: string;
	
	    static createFrom(source: any = {}) {
	        return new EventDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.database = source["database"];
	        this.name = source["name"];
	        this.origName = source["origName"];
	        this.definer = source["definer"];
	        this.recurring = source["recurring"];
	        this.executeAt = source["executeAt"];
	        this.intervalValue = source["intervalValue"];
	        this.intervalField = source["intervalField"];
	        this.starts = source["starts"];
	        this.ends = source["ends"];
	        this.preserve = source["preserve"];
	        this.status = source["status"];
	        this.comment = source["comment"];
	        this.body = source["body"];
	    }
	}
	export class EventInfo {
	    name: string;
	    definer: string;
	    type: string;
	    executeAt?: string;
	    intervalValue?: string;
	    intervalField?: string;
	    starts?: string;
	    ends?: string;
	    status: string;
	    onCompletion: string;
	    body: string;
	    comment?: string;
	    lastExecuted?: string;
	    created?: string;
	
	    static createFrom(source: any = {}) {
	        return new EventInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.definer = source["definer"];
	        this.type = source["type"];
	        this.executeAt = source["executeAt"];
	        this.intervalValue = source["intervalValue"];
	        this.intervalField = source["intervalField"];
	        this.starts = source["starts"];
	        this.ends = source["ends"];
	        this.status = source["status"];
	        this.onCompletion = source["onCompletion"];
	        this.body = source["body"];
	        this.comment = source["comment"];
	        this.lastExecuted = source["lastExecuted"];
	        this.created = source["created"];
	    }
	}
	export class ForeignKeyDef {
	    origName: string;
	    name: string;
	    columns: string[];
	    refDatabase: string;
	    refTable: string;
	    refColumns: string[];
	    onUpdate: string;
	    onDelete: string;
	
	    static createFrom(source: any = {}) {
	        return new ForeignKeyDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.origName = source["origName"];
	        this.name = source["name"];
	        this.columns = source["columns"];
	        this.refDatabase = source["refDatabase"];
	        this.refTable = source["refTable"];
	        this.refColumns = source["refColumns"];
	        this.onUpdate = source["onUpdate"];
	        this.onDelete = source["onDelete"];
	    }
	}
	export class ForeignKeyInfo {
	    name: string;
	    database: string;
	    table: string;
	    columns: string[];
	    refDatabase: string;
	    refTable: string;
	    refColumns: string[];
	    onUpdate: string;
	    onDelete: string;
	
	    static createFrom(source: any = {}) {
	        return new ForeignKeyInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.database = source["database"];
	        this.table = source["table"];
	        this.columns = source["columns"];
	        this.refDatabase = source["refDatabase"];
	        this.refTable = source["refTable"];
	        this.refColumns = source["refColumns"];
	        this.onUpdate = source["onUpdate"];
	        this.onDelete = source["onDelete"];
	    }
	}
	export class IndexPart {
	    column: string;
	    length: number;
	    desc: boolean;
	    expression: string;
	
	    static createFrom(source: any = {}) {
	        return new IndexPart(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.column = source["column"];
	        this.length = source["length"];
	        this.desc = source["desc"];
	        this.expression = source["expression"];
	    }
	}
	export class IndexDef {
	    origName: string;
	    name: string;
	    kind: string;
	    parts: IndexPart[];
	    algorithm: string;
	    comment: string;
	    invisible: boolean;
	
	    static createFrom(source: any = {}) {
	        return new IndexDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.origName = source["origName"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.parts = this.convertValues(source["parts"], IndexPart);
	        this.algorithm = source["algorithm"];
	        this.comment = source["comment"];
	        this.invisible = source["invisible"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class IndexInfo {
	    name: string;
	    columns: string[];
	    unique: boolean;
	    primary: boolean;
	    type: string;
	
	    static createFrom(source: any = {}) {
	        return new IndexInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.columns = source["columns"];
	        this.unique = source["unique"];
	        this.primary = source["primary"];
	        this.type = source["type"];
	    }
	}
	
	
	export class Process {
	    id: number;
	    user: string;
	    host: string;
	    db?: string;
	    command: string;
	    time: number;
	    state?: string;
	    info?: string;
	    progress?: number;
	
	    static createFrom(source: any = {}) {
	        return new Process(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.user = source["user"];
	        this.host = source["host"];
	        this.db = source["db"];
	        this.command = source["command"];
	        this.time = source["time"];
	        this.state = source["state"];
	        this.info = source["info"];
	        this.progress = source["progress"];
	    }
	}
	export class RoutineParam {
	    mode: string;
	    name: string;
	    type: string;
	
	    static createFrom(source: any = {}) {
	        return new RoutineParam(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.name = source["name"];
	        this.type = source["type"];
	    }
	}
	export class RoutineDef {
	    database: string;
	    name: string;
	    origName: string;
	    type: string;
	    origType: string;
	    params: RoutineParam[];
	    returns: string;
	    body: string;
	    deterministic: boolean;
	    dataAccess: string;
	    security: string;
	    definer: string;
	    comment: string;
	
	    static createFrom(source: any = {}) {
	        return new RoutineDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.database = source["database"];
	        this.name = source["name"];
	        this.origName = source["origName"];
	        this.type = source["type"];
	        this.origType = source["origType"];
	        this.params = this.convertValues(source["params"], RoutineParam);
	        this.returns = source["returns"];
	        this.body = source["body"];
	        this.deterministic = source["deterministic"];
	        this.dataAccess = source["dataAccess"];
	        this.security = source["security"];
	        this.definer = source["definer"];
	        this.comment = source["comment"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RoutineDetail {
	    name: string;
	    type: string;
	    params: RoutineParam[];
	    returns?: string;
	    body: string;
	    deterministic: boolean;
	    dataAccess: string;
	    security: string;
	    definer: string;
	    comment?: string;
	    created?: string;
	    modified?: string;
	
	    static createFrom(source: any = {}) {
	        return new RoutineDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.params = this.convertValues(source["params"], RoutineParam);
	        this.returns = source["returns"];
	        this.body = source["body"];
	        this.deterministic = source["deterministic"];
	        this.dataAccess = source["dataAccess"];
	        this.security = source["security"];
	        this.definer = source["definer"];
	        this.comment = source["comment"];
	        this.created = source["created"];
	        this.modified = source["modified"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RoutineInfo {
	    name: string;
	    type: string;
	
	    static createFrom(source: any = {}) {
	        return new RoutineInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	    }
	}
	
	export class RowEdit {
	    op: string;
	    where: ColumnValue[];
	    values: ColumnValue[];
	
	    static createFrom(source: any = {}) {
	        return new RowEdit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.op = source["op"];
	        this.where = this.convertValues(source["where"], ColumnValue);
	        this.values = this.convertValues(source["values"], ColumnValue);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ServerInfo {
	    version: string;
	    flavor: string;
	    tlsActive: boolean;
	    tlsCipher?: string;
	    user: string;
	
	    static createFrom(source: any = {}) {
	        return new ServerInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.flavor = source["flavor"];
	        this.tlsActive = source["tlsActive"];
	        this.tlsCipher = source["tlsCipher"];
	        this.user = source["user"];
	    }
	}
	export class ServerVariable {
	    name: string;
	    session?: string;
	    global?: string;
	
	    static createFrom(source: any = {}) {
	        return new ServerVariable(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.session = source["session"];
	        this.global = source["global"];
	    }
	}
	
	export class StatusValue {
	    name: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new StatusValue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.value = source["value"];
	    }
	}
	export class TLSOptions {
	    mode: string;
	    caFile?: string;
	    certFile?: string;
	    keyFile?: string;
	    serverName?: string;
	
	    static createFrom(source: any = {}) {
	        return new TLSOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.caFile = source["caFile"];
	        this.certFile = source["certFile"];
	        this.keyFile = source["keyFile"];
	        this.serverName = source["serverName"];
	    }
	}
	export class TableOptions {
	    engine: string;
	    collation: string;
	    comment: string;
	    rowFormat: string;
	    autoIncrement: string;
	    avgRowLength: string;
	    maxRows: string;
	    checksum: boolean;
	    partition: string;
	    convertData: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TableOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.engine = source["engine"];
	        this.collation = source["collation"];
	        this.comment = source["comment"];
	        this.rowFormat = source["rowFormat"];
	        this.autoIncrement = source["autoIncrement"];
	        this.avgRowLength = source["avgRowLength"];
	        this.maxRows = source["maxRows"];
	        this.checksum = source["checksum"];
	        this.partition = source["partition"];
	        this.convertData = source["convertData"];
	    }
	}
	export class TableDef {
	    database: string;
	    name: string;
	    origName: string;
	    columns: ColumnDef[];
	    indexes: IndexDef[];
	    foreignKeys: ForeignKeyDef[];
	    checks: CheckDef[];
	    options: TableOptions;
	    mariadb: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TableDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.database = source["database"];
	        this.name = source["name"];
	        this.origName = source["origName"];
	        this.columns = this.convertValues(source["columns"], ColumnDef);
	        this.indexes = this.convertValues(source["indexes"], IndexDef);
	        this.foreignKeys = this.convertValues(source["foreignKeys"], ForeignKeyDef);
	        this.checks = this.convertValues(source["checks"], CheckDef);
	        this.options = this.convertValues(source["options"], TableOptions);
	        this.mariadb = source["mariadb"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TableInfo {
	    name: string;
	    type: string;
	    engine?: string;
	    rows?: number;
	    comment?: string;
	
	    static createFrom(source: any = {}) {
	        return new TableInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.engine = source["engine"];
	        this.rows = source["rows"];
	        this.comment = source["comment"];
	    }
	}
	export class TableKey {
	    kind: string;
	    name?: string;
	    columns: string[];
	
	    static createFrom(source: any = {}) {
	        return new TableKey(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.columns = source["columns"];
	    }
	}
	
	export class TableStatus {
	    name: string;
	    type: string;
	    engine?: string;
	    version?: number;
	    rowFormat?: string;
	    rows?: number;
	    avgRowLength?: number;
	    dataLength?: number;
	    maxDataLength?: number;
	    indexLength?: number;
	    dataFree?: number;
	    autoIncrement?: string;
	    createTime?: string;
	    updateTime?: string;
	    checkTime?: string;
	    collation?: string;
	    checksum?: string;
	    createOptions?: string;
	    comment?: string;
	
	    static createFrom(source: any = {}) {
	        return new TableStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.engine = source["engine"];
	        this.version = source["version"];
	        this.rowFormat = source["rowFormat"];
	        this.rows = source["rows"];
	        this.avgRowLength = source["avgRowLength"];
	        this.dataLength = source["dataLength"];
	        this.maxDataLength = source["maxDataLength"];
	        this.indexLength = source["indexLength"];
	        this.dataFree = source["dataFree"];
	        this.autoIncrement = source["autoIncrement"];
	        this.createTime = source["createTime"];
	        this.updateTime = source["updateTime"];
	        this.checkTime = source["checkTime"];
	        this.collation = source["collation"];
	        this.checksum = source["checksum"];
	        this.createOptions = source["createOptions"];
	        this.comment = source["comment"];
	    }
	}
	export class TriggerDef {
	    database: string;
	    name: string;
	    origName: string;
	    table: string;
	    timing: string;
	    event: string;
	    body: string;
	    definer: string;
	
	    static createFrom(source: any = {}) {
	        return new TriggerDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.database = source["database"];
	        this.name = source["name"];
	        this.origName = source["origName"];
	        this.table = source["table"];
	        this.timing = source["timing"];
	        this.event = source["event"];
	        this.body = source["body"];
	        this.definer = source["definer"];
	    }
	}
	export class TriggerInfo {
	    name: string;
	    table: string;
	    timing: string;
	    event: string;
	    statement: string;
	    definer: string;
	    created?: string;
	    sqlMode?: string;
	    order: number;
	
	    static createFrom(source: any = {}) {
	        return new TriggerInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.table = source["table"];
	        this.timing = source["timing"];
	        this.event = source["event"];
	        this.statement = source["statement"];
	        this.definer = source["definer"];
	        this.created = source["created"];
	        this.sqlMode = source["sqlMode"];
	        this.order = source["order"];
	    }
	}
	export class ViewDef {
	    database: string;
	    name: string;
	    origName: string;
	    definer: string;
	    security: string;
	    algorithm: string;
	    checkOption: string;
	    select: string;
	
	    static createFrom(source: any = {}) {
	        return new ViewDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.database = source["database"];
	        this.name = source["name"];
	        this.origName = source["origName"];
	        this.definer = source["definer"];
	        this.security = source["security"];
	        this.algorithm = source["algorithm"];
	        this.checkOption = source["checkOption"];
	        this.select = source["select"];
	    }
	}
	export class Warning {
	    level: string;
	    code: number;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Warning(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.level = source["level"];
	        this.code = source["code"];
	        this.message = source["message"];
	    }
	}

}

export namespace dump {
	
	export class Selection {
	    database: string;
	    tables: string[];
	    routines: boolean;
	    events: boolean;
	    triggers: boolean;
	    allObjects: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Selection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.database = source["database"];
	        this.tables = source["tables"];
	        this.routines = source["routines"];
	        this.events = source["events"];
	        this.triggers = source["triggers"];
	        this.allObjects = source["allObjects"];
	    }
	}
	export class Options {
	    selections: Selection[];
	    dropDatabase: boolean;
	    createDatabase: boolean;
	    dropObjects: boolean;
	    createObjects: boolean;
	    data: string;
	    maxInsertKB: number;
	    comments: boolean;
	    wrapTransactions: boolean;
	    removeAutoIncrement: boolean;
	    removeDefiner: boolean;
	    targetDatabase: string;
	
	    static createFrom(source: any = {}) {
	        return new Options(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.selections = this.convertValues(source["selections"], Selection);
	        this.dropDatabase = source["dropDatabase"];
	        this.createDatabase = source["createDatabase"];
	        this.dropObjects = source["dropObjects"];
	        this.createObjects = source["createObjects"];
	        this.data = source["data"];
	        this.maxInsertKB = source["maxInsertKB"];
	        this.comments = source["comments"];
	        this.wrapTransactions = source["wrapTransactions"];
	        this.removeAutoIncrement = source["removeAutoIncrement"];
	        this.removeDefiner = source["removeDefiner"];
	        this.targetDatabase = source["targetDatabase"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace gridexport {
	
	export class Column {
	    name: string;
	    type: string;
	    numeric: boolean;
	    binary: boolean;
	    key: boolean;
	    autoIncrement: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Column(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.numeric = source["numeric"];
	        this.binary = source["binary"];
	        this.key = source["key"];
	        this.autoIncrement = source["autoIncrement"];
	    }
	}
	export class Options {
	    format: string;
	    separator: string;
	    encloser: string;
	    lineTerminator: string;
	    nullText: string;
	    header: boolean;
	    removeLineBreaks: boolean;
	    table: string;
	    query: string;
	    skipAutoIncrement: boolean;
	    rowsPerInsert: number;
	
	    static createFrom(source: any = {}) {
	        return new Options(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.format = source["format"];
	        this.separator = source["separator"];
	        this.encloser = source["encloser"];
	        this.lineTerminator = source["lineTerminator"];
	        this.nullText = source["nullText"];
	        this.header = source["header"];
	        this.removeLineBreaks = source["removeLineBreaks"];
	        this.table = source["table"];
	        this.query = source["query"];
	        this.skipAutoIncrement = source["skipAutoIncrement"];
	        this.rowsPerInsert = source["rowsPerInsert"];
	    }
	}

}

export namespace history {
	
	export class Entry {
	    id: number;
	    connectionId: string;
	    connectionName: string;
	    database: string;
	    sql: string;
	    // Go type: time
	    executedAt: any;
	    durationMs: number;
	    success: boolean;
	    rowCount: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.connectionId = source["connectionId"];
	        this.connectionName = source["connectionName"];
	        this.database = source["database"];
	        this.sql = source["sql"];
	        this.executedAt = this.convertValues(source["executedAt"], null);
	        this.durationMs = source["durationMs"];
	        this.success = source["success"];
	        this.rowCount = source["rowCount"];
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Query {
	    connectionId: string;
	    search: string;
	    limit: number;
	
	    static createFrom(source: any = {}) {
	        return new Query(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connectionId = source["connectionId"];
	        this.search = source["search"];
	        this.limit = source["limit"];
	    }
	}

}

export namespace mysql {
	
	export class Account {
	    user: string;
	    host: string;
	    isRole: boolean;
	    locked: boolean;
	    plugin: string;
	    emptyPassword: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Account(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user = source["user"];
	        this.host = source["host"];
	        this.isRole = source["isRole"];
	        this.locked = source["locked"];
	        this.plugin = source["plugin"];
	        this.emptyPassword = source["emptyPassword"];
	    }
	}
	export class BulkSpec {
	    moveTo: string;
	    engine: string;
	    convertCharset: string;
	    convertCollation: string;
	    defaultCollation: string;
	    resetAutoIncrement: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BulkSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.moveTo = source["moveTo"];
	        this.engine = source["engine"];
	        this.convertCharset = source["convertCharset"];
	        this.convertCollation = source["convertCollation"];
	        this.defaultCollation = source["defaultCollation"];
	        this.resetAutoIncrement = source["resetAutoIncrement"];
	    }
	}
	export class Grant {
	    level: string;
	    database: string;
	    name: string;
	    routineType: string;
	    privileges: string[];
	    columns: Record<string, Array<string>>;
	    grantOption: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Grant(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.level = source["level"];
	        this.database = source["database"];
	        this.name = source["name"];
	        this.routineType = source["routineType"];
	        this.privileges = source["privileges"];
	        this.columns = source["columns"];
	        this.grantOption = source["grantOption"];
	    }
	}
	export class PrivilegeCatalog {
	    global: string[];
	    database: string[];
	    table: string[];
	    column: string[];
	    routine: string[];
	    plugins: string[];
	    roles: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PrivilegeCatalog(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.global = source["global"];
	        this.database = source["database"];
	        this.table = source["table"];
	        this.column = source["column"];
	        this.routine = source["routine"];
	        this.plugins = source["plugins"];
	        this.roles = source["roles"];
	    }
	}
	export class RoleGrant {
	    role: string;
	    adminOption: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RoleGrant(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.adminOption = source["adminOption"];
	    }
	}
	export class SearchSpec {
	    text: string;
	    match: string;
	    caseSensitive: boolean;
	    types: string;
	
	    static createFrom(source: any = {}) {
	        return new SearchSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.match = source["match"];
	        this.caseSensitive = source["caseSensitive"];
	        this.types = source["types"];
	    }
	}
	export class UserDef {
	    user: string;
	    host: string;
	    origUser: string;
	    origHost: string;
	    isRole: boolean;
	    password: string;
	    plugin: string;
	    require: string;
	    cipher: string;
	    issuer: string;
	    subject: string;
	    maxQueries: number;
	    maxUpdates: number;
	    maxConnections: number;
	    maxUserConnections: number;
	    locked: boolean;
	    passwordExpire: string;
	    expireDays: number;
	    grants: Grant[];
	    roles: RoleGrant[];
	    defaultRoles: string[];
	
	    static createFrom(source: any = {}) {
	        return new UserDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user = source["user"];
	        this.host = source["host"];
	        this.origUser = source["origUser"];
	        this.origHost = source["origHost"];
	        this.isRole = source["isRole"];
	        this.password = source["password"];
	        this.plugin = source["plugin"];
	        this.require = source["require"];
	        this.cipher = source["cipher"];
	        this.issuer = source["issuer"];
	        this.subject = source["subject"];
	        this.maxQueries = source["maxQueries"];
	        this.maxUpdates = source["maxUpdates"];
	        this.maxConnections = source["maxConnections"];
	        this.maxUserConnections = source["maxUserConnections"];
	        this.locked = source["locked"];
	        this.passwordExpire = source["passwordExpire"];
	        this.expireDays = source["expireDays"];
	        this.grants = this.convertValues(source["grants"], Grant);
	        this.roles = this.convertValues(source["roles"], RoleGrant);
	        this.defaultRoles = source["defaultRoles"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace query {
	
	export class AdminResult {
	    sql: string;
	    rowsAffected?: number;
	    lastInsertId?: number;
	    durationMs: number;
	    error?: database.DBError;
	
	    static createFrom(source: any = {}) {
	        return new AdminResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sql = source["sql"];
	        this.rowsAffected = source["rowsAffected"];
	        this.lastInsertId = source["lastInsertId"];
	        this.durationMs = source["durationMs"];
	        this.error = this.convertValues(source["error"], database.DBError);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Column {
	    name: string;
	    type: string;
	    nullable?: boolean;
	    binary: boolean;
	    numeric: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Column(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.nullable = source["nullable"];
	        this.binary = source["binary"];
	        this.numeric = source["numeric"];
	    }
	}
	export class ExecuteRequest {
	    tabId: string;
	    connectionId: string;
	    sql: string;
	    cursorOffset: number;
	    mode: string;
	    database: string;
	    confirmed: boolean;
	    source?: string;
	    profile?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ExecuteRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tabId = source["tabId"];
	        this.connectionId = source["connectionId"];
	        this.sql = source["sql"];
	        this.cursorOffset = source["cursorOffset"];
	        this.mode = source["mode"];
	        this.database = source["database"];
	        this.confirmed = source["confirmed"];
	        this.source = source["source"];
	        this.profile = source["profile"];
	    }
	}
	export class StatementResult {
	    index: number;
	    sql: string;
	    line: number;
	    kind: string;
	    columns: Column[];
	    rows: any[][];
	    hasRows: boolean;
	    hasMore: boolean;
	    limitReached: boolean;
	    cursorId?: string;
	    truncatedCells?: number[][];
	    rowsAffected?: number;
	    lastInsertId?: number;
	    durationMs: number;
	    error?: database.DBError;
	    warnings?: database.Warning[];
	    profile?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new StatementResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.sql = source["sql"];
	        this.line = source["line"];
	        this.kind = source["kind"];
	        this.columns = this.convertValues(source["columns"], Column);
	        this.rows = source["rows"];
	        this.hasRows = source["hasRows"];
	        this.hasMore = source["hasMore"];
	        this.limitReached = source["limitReached"];
	        this.cursorId = source["cursorId"];
	        this.truncatedCells = source["truncatedCells"];
	        this.rowsAffected = source["rowsAffected"];
	        this.lastInsertId = source["lastInsertId"];
	        this.durationMs = source["durationMs"];
	        this.error = this.convertValues(source["error"], database.DBError);
	        this.warnings = this.convertValues(source["warnings"], database.Warning);
	        this.profile = source["profile"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ExecuteResponse {
	    executionId: string;
	    results: StatementResult[];
	    database: string;
	    inTransaction: boolean;
	    cancelled: boolean;
	    durationMs: number;
	    notice?: string;
	
	    static createFrom(source: any = {}) {
	        return new ExecuteResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.executionId = source["executionId"];
	        this.results = this.convertValues(source["results"], StatementResult);
	        this.database = source["database"];
	        this.inTransaction = source["inTransaction"];
	        this.cancelled = source["cancelled"];
	        this.durationMs = source["durationMs"];
	        this.notice = source["notice"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FetchRequest {
	    tabId: string;
	    cursorId: string;
	
	    static createFrom(source: any = {}) {
	        return new FetchRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tabId = source["tabId"];
	        this.cursorId = source["cursorId"];
	    }
	}
	export class FetchResponse {
	    rows: any[][];
	    hasMore: boolean;
	    limitReached: boolean;
	    truncatedCells?: number[][];
	    offset: number;
	    durationMs: number;
	    error?: database.DBError;
	
	    static createFrom(source: any = {}) {
	        return new FetchResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rows = source["rows"];
	        this.hasMore = source["hasMore"];
	        this.limitReached = source["limitReached"];
	        this.truncatedCells = source["truncatedCells"];
	        this.offset = source["offset"];
	        this.durationMs = source["durationMs"];
	        this.error = this.convertValues(source["error"], database.DBError);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class TabState {
	    tabId: string;
	    connectionId: string;
	    database: string;
	    inTransaction: boolean;
	    running: boolean;
	    hasCursor: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TabState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tabId = source["tabId"];
	        this.connectionId = source["connectionId"];
	        this.database = source["database"];
	        this.inTransaction = source["inTransaction"];
	        this.running = source["running"];
	        this.hasCursor = source["hasCursor"];
	    }
	}

}

export namespace tasks {
	
	export class Task {
	    id: string;
	    kind: string;
	    title: string;
	    status: string;
	    done: number;
	    total: number;
	    unit: string;
	    message?: string;
	    error?: string;
	    warnings: number;
	    // Go type: time
	    startedAt: any;
	    // Go type: time
	    finishedAt?: any;
	    result?: string;
	    path?: string;
	    data?: any;
	
	    static createFrom(source: any = {}) {
	        return new Task(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.title = source["title"];
	        this.status = source["status"];
	        this.done = source["done"];
	        this.total = source["total"];
	        this.unit = source["unit"];
	        this.message = source["message"];
	        this.error = source["error"];
	        this.warnings = source["warnings"];
	        this.startedAt = this.convertValues(source["startedAt"], null);
	        this.finishedAt = this.convertValues(source["finishedAt"], null);
	        this.result = source["result"];
	        this.path = source["path"];
	        this.data = source["data"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace workspace {
	
	export class SavedQuery {
	    id: string;
	    name: string;
	    sql: string;
	    connectionId?: string;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new SavedQuery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.sql = source["sql"];
	        this.connectionId = source["connectionId"];
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Tab {
	    id: string;
	    title: string;
	    connectionId: string;
	    database: string;
	    sql: string;
	    filePath?: string;
	    savedQueryId?: string;
	    savedContent?: string;
	    kind?: string;
	    objectName?: string;
	    objectType?: string;
	    view?: string;
	
	    static createFrom(source: any = {}) {
	        return new Tab(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.connectionId = source["connectionId"];
	        this.database = source["database"];
	        this.sql = source["sql"];
	        this.filePath = source["filePath"];
	        this.savedQueryId = source["savedQueryId"];
	        this.savedContent = source["savedContent"];
	        this.kind = source["kind"];
	        this.objectName = source["objectName"];
	        this.objectType = source["objectType"];
	        this.view = source["view"];
	    }
	}
	export class State {
	    tabs: Tab[];
	    activeTabId: string;
	    sidebarWidth: number;
	    resultsHeight: number;
	    logVisible?: boolean;
	    logHeight?: number;
	    helpersVisible?: boolean;
	    helpersWidth?: number;
	    openConnections?: string[];
	
	    static createFrom(source: any = {}) {
	        return new State(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tabs = this.convertValues(source["tabs"], Tab);
	        this.activeTabId = source["activeTabId"];
	        this.sidebarWidth = source["sidebarWidth"];
	        this.resultsHeight = source["resultsHeight"];
	        this.logVisible = source["logVisible"];
	        this.logHeight = source["logHeight"];
	        this.helpersVisible = source["helpersVisible"];
	        this.helpersWidth = source["helpersWidth"];
	        this.openConnections = source["openConnections"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}


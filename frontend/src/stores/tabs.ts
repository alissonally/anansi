import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'
import { backend } from '../api/backend'
import { toAppError } from '../api/errors'
import type { Cell, ConfirmationDetails, ExecuteRequest, StatementResult, TabKind, WorkspaceState } from '../api/types'
import { useConnectionsStore } from './connections'
import { useUiStore } from './ui'
import { utf8ByteOffset } from '../utils/text'

export interface ResultSet extends Omit<StatementResult, 'rows' | 'columns'> {
  rows: Cell[][]
  columns: NonNullable<StatementResult['columns']>
  loadingMore: boolean
  truncated: Set<string> // "row:col"
}

export type TabStatus = 'idle' | 'running' | 'cancelling'

export interface ErrorMarker {
  line: number // absolute, 1-based, in the editor
  message: string
}

export interface Tab {
  id: string
  /** "query" for SQL tabs; other kinds show an object or server view. */
  kind: TabKind
  /** Object tabs: object name (table, routine...) and routine type. */
  objectName?: string
  objectType?: string
  /** Object tabs: selected sub-view (e.g. "data", "structure"). */
  view?: string
  /** Object tabs: unsaved changes in an object editor. */
  objectDirty?: boolean
  /** Table tabs: filter to apply on the data grid (consumed once). */
  dataFilter?: string
  /** SQL tabs: collect the server profile on the next runs. */
  profile?: boolean
  /** SQL tabs: bind parameter values (:name), kept in memory only. */
  params?: Record<string, string>
  /** SQL tabs: substitute bind parameters before running. */
  useParams?: boolean
  title: string
  connectionId: string | null
  database: string
  sql: string
  filePath?: string
  savedQueryId?: string
  /** Content last saved to a file/saved query; null when never saved. */
  savedContent: string | null
  status: TabStatus
  startedAt: number | null
  results: ResultSet[]
  activeResult: number
  /** Request-level error (not tied to a statement). */
  error: string | null
  notice: string | null
  durationMs: number | null
  inTransaction: boolean
  cancelled: boolean
  marker: ErrorMarker | null
  /** Incremented on every execution; stale responses are ignored. */
  execSeq: number
}

export interface RunInput {
  /** Text to consider (whole editor content or selection). */
  sql: string
  /** UTF-16 cursor offset inside sql, or -1 to run every statement. */
  cursorOffset: number
  mode: 'statement' | 'script'
  /** Editor line (1-based) where sql starts; used to place error markers. */
  baseLine: number
}

function newId(): string {
  return crypto.randomUUID()
}

export function isDirty(t: Pick<Tab, 'sql' | 'savedContent'> & Partial<Pick<Tab, 'kind' | 'objectDirty'>>): boolean {
  if (t.kind && t.kind !== 'query') return !!t.objectDirty
  return t.savedContent === null ? t.sql.trim() !== '' : t.sql !== t.savedContent
}

/** Identity of an object tab: one tab per object. */
export interface ObjectTabSpec {
  kind: Exclude<TabKind, 'query'>
  dataFilter?: string
  connectionId: string
  database?: string
  objectName?: string
  objectType?: string
  view?: string
  title?: string
}

export const tabKindLabels: Record<TabKind, string> = {
  query: 'Consulta',
  host: 'Servidor',
  database: 'Database',
  table: 'Tabela',
  view: 'View',
  routine: 'Rotina',
  trigger: 'Trigger',
  event: 'Evento',
  users: 'Usuários',
}

function toResultSet(r: StatementResult): ResultSet {
  const truncated = new Set<string>((r.truncatedCells ?? []).map(([row, col]) => `${row}:${col}`))
  return { ...r, rows: r.rows ?? [], columns: r.columns ?? [], loadingMore: false, truncated }
}

/** Shows the confirmation required by the backend policies. */
export async function confirmStatements(d: ConfirmationDetails): Promise<boolean> {
  const ui = useUiStore()
  const destructive = d.statements.some((s) => s.reason !== 'alteração em conexão de produção')
  return ui.confirm({
    title: d.production ? `Executar em PRODUÇÃO — ${d.connection}` : 'Confirmar operação destrutiva',
    message: d.production
      ? 'As instruções abaixo alteram dados ou estrutura em uma conexão de produção.'
      : 'As instruções abaixo podem remover dados ou estruturas.',
    items: d.statements.map((s) => ({ text: s.sql, note: s.reason })),
    confirmLabel: 'Executar',
    danger: true,
    typeToConfirm: d.production && destructive ? d.connection : undefined,
  })
}

export const useTabsStore = defineStore('tabs', () => {
  const tabs = ref<Tab[]>([])
  const activeId = ref<string | null>(null)
  const restored = ref(false)
  const layout = ref({ sidebarWidth: 280, resultsHeight: 320, logVisible: false, logHeight: 160, helpersVisible: false, helpersWidth: 300 })
  let counter = 0

  const active = computed(() => tabs.value.find((t) => t.id === activeId.value) ?? null)

  function find(id: string): Tab | undefined {
    return tabs.value.find((t) => t.id === id)
  }

  function makeTab(partial: Partial<Tab> = {}): Tab {
    if (!partial.kind || partial.kind === 'query') counter++
    return {
      id: newId(),
      kind: 'query',
      title: `Consulta ${counter}`,
      connectionId: null,
      database: '',
      sql: '',
      savedContent: null,
      status: 'idle',
      startedAt: null,
      results: [],
      activeResult: 0,
      error: null,
      notice: null,
      durationMs: null,
      inTransaction: false,
      cancelled: false,
      marker: null,
      execSeq: 0,
      ...partial,
    }
  }

  function open(partial: Partial<Tab> = {}): Tab {
    const conns = useConnectionsStore()
    // Default to the active tab's connection, or the only connected one.
    if (partial.connectionId === undefined) {
      const fromActive = active.value?.connectionId ?? null
      const connected = conns.profiles.filter((p) => conns.state(p.id).status === 'connected')
      partial.connectionId = fromActive ?? (connected.length === 1 ? connected[0]!.id : null)
      if (partial.database === undefined && partial.connectionId) {
        partial.database = active.value?.connectionId === partial.connectionId ? active.value.database : (conns.byId.get(partial.connectionId)?.database ?? '')
      }
    }
    const t = makeTab(partial)
    const idx = tabs.value.findIndex((x) => x.id === activeId.value)
    tabs.value.splice(idx >= 0 ? idx + 1 : tabs.value.length, 0, t)
    activeId.value = t.id
    return find(t.id)!
  }

  /** Opens (or activates) the tab of an object or server view. */
  function openObject(spec: ObjectTabSpec): Tab {
    const db = spec.database ?? ''
    const existing = tabs.value.find((t) => t.kind === spec.kind && t.connectionId === spec.connectionId &&
      (spec.kind === 'host' || t.database === db) && (t.objectName ?? '') === (spec.objectName ?? '') &&
      (t.objectType ?? '') === (spec.objectType ?? ''))
    if (existing) {
      if (spec.view) existing.view = spec.view
      if (spec.dataFilter !== undefined) existing.dataFilter = spec.dataFilter
      activeId.value = existing.id
      return existing
    }
    const conns = useConnectionsStore()
    const title = spec.title ?? (spec.kind === 'host' ? (conns.byId.get(spec.connectionId)?.name ?? 'Servidor') : spec.kind === 'database' ? db : (spec.objectName ?? tabKindLabels[spec.kind]))
    return open({
      kind: spec.kind, connectionId: spec.connectionId, database: db, objectName: spec.objectName, objectType: spec.objectType,
      view: spec.view, title, dataFilter: spec.dataFilter,
    })
  }

  /** Closes the object tabs of an object that no longer exists. */
  function closeObjectTabs(connectionId: string, database: string, objectName?: string) {
    for (const t of [...tabs.value]) {
      if (t.kind === 'query' || t.connectionId !== connectionId || t.database !== database) continue
      if (objectName === undefined ? t.kind !== 'host' : t.objectName === objectName) void close(t.id, true)
    }
  }

  function activate(id: string) {
    if (find(id)) activeId.value = id
  }

  function rename(id: string, title: string) {
    const t = find(id)
    const v = title.trim()
    if (t && v) t.title = v.slice(0, 80)
  }

  function duplicate(id: string): Tab | undefined {
    const t = find(id)
    if (!t || t.kind !== 'query') return undefined
    activeId.value = id
    return open({ title: `${t.title} (cópia)`, connectionId: t.connectionId, database: t.database, sql: t.sql })
  }

  function setSql(id: string, sql: string) {
    const t = find(id)
    if (t && t.sql !== sql) {
      t.sql = sql
      if (t.marker) t.marker = null
    }
  }

  /** Asks the user before losing work in a tab. Returns true to proceed. */
  async function confirmClose(t: Tab): Promise<boolean> {
    const ui = useUiStore()
    if (t.status !== 'idle') {
      const ok = await ui.confirm({
        title: 'Consulta em execução',
        message: `A aba "${t.title}" está executando uma consulta. Fechar a aba cancela a execução.`,
        confirmLabel: 'Cancelar e fechar',
        danger: true,
      })
      if (!ok) return false
    }
    if (t.inTransaction) {
      const ok = await ui.confirm({
        title: 'Transação aberta',
        message: `A aba "${t.title}" possui uma transação aberta. Ao fechar, as alterações não confirmadas serão revertidas (ROLLBACK).`,
        confirmLabel: 'Fechar e reverter',
        danger: true,
      })
      if (!ok) return false
    }
    if (isDirty(t)) {
      const ok = await ui.confirm({
        title: 'Alterações não salvas',
        message: `O conteúdo da aba "${t.title}" não foi salvo em arquivo nem como consulta salva e será descartado.`,
        confirmLabel: 'Descartar',
        danger: true,
      })
      if (!ok) return false
    }
    return true
  }

  async function close(id: string, skipConfirm = false): Promise<boolean> {
    const t = find(id)
    if (!t) return false
    if (!skipConfirm && !(await confirmClose(t))) return false
    try {
      await backend.releaseTab(id, true)
    } catch {
      // The backend context may not exist (tab never executed).
    }
    const idx = tabs.value.findIndex((x) => x.id === id)
    if (idx < 0) return true
    tabs.value.splice(idx, 1)
    if (activeId.value === id) {
      const next = tabs.value[idx] ?? tabs.value[idx - 1] ?? null
      activeId.value = next?.id ?? null
    }
    return true
  }

  async function closeRight(id: string) {
    const i = tabs.value.findIndex((t) => t.id === id)
    for (const t of tabs.value.slice(i + 1).reverse()) {
      if (!(await close(t.id))) return
    }
  }

  async function closeAll() {
    for (const t of [...tabs.value].reverse()) {
      if (!(await close(t.id))) return
    }
  }

  async function closeOthers(id: string) {
    for (const t of [...tabs.value]) {
      if (t.id !== id && !(await close(t.id))) return
    }
    activeId.value = id
  }

  /**
   * Associates a tab with a connection. Confirms when an open transaction
   * would be rolled back or when switching into a production connection.
   */
  async function setConnection(id: string, connectionId: string | null): Promise<boolean> {
    const t = find(id)
    if (!t || t.connectionId === connectionId) return true
    const ui = useUiStore()
    const conns = useConnectionsStore()
    if (t.status !== 'idle') {
      ui.toast('warning', 'Aguarde o término da execução para trocar a conexão da aba.')
      return false
    }
    if (t.inTransaction) {
      const ok = await ui.confirm({
        title: 'Transação aberta',
        message: 'Trocar a conexão desta aba encerra a transação aberta; as alterações não confirmadas serão revertidas.',
        confirmLabel: 'Trocar e reverter',
        danger: true,
      })
      if (!ok) return false
    }
    const next = connectionId ? conns.byId.get(connectionId) : undefined
    const prev = t.connectionId ? conns.byId.get(t.connectionId) : undefined
    if (next?.environment === 'production' && prev?.environment !== 'production' && t.sql.trim() !== '') {
      const ok = await ui.confirm({
        title: 'Mudar para produção',
        message: `A aba passará a executar em "${next.name}", marcada como PRODUÇÃO. Revise o SQL antes de executar.`,
        confirmLabel: 'Usar produção',
        danger: true,
      })
      if (!ok) return false
    }
    try {
      await backend.releaseTab(id, true)
    } catch {
      // ignore: nothing pinned yet
    }
    t.connectionId = connectionId
    t.database = next?.database ?? ''
    t.inTransaction = false
    t.results = []
    t.error = null
    t.notice = null
    t.marker = null
    return true
  }

  function setDatabase(id: string, db: string) {
    const t = find(id)
    if (t) t.database = db
  }

  async function run(id: string, input: RunInput): Promise<void> {
    const t = find(id)
    if (!t) return
    const ui = useUiStore()
    const conns = useConnectionsStore()
    if (t.status !== 'idle') return
    if (!t.connectionId) {
      ui.toast('warning', 'Selecione uma conexão para esta aba antes de executar.')
      return
    }
    if (!input.sql.trim()) {
      ui.toast('info', 'Não há SQL para executar.')
      return
    }
    const connectionId = t.connectionId
    if (conns.state(connectionId).status !== 'connected') {
      if (!(await conns.connect(connectionId))) return
    }
    const seq = ++t.execSeq
    t.status = 'running'
    t.startedAt = performance.now()
    t.error = null
    t.notice = null
    t.marker = null
    t.cancelled = false
    t.durationMs = null

    const req: ExecuteRequest = {
      tabId: t.id,
      connectionId,
      sql: input.sql,
      cursorOffset: input.cursorOffset < 0 ? -1 : utf8ByteOffset(input.sql, input.cursorOffset),
      mode: input.mode,
      database: t.database,
      confirmed: false,
      profile: !!t.profile,
    }
    try {
      let resp
      try {
        resp = await backend.execute(req)
      } catch (e) {
        const err = toAppError(e)
        if (err.code !== 'confirmation_required') throw err
        // Nothing is running while the user decides.
        t.status = 'idle'
        t.startedAt = null
        const ok = await confirmStatements(err.details as ConfirmationDetails)
        const cur = find(id)
        if (!cur || cur.execSeq !== seq) return
        if (!ok) {
          cur.notice = 'Execução não confirmada; nada foi executado.'
          return
        }
        cur.status = 'running'
        cur.startedAt = performance.now()
        resp = await backend.execute({ ...req, confirmed: true })
      }
      const cur = find(id)
      if (!cur || cur.execSeq !== seq) return // tab closed or superseded
      cur.results = (resp.results ?? []).map(toResultSet)
      const firstError = cur.results.findIndex((r) => r.error)
      const firstRows = cur.results.findIndex((r) => r.hasRows)
      cur.activeResult = firstError >= 0 ? firstError : firstRows >= 0 ? firstRows : Math.max(0, cur.results.length - 1)
      cur.database = resp.database
      cur.inTransaction = resp.inTransaction
      cur.cancelled = resp.cancelled
      cur.durationMs = resp.durationMs
      cur.notice = resp.notice ?? null
      if (firstError >= 0) {
        const r = cur.results[firstError]!
        const errLine = r.error?.line && r.error.line > 0 ? r.error.line - 1 : 0
        cur.marker = { line: input.baseLine + r.line - 1 + errLine, message: r.error?.message ?? '' }
      }
      if (cur.results.some((r) => r.error?.category === 'network')) {
        void conns.refreshStates()
      }
    } catch (e) {
      const err = toAppError(e)
      const cur = find(id)
      if (!cur || cur.execSeq !== seq) return
      cur.results = []
      cur.error = err.message
      if (err.code === 'not_connected') {
        void conns.refreshStates()
      }
    } finally {
      const cur = find(id)
      if (cur && cur.execSeq === seq) {
        cur.status = 'idle'
        if (cur.durationMs === null && cur.startedAt !== null) cur.durationMs = performance.now() - cur.startedAt
        cur.startedAt = null
      }
    }
  }

  async function cancel(id: string) {
    const t = find(id)
    if (!t || t.status !== 'running') return
    t.status = 'cancelling'
    try {
      await backend.cancel(id)
    } catch (e) {
      useUiStore().toast('error', toAppError(e).message)
    }
  }

  async function fetchMore(id: string, index: number) {
    const t = find(id)
    const r = t?.results[index]
    if (!t || !r || !r.cursorId || !r.hasMore || r.loadingMore || t.status !== 'idle') return
    r.loadingMore = true
    const seq = t.execSeq
    try {
      const page = await backend.fetchMore(id, r.cursorId)
      if (t.execSeq !== seq) return
      for (const [row, col] of page.truncatedCells ?? []) r.truncated.add(`${row}:${col}`)
      r.rows.push(...(page.rows ?? []))
      r.hasMore = page.hasMore
      r.limitReached = page.limitReached
      if (page.error) r.error = page.error
    } catch (e) {
      r.hasMore = false
      useUiStore().toast('error', toAppError(e).message)
    } finally {
      r.loadingMore = false
    }
  }

  function markSaved(id: string, patch: { filePath?: string; savedQueryId?: string; title?: string }) {
    const t = find(id)
    if (!t) return
    t.savedContent = t.sql
    if (patch.filePath !== undefined) t.filePath = patch.filePath
    if (patch.savedQueryId !== undefined) t.savedQueryId = patch.savedQueryId
    if (patch.title) t.title = patch.title
  }

  // ---- persistence ---------------------------------------------------------

  function snapshot(): WorkspaceState {
    return {
      tabs: tabs.value.map((t) => ({
        id: t.id,
        title: t.title,
        connectionId: t.connectionId ?? '',
        database: t.database,
        sql: t.kind === 'query' ? t.sql : '',
        filePath: t.filePath,
        savedQueryId: t.savedQueryId,
        savedContent: t.savedContent,
        kind: t.kind,
        objectName: t.objectName,
        objectType: t.objectType,
        view: t.view,
      })),
      activeTabId: activeId.value ?? '',
      sidebarWidth: layout.value.sidebarWidth,
      resultsHeight: layout.value.resultsHeight,
      logVisible: layout.value.logVisible,
      logHeight: layout.value.logHeight,
      helpersVisible: layout.value.helpersVisible,
      helpersWidth: layout.value.helpersWidth,
      openConnections: useConnectionsStore().profiles.filter((p) => useConnectionsStore().state(p.id).status === 'connected').map((p) => p.id),
    }
  }

  /** Sessions open when the workspace was last saved. */
  const lastOpenConnections = ref<string[]>([])

  /** Restores the default panel sizes (HeidiSQL "Reset panel dimensions"). */
  function resetLayout() {
    Object.assign(layout.value, { sidebarWidth: 280, resultsHeight: 320, logHeight: 160, helpersWidth: 300 })
  }

  async function restore() {
    const conns = useConnectionsStore()
    try {
      const st = await backend.loadWorkspace()
      for (const p of st.tabs ?? []) {
        const valid = p.connectionId && conns.byId.has(p.connectionId)
        const kind = p.kind || 'query'
        if (kind !== 'query' && !valid) continue // object of a deleted connection
        // One tab per object, even if the file lists it twice.
        if (kind !== 'query' && tabs.value.some((t) => t.kind === kind && t.connectionId === p.connectionId && t.database === p.database &&
          (t.objectName ?? '') === (p.objectName ?? '') && (t.objectType ?? '') === (p.objectType ?? ''))) continue
        if (tabs.value.some((t) => t.id === p.id)) continue
        tabs.value.push(makeTab({
          kind,
          objectName: p.objectName || undefined,
          objectType: p.objectType || undefined,
          view: p.view || undefined,
          id: p.id || newId(),
          title: p.title || `Consulta ${counter + 1}`,
          connectionId: valid ? p.connectionId : null,
          database: valid ? p.database : '',
          sql: p.sql,
          filePath: p.filePath,
          savedQueryId: p.savedQueryId,
          savedContent: p.savedContent ?? null,
        }))
      }
      // Continue numbering after restored "Consulta N" titles.
      for (const t of tabs.value) {
        const n = Number(/^Consulta (\d+)$/.exec(t.title)?.[1] ?? 0)
        if (n > counter) counter = n
      }
      if (st.sidebarWidth > 0) layout.value.sidebarWidth = st.sidebarWidth
      if (st.resultsHeight > 0) layout.value.resultsHeight = st.resultsHeight
      layout.value.logVisible = !!st.logVisible
      if (st.logHeight && st.logHeight > 0) layout.value.logHeight = st.logHeight
      layout.value.helpersVisible = !!st.helpersVisible
      if (st.helpersWidth && st.helpersWidth > 0) layout.value.helpersWidth = st.helpersWidth
      lastOpenConnections.value = (st.openConnections ?? []).filter((id) => conns.byId.has(id))
      activeId.value = tabs.value.some((t) => t.id === st.activeTabId) ? st.activeTabId : (tabs.value[0]?.id ?? null)
    } catch (e) {
      useUiStore().toast('warning', `Não foi possível restaurar as abas: ${toAppError(e).message}`)
    }
    restored.value = true
  }

  let saveTimer: number | undefined
  let lastSaveError = ''
  function schedulePersist() {
    if (!restored.value) return
    window.clearTimeout(saveTimer)
    saveTimer = window.setTimeout(async () => {
      try {
        await backend.saveWorkspace(snapshot())
        lastSaveError = ''
      } catch (e) {
        const msg = toAppError(e).message
        if (msg !== lastSaveError) useUiStore().toast('warning', msg)
        lastSaveError = msg
      }
    }, 800)
  }

  watch(
    () => [tabs.value.map((t) => [t.id, t.title, t.connectionId, t.database, t.sql, t.filePath, t.savedContent, t.view]), activeId.value, layout.value,
      useConnectionsStore().profiles.map((p) => useConnectionsStore().state(p.id).status === 'connected')],
    schedulePersist,
    { deep: true },
  )

  /** Saves immediately (used before the window closes). */
  async function flush() {
    window.clearTimeout(saveTimer)
    if (restored.value) await backend.saveWorkspace(snapshot())
  }

  return {
    tabs, activeId, active, layout, restored,
    lastOpenConnections, resetLayout, find, open, openObject, closeObjectTabs, activate, rename, duplicate, setSql, close, closeOthers, closeRight, closeAll, setConnection, setDatabase,
    run, cancel, fetchMore, markSaved, restore, flush, snapshot,
  }
})

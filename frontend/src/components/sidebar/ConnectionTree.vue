<script setup lang="ts">
import { computed, ref } from 'vue'
import { environmentLabels, useConnectionsStore } from '../../stores/connections'
import { useMetadataStore } from '../../stores/metadata'
import { useSettingsStore } from '../../stores/settings'
import { useTabsStore } from '../../stores/tabs'
import { useUiStore } from '../../stores/ui'
import { toAppError } from '../../api/errors'
import Icon, { type IconName } from '../common/Icon.vue'
import ContextMenu, { type MenuItem } from '../common/ContextMenu.vue'
import type { ColumnInfo, DatabaseInfo, EventInfo, IndexInfo, ObjectKind, ObjectRef, ProfileView, RoutineInfo, TableInfo, TableRef, TriggerInfo } from '../../api/types'
import { forceQuoteIdent, quoteIdent } from '../../utils/text'
import { formatCount, globMatcher, textMatcher } from '../../utils/format'
import { kindLabels, useObjectActions } from '../../composables/objectActions'
import { tableStatement, type StatementKind } from '../../composables/templates'
import { routineCall } from '../../composables/templates'

const emit = defineEmits<{ insert: [text: string] }>()
const conns = useConnectionsStore()
const meta = useMetadataStore()
const settings = useSettingsStore()
const tabs = useTabsStore()
const ui = useUiStore()
const actions = useObjectActions()

const expanded = ref(new Set<string>())
const filter = ref('')
const menu = ref<{ x: number; y: number; items: MenuItem[] } | null>(null)

type RowKind = 'folder' | 'connection' | 'database' | 'group' | 'table' | 'view' | 'procedure' | 'function' | 'trigger' | 'event' | 'column' | 'indexGroup' | 'index' | 'status'

interface Row {
  key: string
  depth: number
  kind: RowKind
  label: string
  detail?: string
  icon: IconName
  expandable: boolean
  loading?: boolean
  error?: string
  conn: string
  db?: string
  table?: string
  profile?: ProfileView
  title?: string
  group?: string
}

const k = (...p: string[]) => p.join('\u0000')
const f = computed(() => filter.value.trim())
const match = computed(() => textMatcher(f.value))

const groupDefs: { id: string; label: string; icon: IconName; kind: ObjectKind }[] = [
  { id: 'tables', label: 'Tabelas', icon: 'table', kind: 'table' },
  { id: 'views', label: 'Views', icon: 'view', kind: 'view' },
  { id: 'procedures', label: 'Procedures', icon: 'fn', kind: 'procedure' },
  { id: 'functions', label: 'Functions', icon: 'fn', kind: 'function' },
  { id: 'triggers', label: 'Triggers', icon: 'zap', kind: 'trigger' },
  { id: 'events', label: 'Eventos', icon: 'clock', kind: 'event' },
]

// Folders (profile.folder "A/B") are expanded unless collapsed by the user.
const collapsedFolders = ref(new Set<string>())
function isOpen(r: Row): boolean {
  return r.kind === 'folder' ? !collapsedFolders.value.has(r.key) : expanded.value.has(r.key)
}

const rows = computed<Row[]>(() => {
  const out: Row[] = []
  const byFolder = new Map<string, ProfileView[]>()
  for (const p of conns.profiles) {
    const key = p.folder ?? ''
    if (!byFolder.has(key)) byFolder.set(key, [])
    byFolder.get(key)!.push(p)
  }
  const all = conns.folders
  const visibleUnder = (path: string) => [...byFolder.keys()].some((f) => f === path || f.startsWith(path + '/'))
  const render = (path: string, depth: number) => {
    const children = all.filter((x) => x.split('/').length === depth + 1 && (path === '' || x.startsWith(path + '/')))
    for (const fp of children) {
      if (!visibleUnder(fp)) continue
      const fk = k('folder', fp)
      const n = conns.profiles.filter((p) => (p.folder ?? '') === fp || (p.folder ?? '').startsWith(fp + '/')).length
      out.push({ key: fk, depth, kind: 'folder', label: fp.split('/').pop()!, detail: String(n), icon: 'folder', expandable: true, conn: '', title: fp })
      if (!collapsedFolders.value.has(fk) || f.value) render(fp, depth + 1)
    }
    for (const p of byFolder.get(path) ?? []) {
      const i0 = out.length
      addConnection(out, p)
      for (let i = i0; i < out.length; i++) out[i]!.depth += depth
    }
  }
  render('', 0)
  return out
})

function addConnection(out: Row[], p: ProfileView) {
    const st = conns.state(p.id)
    const ck = k(p.id)
    out.push({
      key: ck, depth: 0, kind: 'connection', label: p.name, detail: st.status === 'connected' ? st.server?.version : undefined,
      icon: 'server', expandable: true, loading: st.status === 'connecting', conn: p.id, profile: p,
      title: `${p.user}@${p.socket ? p.socket : `${p.host}:${p.port}`}${p.ssh?.enabled ? ` via SSH ${p.ssh.user}@${p.ssh.host}` : ''} — ${environmentLabels[p.environment]}${p.comment ? '\n' + p.comment : ''}`,
    })
    if (!expanded.value.has(ck) || st.status !== 'connected') return
    const dbs = meta.entry<DatabaseInfo[]>(p.id, 'dbs')
    if (!dbs || dbs.status === 'loading') { out.push(status(1, p.id, 'Carregando…', true)); return }
    if (dbs.status === 'error') { out.push(status(1, p.id, dbs.error ?? 'erro')); return }
    const dbFilter = globMatcher(p.databaseFilter)
    const visible = dbs.data.filter((d) => (settings.prefs.showSystemSchemas || !d.system) && dbFilter(d.name))
    for (const d of visible) {
      const dk = k(p.id, d.name)
      const tbls = meta.entry<TableInfo[]>(p.id, 'tables', d.name)
      const rts = meta.entry<RoutineInfo[]>(p.id, 'routines', d.name)?.data ?? []
      const trg = meta.entry<TriggerInfo[]>(p.id, 'triggers', d.name)?.data ?? []
      const evs = meta.entry<EventInfo[]>(p.id, 'events', d.name)?.data ?? []
      const byGroup: Record<string, { name: string; detail?: string; title?: string }[]> = {
        tables: (tbls?.data ?? []).filter((t) => t.type === 'table').map((t) => ({ name: t.name, detail: t.rows !== undefined && t.rows !== null ? `~${formatCount(t.rows)}` : undefined, title: t.comment || undefined })),
        views: (tbls?.data ?? []).filter((t) => t.type !== 'table').map((t) => ({ name: t.name })),
        procedures: rts.filter((r) => r.type === 'PROCEDURE').map((r) => ({ name: r.name })),
        functions: rts.filter((r) => r.type === 'FUNCTION').map((r) => ({ name: r.name })),
        triggers: trg.map((t) => ({ name: t.name, detail: `${t.timing.toLowerCase()} ${t.event.toLowerCase()}`, title: `${t.timing} ${t.event} em ${t.table}` })),
        events: evs.map((e) => ({ name: e.name, detail: e.status === 'ENABLED' ? undefined : 'desativado' })),
      }
      const dbMatches = match.value(d.name)
      const anyChild = f.value ? Object.values(byGroup).some((l) => l.some((o) => match.value(o.name))) : false
      if (f.value && tbls && !anyChild && !dbMatches) continue
      out.push({ key: dk, depth: 1, kind: 'database', label: d.name, icon: 'database', expandable: true, conn: p.id, db: d.name, loading: tbls?.status === 'loading' })
      if (!expanded.value.has(dk) && !(f.value && tbls && anyChild)) continue
      if (!tbls || tbls.status === 'loading') { if (expanded.value.has(dk)) out.push(status(2, p.id, 'Carregando…', true)); continue }
      if (tbls.status === 'error') { out.push(status(2, p.id, tbls.error ?? 'erro')); continue }
      for (const g of groupDefs) {
        const list = byGroup[g.id]!.filter((o) => !f.value || dbMatches && !anyChild || match.value(o.name))
        if (!list.length && g.id !== 'tables') continue
        const gk = k(p.id, d.name, g.id)
        out.push({ key: gk, depth: 2, kind: 'group', label: g.label, detail: String(list.length), icon: g.icon, expandable: true, conn: p.id, db: d.name, group: g.id })
        if (!expanded.value.has(gk) && !f.value) continue
        for (const o of list) {
          if (g.kind === 'table' || g.kind === 'view') addTable(out, p.id, d.name, g.kind, o)
          else out.push({ key: k(gk, o.name), depth: 3, kind: g.kind, label: o.name, detail: o.detail, title: o.title, icon: g.icon, expandable: false, conn: p.id, db: d.name, table: o.name })
        }
      }
    }
}

function status(depth: number, conn: string, label: string, loading = false): Row {
  return { key: k(conn, 'status', String(depth), label), depth, kind: 'status', label, icon: 'info', expandable: false, loading, conn, error: loading ? undefined : label }
}

function addTable(out: Row[], conn: string, db: string, kind: 'table' | 'view', t: { name: string; detail?: string; title?: string }) {
  const tk = k(conn, db, 't', t.name)
  out.push({ key: tk, depth: 3, kind, label: t.name, icon: kind === 'table' ? 'table' : 'view', expandable: true, conn, db, table: t.name, detail: t.detail, title: t.title })
  if (!expanded.value.has(tk)) return
  const cols = meta.entry<ColumnInfo[]>(conn, 'cols', db, t.name)
  if (!cols || cols.status === 'loading') { out.push(status(4, conn, 'Carregando…', true)); return }
  if (cols.status === 'error') { out.push(status(4, conn, cols.error ?? 'erro')); return }
  for (const c of cols.data) {
    out.push({
      key: k(tk, 'c', c.name), depth: 4, kind: 'column', label: c.name, detail: c.columnType + (c.nullable ? '' : ' not null'),
      icon: c.key === 'PRI' ? 'key' : 'column', expandable: false, conn, db, table: t.name, title: c.comment || undefined,
    })
  }
  if (kind === 'view') return
  const ik = k(tk, 'indexes')
  out.push({ key: ik, depth: 4, kind: 'indexGroup', label: 'Índices', icon: 'index', expandable: true, conn, db, table: t.name })
  if (!expanded.value.has(ik)) return
  const idx = meta.entry<IndexInfo[]>(conn, 'idx', db, t.name)
  if (!idx || idx.status === 'loading') { out.push(status(5, conn, 'Carregando…', true)); return }
  if (idx.status === 'error') { out.push(status(5, conn, idx.error ?? 'erro')); return }
  if (!idx.data.length) out.push(status(5, conn, 'Nenhum índice'))
  for (const i of idx.data) {
    out.push({
      key: k(ik, i.name), depth: 5, kind: 'index', label: i.name, detail: `(${i.columns.join(', ')})${i.unique && !i.primary ? ' unique' : ''}`,
      icon: i.primary ? 'key' : 'index', expandable: false, conn, db, table: t.name,
    })
  }
}

async function loadDatabase(conn: string, db: string, force = false) {
  // Triggers and events need extra privileges; their failure must not hide tables.
  await Promise.all([
    meta.tables(conn, db, force), meta.routines(conn, db, force),
    meta.triggers(conn, db, force).catch(() => undefined), meta.events(conn, db, force).catch(() => undefined),
  ])
}

async function toggle(r: Row) {
  if (!r.expandable) return
  if (r.kind === 'folder') {
    const c = new Set(collapsedFolders.value)
    if (c.has(r.key)) c.delete(r.key)
    else c.add(r.key)
    collapsedFolders.value = c
    return
  }
  const set = new Set(expanded.value)
  if (set.has(r.key)) {
    set.delete(r.key)
    expanded.value = set
    return
  }
  set.add(r.key)
  expanded.value = set
  try {
    switch (r.kind) {
      case 'connection':
        if (conns.state(r.conn).status !== 'connected' && !(await conns.connect(r.conn))) {
          const s = new Set(expanded.value)
          s.delete(r.key)
          expanded.value = s
          return
        }
        await meta.databases(r.conn)
        break
      case 'database':
        await loadDatabase(r.conn, r.db!)
        break
      case 'table':
      case 'view':
        await meta.columns(r.conn, r.db!, r.table!)
        break
      case 'indexGroup':
        await meta.indexes(r.conn, r.db!, r.table!)
        break
    }
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}

function objRef(r: Row): ObjectRef {
  if (r.kind === 'database') return { kind: 'database', database: '', name: r.db! }
  return { kind: r.kind as ObjectKind, database: r.db!, name: r.table! }
}

function qualified(r: Row) {
  return `${forceQuoteIdent(r.db!)}.${forceQuoteIdent(r.table!)}`
}

async function selectTop(r: Row) {
  const t = tabs.open({ title: r.table!, connectionId: r.conn, database: r.db! })
  const sql = `SELECT * FROM ${qualified(r)} LIMIT ${settings.prefs.maxRows};`
  tabs.setSql(t.id, sql)
  await tabs.run(t.id, { sql, cursorOffset: -1, mode: 'statement', baseLine: 1 })
}

function newQuery(conn: string, db?: string) {
  tabs.open({ connectionId: conn, database: db ?? conns.byId.get(conn)?.database ?? '' })
}

async function refresh(r: Row) {
  await meta.invalidate(r.conn, r.kind === 'connection' ? '' : (r.db ?? ''))
  if (r.kind !== 'connection' && r.db) await loadDatabase(r.conn, r.db, true).catch(() => undefined)
}

async function disconnect(id: string) {
  const affected = tabs.tabs.filter((t) => t.connectionId === id && (t.inTransaction || t.status !== 'idle'))
  if (affected.length) {
    const ok = await ui.confirm({
      title: 'Desconectar',
      message: 'Há abas com transação aberta ou consulta em execução nesta conexão. Desconectar cancela as execuções e reverte as transações.',
      items: affected.map((t) => ({ text: t.title, note: t.inTransaction ? 'transação aberta' : 'em execução' })),
      confirmLabel: 'Desconectar',
      danger: true,
    })
    if (!ok) return
  }
  await conns.disconnect(id)
  for (const t of tabs.tabs) if (t.connectionId === id) { t.inTransaction = false; if (t.results.some((r) => r.hasMore)) t.results.forEach((r) => (r.hasMore = false)) }
  await meta.invalidate(id, '', false, false)
}

async function remove(p: ProfileView) {
  const ok = await ui.confirm({ title: 'Excluir conexão', message: `Excluir a conexão "${p.name}"? A senha salva no keyring também será removida.`, confirmLabel: 'Excluir', danger: true })
  if (!ok) return
  try {
    await conns.remove(p.id)
    for (const t of [...tabs.tabs]) {
      if (t.connectionId !== p.id) continue
      if (t.kind === 'query') t.connectionId = null
      else void tabs.close(t.id, true)
    }
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}

async function move(p: ProfileView, delta: number) {
  const ids = conns.profiles.map((x) => x.id)
  const i = ids.indexOf(p.id)
  const j = i + delta
  if (j < 0 || j >= ids.length) return
  ;[ids[i], ids[j]] = [ids[j]!, ids[i]!]
  await conns.reorder(ids).catch((e) => ui.toast('error', toAppError(e).message))
}

async function copy(text: string) {
  await navigator.clipboard.writeText(text).catch(() => undefined)
}

function expand(...keys: string[]) {
  const set = new Set(expanded.value)
  for (const key of keys) set.add(key)
  expanded.value = set
}

function collapseAll(conn?: string) {
  expanded.value = new Set(conn ? [...expanded.value].filter((x) => !x.startsWith(conn)) : [])
}

async function newDatabase(conn: string) {
  if (conns.state(conn).status !== 'connected' && !(await conns.connect(conn))) return
  ui.ddlDialog = {
    kind: 'database',
    connectionId: conn,
    onCreated: async (name) => {
      expand(k(conn), k(conn, name))
      await loadDatabase(conn, name, true).catch(() => undefined)
    },
  }
}

function newObject(conn: string, db: string, kind: ObjectKind) {
  expand(k(conn), k(conn, db))
  actions.openNew(conn, db, kind)
}

function newMenu(conn: string, db: string): MenuItem[] {
  return (['table', 'view', 'procedure', 'function', 'trigger', 'event'] as ObjectKind[]).map((kind) => ({
    label: `Novo(a) ${kindLabels[kind]}…`, icon: kind === 'table' ? 'table' : 'plus', action: () => newObject(conn, db, kind),
  }))
}

async function generate(r: Row, kind: StatementKind) {
  try {
    const cols = await meta.columns(r.conn, r.db!, r.table!)
    tabs.open({ title: `${kind.toUpperCase()} ${r.table}`, connectionId: r.conn, database: r.db!, sql: tableStatement(kind, r.db!, r.table!, cols) })
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}

async function runRoutine(r: Row) {
  try {
    const list = await meta.routineDetails(r.conn, r.db!)
    const det = list.find((x) => x.name === r.table && x.type === r.kind.toUpperCase())
    if (!det) return
    tabs.open({ title: `Executar ${r.table}`, connectionId: r.conn, database: r.db!, sql: routineCall(r.db!, det) })
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}

function openObject(r: Row, view?: string) {
  if (r.kind === 'connection') {
    tabs.openObject({ kind: 'host', connectionId: r.conn, view })
    return
  }
  actions.open(r.conn, objRef(r), view)
}

async function dropRow(r: Row) {
  if (await actions.drop(r.conn, [objRef(r)])) {
    if (r.kind === 'database') await meta.databases(r.conn, true).catch(() => undefined)
  }
}

function toolsMenu(conn: string, databases: string[], tables?: TableRef[]): MenuItem[] {
  const open = (tab: 'maintenance' | 'search' | 'bulk') => () => (ui.toolsDialog = { connectionId: conn, databases, tables, tab })
  return [
    { label: 'Manutenção (CHECK, OPTIMIZE…)…', action: open('maintenance') },
    { label: 'Buscar texto…', icon: 'search', action: open('search') },
    { label: 'Edição em massa…', action: open('bulk') },
    ...(tables ? [] : [{ label: 'Comparar estruturas…', action: () => (ui.compareDialog = { connectionId: conn, database: databases[0] ?? '' }) }]),
  ]
}

async function moveToFolder(p: ProfileView) {
  const r = await ui.prompt({ title: 'Mover para a pasta', label: 'Pasta (vazio = raiz; use / para subpastas)', value: p.folder, confirmLabel: 'Mover' })
  if (!r) return
  try {
    await conns.save({ ...p, folder: r.value }, null)
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}

async function renameFolder(path: string) {
  const r = await ui.prompt({ title: 'Renomear pasta', label: 'Novo caminho', value: path, confirmLabel: 'Renomear' })
  if (!r || r.value.trim() === path) return
  const to = r.value.split('/').map((x) => x.trim()).filter(Boolean).join('/')
  try {
    for (const p of conns.profiles.filter((x) => x.folder === path || x.folder.startsWith(path + '/'))) {
      await conns.save({ ...p, folder: (to + p.folder.slice(path.length)).replace(/^\//, '') }, null)
    }
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}

function openMenu(e: MouseEvent, r: Row) {
  e.preventDefault()
  let items: MenuItem[]
  const connected = conns.state(r.conn).status === 'connected'
  const sep: MenuItem = { separator: true, label: '' }
  switch (r.kind) {
    case 'connection': {
      const p = r.profile!
      items = [
        connected
          ? { label: 'Desconectar', icon: 'unplug', action: () => disconnect(p.id) }
          : { label: 'Conectar', icon: 'plug', action: () => toggle(r) },
        { label: 'Visão do servidor', icon: 'server', action: () => openObject(r), disabled: !connected },
        { label: 'Processos', action: () => openObject(r, 'processes'), disabled: !connected },
        { label: 'Variáveis', action: () => openObject(r, 'variables'), disabled: !connected },
        { label: 'Nova consulta', icon: 'plus', action: () => newQuery(p.id) },
        { label: 'Novo database…', icon: 'database', action: () => newDatabase(p.id) },
        { label: 'Exportar databases como SQL…', icon: 'download', disabled: !connected, action: () => (ui.dumpDialog = { connectionId: p.id, databases: [] }) },
        { label: 'Executar arquivo SQL…', icon: 'upload', disabled: !connected, action: () => actions.runSqlFile(p.id, '') },
        { label: 'Gerenciar usuários', icon: 'user', disabled: !connected, action: () => tabs.openObject({ kind: 'users', connectionId: p.id }) },
        { label: 'Ferramentas', icon: 'wrench', disabled: !connected, children: toolsMenu(p.id, []) },
        { label: 'Filtrar databases…', icon: 'filter', disabled: !connected, action: () => (ui.databaseFilterDialog = { connectionId: p.id }) },
        { label: 'Atualizar', icon: 'refresh', action: () => refresh(r), disabled: !connected },
        { label: 'Recolher tudo', action: () => collapseAll(p.id) },
        sep,
        { label: 'Editar…', icon: 'edit', action: () => ui.openConnectionDialog(p.id) },
        { label: 'Duplicar', icon: 'copy', action: () => conns.duplicate(p.id).catch((er) => ui.toast('error', toAppError(er).message)) },
        { label: 'Mover para a pasta…', icon: 'folder', action: () => moveToFolder(p) },
        { label: 'Mover para cima', icon: 'arrowUp', action: () => move(p, -1) },
        { label: 'Mover para baixo', icon: 'arrowDown', action: () => move(p, 1) },
        sep,
        { label: 'Exportar esta conexão…', action: () => conns.exportProfiles([p.id]) },
        { label: 'Exportar todas as conexões…', action: () => conns.exportProfiles() },
        { label: 'Importar conexões…', icon: 'upload', action: () => conns.importProfiles() },
        sep,
        { label: 'Excluir…', icon: 'trash', danger: true, action: () => remove(p) },
      ]
      break
    }
    case 'folder':
      items = [
        { label: 'Renomear pasta…', icon: 'edit', action: () => renameFolder(r.title!) },
        { label: 'Exportar conexões da pasta…', action: () => conns.exportProfiles(conns.profiles.filter((p) => p.folder === r.title || p.folder.startsWith(r.title + '/')).map((p) => p.id)) },
        { label: 'Importar conexões…', icon: 'upload', action: () => conns.importProfiles() },
      ]
      break
    case 'database':
      items = [
        { label: 'Abrir', icon: 'database', action: () => openObject(r) },
        { label: 'Nova consulta neste database', icon: 'terminal', action: () => newQuery(r.conn, r.db) },
        ...newMenu(r.conn, r.db!),
        { label: 'Novo database…', icon: 'database', action: () => newDatabase(r.conn) },
        sep,
        { label: 'Exportar como SQL…', icon: 'download', action: () => (ui.dumpDialog = { connectionId: r.conn, databases: [r.db!] }) },
        { label: 'Importar CSV…', icon: 'upload', action: () => (ui.csvDialog = { connectionId: r.conn, database: r.db! }) },
        { label: 'Executar arquivo SQL…', action: () => actions.runSqlFile(r.conn, r.db!) },
        { label: 'Ferramentas', icon: 'wrench', children: toolsMenu(r.conn, [r.db!]) },
        sep,
        { label: 'Copiar nome', icon: 'copy', action: () => copy(r.db!) },
        { label: 'Copiar CREATE DATABASE', action: () => actions.copyDdl(r.conn, objRef(r)) },
        { label: 'Atualizar', icon: 'refresh', action: () => refresh(r) },
        sep,
        { label: 'Excluir database…', icon: 'trash', danger: true, action: () => dropRow(r) },
      ]
      break
    case 'table':
    case 'view':
      items = [
        { label: 'Abrir dados', icon: 'table', action: () => openObject(r, 'data') },
        { label: 'Abrir estrutura', icon: 'column', action: () => openObject(r, 'structure') },
        { label: `Selecionar ${settings.prefs.maxRows} linhas em nova aba`, icon: 'play', action: () => selectTop(r) },
        sep,
        { label: 'Gerar SELECT', action: () => generate(r, 'select') },
        ...(r.kind === 'table' ? [
          { label: 'Gerar INSERT', action: () => generate(r, 'insert') },
          { label: 'Gerar UPDATE', action: () => generate(r, 'update') },
          { label: 'Gerar DELETE', action: () => generate(r, 'delete') },
        ] : []),
        { label: 'Exportar dados…', icon: 'download', action: () => openObject(r, 'data') },
        { label: 'Exportar como SQL…', action: () => (ui.dumpDialog = { connectionId: r.conn, databases: [r.db!], tables: [r.table!] }) },
        ...(r.kind === 'table' ? [
          { label: 'Importar CSV…', icon: 'upload' as IconName, action: () => (ui.csvDialog = { connectionId: r.conn, database: r.db!, table: r.table! }) },
          { label: 'Copiar tabela…', icon: 'copy' as IconName, action: () => (ui.copyTableDialog = { connectionId: r.conn, database: r.db!, table: r.table! }) },
          { label: 'Gerar dados de teste…', action: () => (ui.generateDialog = { connectionId: r.conn, database: r.db!, table: r.table! }) },
          { label: 'Ferramentas', icon: 'wrench' as IconName, children: toolsMenu(r.conn, [], [{ database: r.db!, table: r.table! }]) },
        ] : []),
        { label: 'Inserir nome no editor', action: () => emit('insert', qualified(r)) },
        { label: 'Copiar nome', icon: 'copy', action: () => copy(r.table!) },
        { label: 'Copiar código CREATE', action: () => actions.copyDdl(r.conn, objRef(r)) },
        sep,
        { label: 'Renomear…', icon: 'edit', action: () => actions.rename(r.conn, objRef(r)) },
        { label: 'Atualizar colunas', icon: 'refresh', action: () => meta.columns(r.conn, r.db!, r.table!, true).catch(() => undefined) },
        ...(r.kind === 'table' ? [{ label: 'Esvaziar tabela…', danger: true, action: () => actions.truncate(r.conn, r.db!, [r.table!]) }] : []),
        { label: `Excluir ${kindLabels[r.kind]}…`, icon: 'trash', danger: true, action: () => dropRow(r) },
      ]
      break
    case 'procedure':
    case 'function':
    case 'trigger':
    case 'event':
      items = [
        { label: 'Abrir', icon: 'edit', action: () => openObject(r) },
        ...(r.kind === 'procedure' || r.kind === 'function' ? [{ label: 'Executar…', icon: 'play' as IconName, action: () => runRoutine(r) }] : []),
        { label: 'Código CREATE em nova aba', action: () => actions.ddlInEditor(r.conn, objRef(r)) },
        { label: 'Copiar código CREATE', icon: 'copy', action: () => actions.copyDdl(r.conn, objRef(r)) },
        { label: 'Copiar nome', action: () => copy(r.table!) },
        ...(r.kind === 'event' ? [{ label: 'Renomear…', icon: 'edit' as IconName, action: () => actions.rename(r.conn, objRef(r)) }] : []),
        sep,
        { label: `Excluir ${kindLabels[r.kind]}…`, icon: 'trash', danger: true, action: () => dropRow(r) },
      ]
      break
    case 'group': {
      const g = groupDefs.find((x) => x.id === r.group)
      items = [
        ...(g ? [{ label: `Novo(a) ${kindLabels[g.kind]}…`, icon: 'plus' as IconName, action: () => newObject(r.conn, r.db!, g.kind) }] : []),
        { label: 'Abrir lista do database', icon: 'database', action: () => actions.open(r.conn, { kind: 'database', database: '', name: r.db! }) },
        { label: 'Atualizar', icon: 'refresh', action: () => refresh(r) },
      ]
      break
    }
    case 'column':
      items = [
        { label: 'Inserir nome no editor', action: () => emit('insert', quoteIdent(r.label)) },
        { label: 'Copiar nome', icon: 'copy', action: () => copy(r.label) },
        { label: 'Abrir estrutura da tabela', icon: 'column', action: () => actions.open(r.conn, { kind: 'table', database: r.db!, name: r.table! }, 'structure') },
      ]
      break
    default:
      return
  }
  menu.value = { x: e.clientX, y: e.clientY, items }
}

function onKey(e: KeyboardEvent, r: Row) {
  if (e.key === 'Enter') {
    e.preventDefault()
    if (r.kind === 'connection' || r.kind === 'database' || r.kind === 'group' || r.kind === 'indexGroup') void toggle(r)
    else onDblClick(r)
  } else if (e.key === ' ') {
    e.preventDefault()
    if (r.expandable) void toggle(r)
  } else if (e.key === 'ArrowRight' && r.expandable && !expanded.value.has(r.key)) {
    void toggle(r)
  } else if (e.key === 'ArrowLeft' && expanded.value.has(r.key)) {
    void toggle(r)
  } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault()
    const el = e.currentTarget as HTMLElement
    const sib = (e.key === 'ArrowDown' ? el.nextElementSibling : el.previousElementSibling) as HTMLElement | null
    sib?.focus()
  } else if (e.key === 'F5') {
    e.preventDefault()
    void refresh(r)
  } else if (e.key === 'Delete' && ['database', 'table', 'view', 'procedure', 'function', 'trigger', 'event'].includes(r.kind)) {
    e.preventDefault()
    void dropRow(r)
  } else if (e.key === 'F2' && (r.kind === 'table' || r.kind === 'view' || r.kind === 'event')) {
    e.preventDefault()
    void actions.rename(r.conn, objRef(r))
  }
}

function onDblClick(r: Row) {
  switch (r.kind) {
    case 'connection':
      if (conns.state(r.conn).status === 'connected') openObject(r)
      break
    case 'database':
    case 'table':
    case 'view':
    case 'procedure':
    case 'function':
    case 'trigger':
    case 'event':
      openObject(r)
      break
    case 'column':
      emit('insert', quoteIdent(r.label))
      break
  }
}
</script>

<template>
  <div class="tree-wrap">
    <div class="search">
      <Icon name="search" :size="13" />
      <input v-model="filter" class="filter" placeholder="Filtrar objetos (regex; ; = ou)" aria-label="Filtrar objetos" />
    </div>
    <div v-if="!conns.profiles.length && conns.loaded" class="empty">
      <Icon name="database" :size="28" />
      <p>Nenhuma conexão cadastrada.</p>
      <button class="btn primary" @click="ui.openConnectionDialog()"><Icon name="plus" :size="14" /> Adicionar conexão</button>
    </div>
    <div v-else class="tree" role="tree" aria-label="Conexões e objetos">
      <div
        v-for="r in rows" :key="r.key" class="node" :class="[r.kind, { connected: r.kind === 'connection' && conns.state(r.conn).status === 'connected' }]"
        role="treeitem" :aria-expanded="r.expandable ? isOpen(r) : undefined" :aria-level="r.depth + 1" tabindex="0"
        :style="{ paddingLeft: 6 + r.depth * 14 + 'px' }" :title="r.title ?? r.label"
        @click="toggle(r)" @dblclick="onDblClick(r)" @contextmenu="openMenu($event, r)" @keydown="onKey($event, r)"
      >
        <span class="chev">
          <Icon v-if="r.expandable" :name="isOpen(r) && (r.kind !== 'connection' || conns.state(r.conn).status === 'connected') ? 'chevronDown' : 'chevronRight'" :size="12" />
        </span>
        <span v-if="r.kind === 'connection'" class="env" :class="`env-${r.profile!.environment}`" :style="r.profile!.color ? { background: r.profile!.color } : undefined" />
        <span v-if="r.loading" class="spinner" />
        <Icon v-else :name="r.icon" :size="14" class="ico" />
        <span class="label ellipsis" :class="{ err: r.error }">{{ r.label }}</span>
        <button
          v-if="r.kind === 'connection' && r.profile!.databaseFilter" class="icon-btn filter-flag" title="Filtro de databases ativo — clique para editar"
          aria-label="Filtro de databases ativo" @click.stop="ui.databaseFilterDialog = { connectionId: r.conn }"
        ><Icon name="filter" :size="12" /></button>
        <span v-if="r.kind === 'connection' && r.profile!.environment === 'production'" class="badge env-badge env-production">prod</span>
        <span v-if="r.detail" class="detail ellipsis">{{ r.detail }}</span>
        <span v-if="r.kind === 'connection'" class="conn-actions">
          <button v-if="conns.state(r.conn).status === 'connected'" class="icon-btn" title="Desconectar" aria-label="Desconectar" @click.stop="disconnect(r.conn)"><Icon name="unplug" :size="13" /></button>
          <button class="icon-btn" title="Nova consulta" aria-label="Nova consulta" @click.stop="newQuery(r.conn)"><Icon name="plus" :size="13" /></button>
          <button class="icon-btn" title="Mais ações" aria-label="Mais ações" @click.stop="openMenu($event, r)"><Icon name="more" :size="13" /></button>
        </span>
      </div>
    </div>
    <ContextMenu v-if="menu" :x="menu.x" :y="menu.y" :items="menu.items" @close="menu = null" />
  </div>
</template>

<style scoped>
.tree-wrap {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}
.search {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 8px;
  padding: 0 8px;
  height: 28px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--bg-input);
  color: var(--text-faint);
}
.filter {
  flex: 1;
  min-width: 0;
  border: none;
  background: transparent;
  outline: none;
  user-select: text;
}
.tree {
  flex: 1;
  overflow: auto;
  padding-bottom: 12px;
}
.node {
  display: flex;
  align-items: center;
  gap: 5px;
  height: 26px;
  padding-right: 6px;
  cursor: pointer;
  white-space: nowrap;
  outline: none;
}
.node:hover {
  background: var(--bg-hover);
}
.node:focus-visible {
  background: var(--bg-active);
}
.chev {
  width: 12px;
  color: var(--text-faint);
  flex: none;
}
.ico {
  color: var(--text-muted);
}
.node.connection .ico {
  color: var(--text-faint);
}
.node.connection.connected .ico {
  color: var(--success);
}
.node.connection .label {
  font-weight: 600;
}
.node.procedure .ico,
.node.function .ico,
.node.trigger .ico,
.node.event .ico {
  color: var(--warning);
}
.node.table .ico,
.node.view .ico {
  color: var(--accent);
}
.node.column .ico {
  color: var(--text-faint);
}
.node.status {
  color: var(--text-faint);
  cursor: default;
}
.label {
  min-width: 0;
}
.label.err {
  color: var(--danger);
  white-space: normal;
}
.detail {
  margin-left: auto;
  padding-left: 8px;
  color: var(--text-faint);
  font-size: 11px;
  max-width: 50%;
}
.env {
  width: 4px;
  height: 16px;
  border-radius: 2px;
  flex: none;
}
.filter-flag {
  flex: none;
  width: 18px;
  height: 18px;
  color: var(--accent);
}
.conn-actions {
  display: none;
  margin-left: auto;
}
.node.connection:hover .conn-actions,
.node.connection:focus-within .conn-actions {
  display: flex;
}
.node.connection:hover .detail {
  display: none;
}
.conn-actions .icon-btn {
  width: 22px;
  height: 22px;
}
.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 32px 16px;
  color: var(--text-muted);
  text-align: center;
}
.empty p {
  margin: 0;
}
</style>

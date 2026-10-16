<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { Tab } from '../../stores/tabs'
import { useTabsStore } from '../../stores/tabs'
import { useConnectionsStore } from '../../stores/connections'
import { useMetadataStore } from '../../stores/metadata'
import { useUiStore } from '../../stores/ui'
import { toAppError } from '../../api/errors'
import type { EventInfo, ObjectKind, ObjectRef, RoutineDetail, TableStatus, TriggerInfo } from '../../api/types'
import DataTable, { type Column } from '../common/DataTable.vue'
import ContextMenu, { type MenuItem } from '../common/ContextMenu.vue'
import Icon from '../common/Icon.vue'
import { formatBytes, formatCount, formatServerTime, textMatcher } from '../../utils/format'
import { kindLabels, useObjectActions } from '../../composables/objectActions'

const props = defineProps<{ tab: Tab }>()
const tabs = useTabsStore()
const conns = useConnectionsStore()
const meta = useMetadataStore()
const ui = useUiStore()
const actions = useObjectActions()

const conn = computed(() => props.tab.connectionId ?? '')
const db = computed(() => props.tab.database)
const filter = ref('')
const loading = ref(false)
const error = ref('')
const details = ref(false)
const selected = ref(new Set<string>())
const menu = ref<{ x: number; y: number; items: MenuItem[] } | null>(null)

interface ObjRow {
  key: string
  kind: ObjectKind
  name: string
  rows?: number
  size?: number
  dataLength?: number
  indexLength?: number
  dataFree?: number
  created?: string
  updated?: string
  engine?: string
  comment?: string
  collation?: string
  autoIncrement?: string
  rowFormat?: string
  avgRowLength?: number
  checkTime?: string
  createOptions?: string
  info?: string
}

const status = computed(() => meta.entry<TableStatus[]>(conn.value, 'status', db.value)?.data ?? [])
const routines = computed(() => meta.entry<RoutineDetail[]>(conn.value, 'rdetails', db.value)?.data ?? [])
const triggers = computed(() => meta.entry<TriggerInfo[]>(conn.value, 'triggers', db.value)?.data ?? [])
const events = computed(() => meta.entry<EventInfo[]>(conn.value, 'events', db.value)?.data ?? [])

async function load(force = true) {
  if (conns.state(conn.value).status !== 'connected' && !(await conns.connect(conn.value))) return
  loading.value = true
  error.value = ''
  const results = await Promise.allSettled([
    meta.tableStatus(conn.value, db.value, force), meta.routineDetails(conn.value, db.value, force),
    meta.triggers(conn.value, db.value, force), meta.events(conn.value, db.value, force),
  ])
  const failed = results.find((r) => r.status === 'rejected') as PromiseRejectedResult | undefined
  if (failed) error.value = toAppError(failed.reason).message
  loading.value = false
}
watch(() => [conn.value, db.value], () => void load(false), { immediate: true })

const all = computed<ObjRow[]>(() => {
  const out: ObjRow[] = []
  for (const t of status.value) {
    const kind: ObjectKind = t.type === 'view' ? 'view' : 'table'
    const size = t.dataLength !== undefined || t.indexLength !== undefined ? (t.dataLength ?? 0) + (t.indexLength ?? 0) : undefined
    out.push({
      key: `${kind}:${t.name}`, kind, name: t.name, rows: kind === 'table' ? t.rows : undefined, size, dataLength: t.dataLength, indexLength: t.indexLength,
      dataFree: t.dataFree, created: t.createTime, updated: t.updateTime, engine: t.engine, comment: t.comment, collation: t.collation,
      autoIncrement: t.autoIncrement, rowFormat: t.rowFormat, avgRowLength: t.avgRowLength, checkTime: t.checkTime, createOptions: t.createOptions,
    })
  }
  for (const r of routines.value) {
    const kind: ObjectKind = r.type === 'FUNCTION' ? 'function' : 'procedure'
    const params = (r.params ?? []).map((p) => `${p.mode ? p.mode + ' ' : ''}${p.name} ${p.type}`).join(', ')
    out.push({ key: `${kind}:${r.name}`, kind, name: r.name, created: r.created, updated: r.modified, comment: r.comment,
      info: `(${params})${r.returns ? ' RETURNS ' + r.returns : ''}` })
  }
  for (const t of triggers.value) {
    out.push({ key: `trigger:${t.name}`, kind: 'trigger', name: t.name, created: t.created, info: `${t.timing} ${t.event} em ${t.table}` })
  }
  for (const e of events.value) {
    out.push({ key: `event:${e.name}`, kind: 'event', name: e.name, created: e.created, comment: e.comment,
      info: `${e.status} · ${e.type === 'RECURRING' ? `a cada ${e.intervalValue} ${e.intervalField}` : `em ${e.executeAt}`}` })
  }
  return out
})

const kindFilter = ref<'' | ObjectKind>('')
const rows = computed(() => {
  const m = textMatcher(filter.value)
  return all.value.filter((r) => (!kindFilter.value || r.kind === kindFilter.value) && (m(r.name) || m(r.comment ?? '')))
})
const maxSize = computed(() => Math.max(1, ...all.value.map((r) => r.size ?? 0)))
const maxRows = computed(() => Math.max(1, ...all.value.map((r) => r.rows ?? 0)))
const totals = computed(() => {
  const count = (k: ObjectKind) => all.value.filter((r) => r.kind === k).length
  return {
    size: all.value.reduce((a, r) => a + (r.size ?? 0), 0), tables: count('table'), views: count('view'),
    procedures: count('procedure'), functions: count('function'), triggers: count('trigger'), events: count('event'),
  }
})

const baseColumns: Column<ObjRow>[] = [
  { key: 'name', label: 'Nome', width: 240 },
  { key: 'kind', label: 'Tipo', width: 90, text: (r) => kindLabels[r.kind] },
  { key: 'rows', label: 'Linhas (≈)', width: 100, align: 'right', text: (r) => formatCount(r.rows), bar: (r) => (r.rows ?? 0) / maxRows.value },
  { key: 'size', label: 'Tamanho', width: 100, align: 'right', text: (r) => formatBytes(r.size), bar: (r) => (r.size ?? 0) / maxSize.value },
  { key: 'created', label: 'Criado', width: 150, text: (r) => formatServerTime(r.created) },
  { key: 'updated', label: 'Atualizado', width: 150, text: (r) => formatServerTime(r.updated) },
  { key: 'engine', label: 'Engine', width: 80 },
  { key: 'comment', label: 'Comentário / detalhe', width: 300, text: (r) => r.comment || r.info || '' },
]
const extraColumns: Column<ObjRow>[] = [
  { key: 'collation', label: 'Collation', width: 170 },
  { key: 'autoIncrement', label: 'Auto incr.', width: 90, align: 'right', sort: (r) => (r.autoIncrement ? Number(r.autoIncrement) : undefined) },
  { key: 'rowFormat', label: 'Formato', width: 90 },
  { key: 'dataLength', label: 'Dados', width: 90, align: 'right', text: (r) => formatBytes(r.dataLength) },
  { key: 'indexLength', label: 'Índices', width: 90, align: 'right', text: (r) => formatBytes(r.indexLength) },
  { key: 'dataFree', label: 'Livre', width: 90, align: 'right', text: (r) => formatBytes(r.dataFree), cls: (r) => ((r.dataFree ?? 0) > (r.size ?? 0) * 0.3 && (r.dataFree ?? 0) > 10 << 20 ? 'warn-cell' : undefined) },
  { key: 'avgRowLength', label: 'Média/linha', width: 90, align: 'right', text: (r) => formatBytes(r.avgRowLength) },
  { key: 'checkTime', label: 'Verificado', width: 150, text: (r) => formatServerTime(r.checkTime) },
  { key: 'createOptions', label: 'Opções', width: 160 },
]
const columns = computed(() => (details.value ? [...baseColumns, ...extraColumns] : baseColumns))

function ref_(r: ObjRow): ObjectRef {
  return { kind: r.kind, database: db.value, name: r.name }
}

function newObject(kind: ObjectKind) {
  actions.openNew(conn.value, db.value, kind)
}

const newItems = (): MenuItem[] => (['table', 'view', 'procedure', 'function', 'trigger', 'event'] as ObjectKind[]).map((k) => ({
  label: `${kindLabels[k][0]!.toUpperCase()}${kindLabels[k].slice(1)}…`, action: () => newObject(k),
}))

function openMenu(e: MouseEvent, row: ObjRow | null, sel: ObjRow[]) {
  const tables = sel.filter((r) => r.kind === 'table')
  const items: MenuItem[] = []
  if (row) {
    items.push({ label: 'Abrir', icon: 'edit', action: () => actions.open(conn.value, ref_(row)), shortcut: 'Enter' })
    if (row.kind === 'table' || row.kind === 'view') {
      items.push({ label: 'Ver dados', icon: 'table', action: () => actions.open(conn.value, ref_(row), 'data') })
      items.push({ label: 'Renomear…', action: async () => { if (await actions.rename(conn.value, ref_(row))) await load() } })
    }
    if (row.kind === 'event') items.push({ label: 'Renomear…', action: async () => { if (await actions.rename(conn.value, ref_(row))) await load() } })
    items.push({ label: 'Copiar código CREATE', icon: 'copy', action: () => actions.copyDdl(conn.value, ref_(row)) })
    items.push({ label: 'Código CREATE em nova aba', action: () => actions.ddlInEditor(conn.value, ref_(row)) })
    items.push({ separator: true, label: '' })
  }
  items.push(...newItems().map((i) => ({ ...i, label: 'Novo(a) ' + i.label.toLowerCase() })))
  items.push({ separator: true, label: '' })
  items.push({ label: sel.length ? `Exportar ${sel.length} objeto(s) como SQL…` : 'Exportar database como SQL…', icon: 'download',
    action: () => (ui.dumpDialog = { connectionId: conn.value, databases: [db.value], tables: sel.filter((r) => r.kind === 'table' || r.kind === 'view').map((r) => r.name) }) })
  items.push({ label: 'Importar CSV…', icon: 'upload', action: () => (ui.csvDialog = { connectionId: conn.value, database: db.value, table: row?.kind === 'table' ? row.name : undefined }) })
  items.push({ label: 'Executar arquivo SQL…', action: () => actions.runSqlFile(conn.value, db.value) })
  const refs = tables.map((t) => ({ database: db.value, table: t.name }))
  const tool = (tab: 'maintenance' | 'search' | 'bulk') => () => (ui.toolsDialog = { connectionId: conn.value, databases: refs.length ? [] : [db.value], tables: refs.length ? refs : undefined, tab })
  items.push({ label: 'Ferramentas', icon: 'wrench', children: [
    { label: 'Manutenção (CHECK, OPTIMIZE…)…', action: tool('maintenance') },
    { label: 'Buscar texto…', icon: 'search', action: tool('search') },
    { label: 'Edição em massa…', action: tool('bulk') },
    { label: 'Copiar tabela…', icon: 'copy', disabled: row?.kind !== 'table', action: () => row && (ui.copyTableDialog = { connectionId: conn.value, database: db.value, table: row.name }) },
    { label: 'Gerar dados de teste…', disabled: row?.kind !== 'table', action: () => row && (ui.generateDialog = { connectionId: conn.value, database: db.value, table: row.name }) },
    { label: 'Comparar estruturas…', action: () => (ui.compareDialog = { connectionId: conn.value, database: db.value }) },
  ] })
  items.push({ separator: true, label: '' })
  items.push({ label: 'Copiar linhas selecionadas', icon: 'copy', disabled: !sel.length, action: () => table.value?.copySelection() })
  items.push({ label: 'Atualizar', icon: 'refresh', action: () => load(), shortcut: 'F5' })
  items.push({ separator: true, label: '' })
  items.push({ label: tables.length > 1 ? `Esvaziar ${tables.length} tabelas…` : 'Esvaziar tabela…', disabled: !tables.length, danger: true,
    action: async () => { if (await actions.truncate(conn.value, db.value, tables.map((t) => t.name))) await load() } })
  items.push({ label: sel.length > 1 ? `Excluir ${sel.length} objetos…` : 'Excluir…', icon: 'trash', danger: true, disabled: !sel.length,
    action: async () => { if (await actions.drop(conn.value, sel.map(ref_))) { selected.value = new Set(); await load() } } })
  menu.value = { x: e.clientX, y: e.clientY, items }
}

const table = ref<{ copySelection: () => Promise<void>; selectedRows: () => ObjRow[] } | null>(null)

function onKey(e: KeyboardEvent) {
  if (e.key === 'F5') {
    e.preventDefault()
    void load()
  } else if (e.key === 'Delete' && selected.value.size) {
    e.preventDefault()
    const sel = all.value.filter((r) => selected.value.has(r.key))
    void actions.drop(conn.value, sel.map(ref_)).then((ok) => { if (ok) void load() })
  } else if (e.key === 'F2' && selected.value.size === 1) {
    const r = all.value.find((x) => selected.value.has(x.key))
    if (r && (r.kind === 'table' || r.kind === 'view' || r.kind === 'event')) void actions.rename(conn.value, ref_(r)).then((n) => { if (n) void load() })
  }
}

const newMenu = ref<{ x: number; y: number } | null>(null)
function showNew(e: MouseEvent) {
  const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
  newMenu.value = { x: r.left, y: r.bottom + 2 }
}
</script>

<template>
  <div class="dbv" @keydown="onKey">
    <header class="head">
      <Icon name="database" :size="16" />
      <strong>{{ db }}</strong>
      <span class="muted">
        {{ formatBytes(totals.size) }} · {{ totals.tables }} tabelas · {{ totals.views }} views · {{ totals.procedures }} procedures ·
        {{ totals.functions }} functions · {{ totals.triggers }} triggers · {{ totals.events }} eventos
      </span>
    </header>
    <div class="toolbar">
      <div class="search">
        <Icon name="search" :size="12" />
        <input v-model="filter" placeholder="Filtrar objetos (regex; ; = ou)" aria-label="Filtrar objetos" />
      </div>
      <select v-model="kindFilter" class="select compact" aria-label="Tipo">
        <option value="">Todos os tipos</option>
        <option v-for="(l, k) in kindLabels" v-show="k !== 'database'" :key="k" :value="k">{{ l }}</option>
      </select>
      <label class="check"><input v-model="details" type="checkbox" /> mais colunas</label>
      <button class="btn small" @click="showNew"><Icon name="plus" :size="12" /> Novo</button>
      <button class="btn small" @click="tabs.open({ connectionId: conn, database: db })"><Icon name="terminal" :size="12" /> Consulta</button>
      <span class="spacer" />
      <span v-if="selected.size > 1" class="muted sel">{{ selected.size }} selecionados</span>
      <span v-if="error" class="err ellipsis" :title="error">{{ error }}</span>
      <button class="icon-btn" title="Atualizar (F5)" aria-label="Atualizar" @click="load()"><span v-if="loading" class="spinner" /><Icon v-else name="refresh" :size="14" /></button>
    </div>
    <div class="content">
      <DataTable
        ref="table" v-model:selected="selected" :columns="columns" :rows="rows" :row-key="(r) => r.key" :loading="loading" sort-key="name"
        empty-text="Nenhum objeto neste database." @activate="(r) => actions.open(conn, ref_(r))" @menu="openMenu"
      />
    </div>
    <ContextMenu v-if="menu" :x="menu.x" :y="menu.y" :items="menu.items" @close="menu = null" />
    <ContextMenu v-if="newMenu" :x="newMenu.x" :y="newMenu.y" :items="newItems()" @close="newMenu = null" />
  </div>
</template>

<style scoped>
.dbv {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}
.head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border-bottom: 1px solid var(--border);
  background: var(--bg-elev);
  flex-wrap: wrap;
  flex: none;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 5px 8px;
  border-bottom: 1px solid var(--border);
  flex-wrap: wrap;
  flex: none;
}
.search {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 26px;
  padding: 0 8px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius);
  background: var(--bg-input);
  color: var(--text-faint);
  width: 260px;
}
.search input {
  flex: 1;
  min-width: 0;
  border: none;
  outline: none;
  background: transparent;
  color: var(--text);
  user-select: text;
}
.spacer {
  flex: 1;
}
.sel {
  font-size: 12px;
}
.err {
  color: var(--danger);
  max-width: 40%;
  font-size: 12px;
}
.content {
  flex: 1;
  min-height: 0;
}
</style>

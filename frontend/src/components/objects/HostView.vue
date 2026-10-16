<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import type { Tab } from '../../stores/tabs'
import { useTabsStore } from '../../stores/tabs'
import { useConnectionsStore } from '../../stores/connections'
import { useMetadataStore } from '../../stores/metadata'
import { useSettingsStore } from '../../stores/settings'
import { useUiStore } from '../../stores/ui'
import { backend } from '../../api/backend'
import { toAppError } from '../../api/errors'
import type { DatabaseSummary, Process, ServerVariable, StatusValue } from '../../api/types'
import DataTable, { type Column } from '../common/DataTable.vue'
import ContextMenu, { type MenuItem } from '../common/ContextMenu.vue'
import Icon from '../common/Icon.vue'
import { formatBytes, formatCount, formatServerTime, formatUptime, textMatcher } from '../../utils/format'
import { useObjectActions, withConfirmation } from '../../composables/objectActions'
import { useDdlOptions } from '../dialogs/ddl'

const props = defineProps<{ tab: Tab }>()
const tabs = useTabsStore()
const conns = useConnectionsStore()
const meta = useMetadataStore()
const settings = useSettingsStore()
const ui = useUiStore()
const actions = useObjectActions()

type View = 'databases' | 'variables' | 'status' | 'processes' | 'commands'
const views: { id: View; label: string }[] = [
  { id: 'databases', label: 'Databases' },
  { id: 'variables', label: 'Variáveis' },
  { id: 'status', label: 'Status' },
  { id: 'processes', label: 'Processos' },
  { id: 'commands', label: 'Estatísticas de comandos' },
]
const view = computed<View>(() => (views.some((v) => v.id === props.tab.view) ? (props.tab.view as View) : 'databases'))
function setView(v: View) {
  const t = tabs.find(props.tab.id)
  if (t) t.view = v
}

const conn = computed(() => props.tab.connectionId ?? '')
const profile = computed(() => conns.byId.get(conn.value))
const state = computed(() => conns.state(conn.value))
const filter = ref('')
const loading = ref(false)
const error = ref('')
const selected = ref(new Set<string>())
const menu = ref<{ x: number; y: number; items: MenuItem[] } | null>(null)

// FLUSH (HeidiSQL: Tools > Flush).
const flushes: [string, string][] = [
  ['PRIVILEGES', 'recarrega as tabelas de privilégios'], ['HOSTS', 'limpa o cache de hosts bloqueados'], ['TABLES', 'fecha as tabelas abertas'],
  ['STATUS', 'zera as variáveis de status da sessão'], ['LOGS', 'fecha e reabre os logs'], ['USER_RESOURCES', 'zera os contadores de limites por hora'],
  ['QUERY CACHE', 'desfragmenta o cache de consultas'],
]
function flushMenu(e: MouseEvent) {
  const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
  menu.value = { x: r.left, y: r.bottom + 2, items: flushes.map(([w, hint]) => ({ label: `FLUSH ${w}`, hint, action: () => flush(w) })) }
}
async function flush(what: string) {
  try {
    const ok = await withConfirmation((c) => backend.flush(conn.value, what, c).then(() => true))
    if (ok) ui.toast('success', `FLUSH ${what} executado.`)
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}

const summaries = ref<DatabaseSummary[]>([])
const variables = ref<ServerVariable[]>([])
const status = ref<StatusValue[]>([])
const processes = ref<Process[]>([])
const statusAt = ref(0)

const uptime = computed(() => Number(status.value.find((s) => s.name === 'Uptime')?.value ?? 0))
const ownConnections = computed(() => status.value.find((s) => s.name === 'Threads_connected')?.value)

async function load(v: View = view.value, quiet = false) {
  if (state.value.status !== 'connected') {
    if (!(await conns.connect(conn.value))) return
  }
  if (!quiet) loading.value = true
  error.value = ''
  try {
    switch (v) {
      case 'databases':
        summaries.value = await meta.summaries(conn.value, true)
        break
      case 'variables':
        variables.value = await backend.serverVariables(conn.value)
        break
      case 'status':
      case 'commands':
        status.value = await backend.serverStatus(conn.value)
        statusAt.value = Date.now()
        break
      case 'processes':
        processes.value = await backend.processList(conn.value)
        break
    }
  } catch (e) {
    error.value = toAppError(e).message
  } finally {
    loading.value = false
  }
}

watch(view, (v) => {
  selected.value = new Set()
  void load(v)
}, { immediate: true })
// Uptime/status for the header.
if (view.value !== 'status' && view.value !== 'commands') void backend.serverStatus(conn.value).then((s) => (status.value = s)).catch(() => undefined)

// ---- auto refresh (processes, status) ----
const interval = ref(0)
let timer: number | undefined
watch([interval, view], ([sec]) => {
  window.clearInterval(timer)
  if (sec > 0) timer = window.setInterval(() => { if (tabs.activeId === props.tab.id) void load(view.value, true) }, sec * 1000)
})
onBeforeUnmount(() => window.clearInterval(timer))

const match = computed(() => textMatcher(filter.value))

// ---- databases ----
const maxSize = computed(() => Math.max(1, ...summaries.value.map((d) => d.size)))
const dbRows = computed(() => summaries.value.filter((d) => (settings.prefs.showSystemSchemas || !d.system) && match.value(d.name)))
const dbColumns: Column<DatabaseSummary>[] = [
  { key: 'name', label: 'Database', width: 220 },
  { key: 'size', label: 'Tamanho', width: 110, align: 'right', text: (d) => formatBytes(d.size), bar: (d) => d.size / maxSize.value },
  { key: 'tables', label: 'Tabelas', width: 80, align: 'right', text: (d) => formatCount(d.tables) },
  { key: 'views', label: 'Views', width: 70, align: 'right', text: (d) => formatCount(d.views) },
  { key: 'lastUpdate', label: 'Última alteração', width: 160, text: (d) => formatServerTime(d.lastUpdate) },
  { key: 'charset', label: 'Charset', width: 100 },
  { key: 'collation', label: 'Collation padrão', width: 200 },
]

// ---- variables ----
const onlyDiff = ref(false)
const varRows = computed(() => variables.value.filter((v) => (match.value(v.name) || match.value(v.session ?? '') || match.value(v.global ?? '')) &&
  (!onlyDiff.value || (v.session !== undefined && v.global !== undefined && v.session !== v.global))))
const differs = (v: ServerVariable) => v.session !== undefined && v.global !== undefined && v.session !== v.global
const varColumns: Column<ServerVariable>[] = [
  { key: 'name', label: 'Variável', width: 300 },
  { key: 'session', label: 'Sessão', width: 260, mono: true, cls: (v) => (v.session === undefined ? 'muted-cell' : differs(v) ? 'diff-cell' : undefined), text: (v) => v.session ?? '—' },
  { key: 'global', label: 'Global', width: 260, mono: true, cls: (v) => (v.global === undefined ? 'muted-cell' : undefined), text: (v) => v.global ?? '—' },
]

// ---- status ----
interface StatusRow extends StatusValue { num: number | null; perHour: number | null; perSec: number | null }
const statusRows = computed<StatusRow[]>(() => status.value.filter((s) => match.value(s.name) || match.value(s.value)).map((s) => {
  const n = /^-?\d+(\.\d+)?$/.test(s.value) ? Number(s.value) : null
  const up = uptime.value
  return { ...s, num: n, perHour: n !== null && up > 0 && s.name !== 'Uptime' ? n / (up / 3600) : null, perSec: n !== null && up > 0 && s.name !== 'Uptime' ? n / up : null }
}))
const statusText = (s: StatusRow) => (s.num !== null && /^Bytes_|_bytes$/i.test(s.name) ? `${formatBytes(s.num)}` : s.name === 'Uptime' ? formatUptime(Number(s.value)) : s.num !== null ? formatCount(s.num) : s.value)
const statusColumns: Column<StatusRow>[] = [
  { key: 'name', label: 'Variável', width: 300 },
  { key: 'value', label: 'Valor', width: 200, align: 'right', mono: true, text: statusText, sort: (s) => s.num ?? s.value },
  { key: 'perHour', label: 'Média por hora', width: 140, align: 'right', text: (s) => formatCount(s.perHour, 1) },
  { key: 'perSec', label: 'Média por segundo', width: 140, align: 'right', text: (s) => formatCount(s.perSec, 2) },
]

// ---- command statistics ----
interface CmdRow { name: string; total: number; perHour: number; perSec: number; pct: number }
const cmdRows = computed<CmdRow[]>(() => {
  const coms = status.value.filter((s) => s.name.startsWith('Com_') && Number(s.value) > 0)
  const sum = coms.reduce((a, s) => a + Number(s.value), 0) || 1
  const up = uptime.value || 1
  return coms.map((s) => {
    const total = Number(s.value)
    return { name: s.name.slice(4).replace(/_/g, ' '), total, perHour: total / (up / 3600), perSec: total / up, pct: (total / sum) * 100 }
  }).filter((c) => match.value(c.name))
})
const maxPct = computed(() => Math.max(1, ...cmdRows.value.map((c) => c.pct)))
const cmdColumns: Column<CmdRow>[] = [
  { key: 'name', label: 'Comando', width: 220 },
  { key: 'total', label: 'Total', width: 120, align: 'right', text: (c) => formatCount(c.total) },
  { key: 'perHour', label: 'Média por hora', width: 140, align: 'right', text: (c) => formatCount(c.perHour, 1) },
  { key: 'perSec', label: 'Média por segundo', width: 140, align: 'right', text: (c) => formatCount(c.perSec, 2) },
  { key: 'pct', label: 'Percentual', width: 200, align: 'right', text: (c) => `${formatCount(c.pct, 2)} %`, bar: (c) => c.pct / maxPct.value },
]

// ---- processes ----
const hideSleep = ref(false)
const procRows = computed(() => processes.value.filter((p) => (!hideSleep.value || p.command !== 'Sleep') &&
  (match.value(p.user) || match.value(p.host) || match.value(p.db ?? '') || match.value(p.info ?? '') || match.value(p.state ?? '') || match.value(String(p.id)))))
const maxTime = computed(() => Math.max(60, ...processes.value.filter((p) => p.command !== 'Sleep' && p.command !== 'Daemon').map((p) => p.time)))
const procColumns: Column<Process>[] = [
  { key: 'id', label: 'ID', width: 80, align: 'right' },
  { key: 'user', label: 'Usuário', width: 120 },
  { key: 'host', label: 'Host', width: 160 },
  { key: 'db', label: 'DB', width: 120 },
  { key: 'command', label: 'Comando', width: 100, cls: (p) => (p.command === 'Sleep' ? 'muted-cell' : p.command === 'Query' ? 'diff-cell' : undefined) },
  { key: 'time', label: 'Tempo', width: 90, align: 'right', text: (p) => formatCount(p.time), bar: (p) => (p.command === 'Sleep' || p.command === 'Daemon' ? 0 : p.time / maxTime.value) },
  { key: 'state', label: 'Estado', width: 160 },
  { key: 'progress', label: '%', width: 60, align: 'right', text: (p) => (p.progress ? formatCount(p.progress, 1) : '') },
  { key: 'info', label: 'Instrução', width: 500, mono: true, text: (p) => (p.info ?? '').replace(/\s+/g, ' ') },
]
const focusedProcess = computed(() => processes.value.find((p) => selected.value.has(String(p.id))) ?? null)

async function kill(ids: number[], queryOnly: boolean) {
  if (!ids.length) return
  try {
    const res = await withConfirmation((c) => backend.killProcesses(conn.value, ids, queryOnly, c))
    if (!res) return
    // Like HeidiSQL, "no such thread" is not an error worth reporting.
    const failed = res.filter((r) => r.error && r.error.code !== 1094)
    if (failed.length) ui.toast('error', `${failed[0]!.error!.message} (${failed[0]!.sql})`)
    else ui.toast('success', queryOnly ? 'Instrução interrompida.' : `${ids.length === 1 ? 'Processo encerrado' : `${ids.length} processos encerrados`}.`)
    await load('processes', true)
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}

function explain(p: Process) {
  if (!p.info) return
  const sql = `EXPLAIN ${p.info.trim().replace(/;$/, '')};`
  const t = tabs.open({ title: `EXPLAIN #${p.id}`, connectionId: conn.value, database: p.db ?? '', sql })
  void tabs.run(t.id, { sql, cursorOffset: -1, mode: 'statement', baseLine: 1 })
}

// ---- actions ----
const { charsets } = useDdlOptions(() => conn.value)

async function alterDatabase(d: DatabaseSummary) {
  const choices = charsets.value.flatMap((c) => (c.collations ?? []).map((coll) => ({ value: `${c.name}|${coll}`, label: `${coll} (${c.name})` })))
  if (!choices.length) {
    ui.toast('warning', 'Lista de collations indisponível.')
    return
  }
  const r = await ui.prompt({ title: `Alterar database "${d.name}"`, label: 'Charset e collation padrão', value: `${d.charset}|${d.collation}`, choices, confirmLabel: 'Alterar',
    message: 'Afeta apenas tabelas criadas daqui em diante; as existentes mantêm suas collations.' })
  if (!r) return
  const [cs, coll] = r.value.split('|')
  try {
    const ok = await withConfirmation((c) => backend.alterDatabase(conn.value, d.name, cs ?? '', coll ?? '', c).then(() => true))
    if (ok) {
      ui.toast('success', `Database "${d.name}" alterado.`)
      await load('databases', true)
    }
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}

async function editVariable(v: ServerVariable) {
  const cur = v.global ?? v.session ?? ''
  const isBool = /^(ON|OFF)$/i.test(cur)
  const r = await ui.prompt({
    title: `Alterar ${v.name} (GLOBAL)`,
    label: 'Novo valor global',
    value: isBool ? cur.toUpperCase() : cur,
    choices: isBool ? [{ value: 'ON', label: 'ON' }, { value: 'OFF', label: 'OFF' }] : undefined,
    message: `Sessão: ${v.session ?? '—'}\nGlobal: ${v.global ?? '—'}\n\nO valor global vale para novas conexões de todos os clientes e se perde ao reiniciar o servidor (use SET PERSIST no editor para torná-lo permanente no MySQL 8).`,
    checkbox: isBool ? undefined : { label: 'Tratar como expressão SQL (sem aspas)', value: false },
    confirmLabel: 'Aplicar',
  })
  if (!r) return
  try {
    const ok = await withConfirmation((c) => backend.setVariable(conn.value, true, v.name, r.value, r.checked ? 'raw' : 'auto', c).then(() => true))
    if (ok) {
      ui.toast('success', `${v.name} alterada.`)
      await load('variables', true)
    }
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}

function sessionSetInTab(v: ServerVariable) {
  const val = v.session ?? v.global ?? ''
  const lit = /^-?\d+(\.\d+)?$/.test(val) || /^(ON|OFF)$/i.test(val) ? val : `'${val.replace(/\\/g, '\\\\').replace(/'/g, "''")}'`
  tabs.open({ connectionId: conn.value, sql: `SET SESSION ${v.name} = ${lit};\n` })
}

function copy(text: string) {
  void navigator.clipboard.writeText(text).then(() => ui.toast('success', 'Copiado.', 1500))
}

function openMenu(e: MouseEvent, row: unknown, sel: unknown[]) {
  let items: MenuItem[]
  const refresh: MenuItem = { label: 'Atualizar', icon: 'refresh', action: () => load(), shortcut: 'F5' }
  switch (view.value) {
    case 'databases': {
      const d = row as DatabaseSummary | null
      const all = sel as DatabaseSummary[]
      items = [
        { label: 'Abrir', icon: 'database', disabled: !d, action: () => d && actions.open(conn.value, { kind: 'database', database: d.name, name: d.name }) },
        { label: 'Nova consulta neste database', icon: 'plus', disabled: !d, action: () => d && tabs.open({ connectionId: conn.value, database: d.name }) },
        { label: 'Novo database…', icon: 'plus', action: () => (ui.ddlDialog = { kind: 'database', connectionId: conn.value, onCreated: () => void load('databases', true) }) },
        { label: 'Alterar charset/collation…', icon: 'edit', disabled: !d, action: () => d && alterDatabase(d) },
        { label: 'Copiar CREATE DATABASE', icon: 'copy', disabled: !d, action: () => d && actions.copyDdl(conn.value, { kind: 'database', database: '', name: d.name }) },
        { label: all.length > 1 ? `Exportar ${all.length} databases como SQL…` : 'Exportar como SQL…', icon: 'download', action: () => (ui.dumpDialog = { connectionId: conn.value, databases: all.map((x) => x.name) }) },
        { label: 'Executar arquivo SQL…', icon: 'upload', action: () => actions.runSqlFile(conn.value, d?.name ?? '') },
        { label: 'Ferramentas', icon: 'wrench', disabled: !all.length, children: [
          { label: 'Manutenção (CHECK, OPTIMIZE…)…', action: () => (ui.toolsDialog = { connectionId: conn.value, databases: all.map((x) => x.name), tab: 'maintenance' }) },
          { label: 'Buscar texto…', icon: 'search', action: () => (ui.toolsDialog = { connectionId: conn.value, databases: all.map((x) => x.name), tab: 'search' }) },
          { label: 'Comparar estruturas…', action: () => (ui.compareDialog = { connectionId: conn.value, database: d?.name ?? '' }) },
        ] },
        { separator: true, label: '' },
        refresh,
        { separator: true, label: '' },
        { label: all.length > 1 ? `Excluir ${all.length} databases…` : 'Excluir database…', icon: 'trash', danger: true, disabled: !all.length,
          action: async () => { if (await actions.drop(conn.value, all.map((x) => ({ kind: 'database', database: '', name: x.name })))) await load('databases', true) } },
      ]
      break
    }
    case 'variables': {
      const v = row as ServerVariable | null
      items = [
        { label: 'Alterar valor global…', icon: 'edit', disabled: !v || v.global === undefined, action: () => v && editVariable(v) },
        { label: 'SET SESSION em nova aba', icon: 'plus', disabled: !v || v.session === undefined, action: () => v && sessionSetInTab(v) },
        { label: 'Copiar nome', icon: 'copy', disabled: !v, action: () => v && copy(v.name) },
        { label: 'Copiar valor', disabled: !v, action: () => v && copy(v.session ?? v.global ?? '') },
        { separator: true, label: '' },
        refresh,
      ]
      break
    }
    case 'processes': {
      const ids = (sel as Process[]).map((p) => p.id)
      const p = row as Process | null
      items = [
        { label: ids.length > 1 ? `Interromper instrução de ${ids.length} processos (KILL QUERY)…` : 'Interromper instrução (KILL QUERY)…', icon: 'stop', disabled: !ids.length, action: () => kill(ids, true) },
        { label: ids.length > 1 ? `Encerrar ${ids.length} conexões (KILL)…` : 'Encerrar conexão (KILL)…', icon: 'x', danger: true, disabled: !ids.length, action: () => kill(ids, false) },
        { separator: true, label: '' },
        { label: 'EXPLAIN da instrução', disabled: !p?.info, action: () => p && explain(p) },
        { label: 'Copiar instrução', icon: 'copy', disabled: !p?.info, action: () => p && copy(p.info ?? '') },
        { label: 'Abrir instrução em nova aba', disabled: !p?.info, action: () => p && tabs.open({ connectionId: conn.value, database: p.db ?? '', sql: (p.info ?? '') + ';\n' }) },
        { separator: true, label: '' },
        refresh,
      ]
      break
    }
    default:
      items = [{ label: 'Copiar linhas selecionadas', icon: 'copy', disabled: !sel.length, action: () => table.value?.copySelection() }, refresh]
  }
  menu.value = { x: e.clientX, y: e.clientY, items }
}

const table = ref<{ copySelection: () => Promise<void> } | null>(null)

function onKey(e: KeyboardEvent) {
  if (e.key === 'F5') {
    e.preventDefault()
    void load()
  }
}
</script>

<template>
  <div class="host" @keydown="onKey">
    <header class="head">
      <div class="title">
        <Icon name="server" :size="16" />
        <strong>{{ profile?.name }}</strong>
        <span v-if="state.server" class="muted">{{ state.server.flavor }} {{ state.server.version }} · {{ state.server.user }}</span>
        <span v-if="uptime" class="muted">· ativo há {{ formatUptime(uptime) }}</span>
        <span v-if="ownConnections" class="muted">· {{ ownConnections }} conexões</span>
      </div>
      <nav class="subtabs" role="tablist">
        <button v-for="v in views" :key="v.id" role="tab" :aria-selected="view === v.id" :class="{ active: view === v.id }" @click="setView(v.id)">
          {{ v.label }}
          <span v-if="v.id === 'databases' && summaries.length" class="n">{{ dbRows.length }}</span>
          <span v-if="v.id === 'processes' && processes.length" class="n">{{ processes.length }}</span>
        </button>
      </nav>
    </header>
    <div class="toolbar">
      <div class="search">
        <Icon name="search" :size="12" />
        <input v-model="filter" placeholder="Filtrar (regex; use ; para alternativas)" aria-label="Filtrar" />
      </div>
      <label v-if="view === 'variables'" class="check"><input v-model="onlyDiff" type="checkbox" /> sessão ≠ global</label>
      <label v-if="view === 'processes'" class="check"><input v-model="hideSleep" type="checkbox" /> ocultar ociosos</label>
      <template v-if="view === 'processes'">
        <button class="btn small" :disabled="!selected.size" @click="kill([...selected].map(Number), true)"><Icon name="stop" :size="10" /> KILL QUERY</button>
        <button class="btn small" :disabled="!selected.size" @click="kill([...selected].map(Number), false)"><Icon name="x" :size="11" /> KILL</button>
      </template>
      <span class="spacer" />
      <span v-if="error" class="err ellipsis" :title="error">{{ error }}</span>
      <label v-if="view === 'processes' || view === 'status' || view === 'commands'" class="auto">
        Atualizar a cada
        <select v-model.number="interval" class="select compact">
          <option :value="0">—</option>
          <option :value="1">1 s</option>
          <option :value="2">2 s</option>
          <option :value="5">5 s</option>
          <option :value="10">10 s</option>
          <option :value="30">30 s</option>
        </select>
      </label>
      <button class="btn small" title="Gerenciar usuários e privilégios" @click="tabs.openObject({ kind: 'users', connectionId: conn })"><Icon name="user" :size="11" /> Usuários</button>
      <button class="btn small" title="FLUSH" aria-haspopup="menu" @click="flushMenu"><Icon name="zap" :size="11" /> Flush</button>
      <button class="icon-btn" title="Atualizar (F5)" aria-label="Atualizar" @click="load()"><span v-if="loading" class="spinner" /><Icon v-else name="refresh" :size="14" /></button>
    </div>
    <div class="content">
      <DataTable
        v-if="view === 'databases'" ref="table" v-model:selected="selected" :columns="dbColumns" :rows="dbRows" :row-key="(d) => d.name" :loading="loading"
        sort-key="name" @activate="(d) => actions.open(conn, { kind: 'database', database: d.name, name: d.name })" @menu="openMenu"
      />
      <DataTable
        v-else-if="view === 'variables'" ref="table" v-model:selected="selected" :columns="varColumns" :rows="varRows" :row-key="(v) => v.name" :loading="loading"
        sort-key="name" @activate="editVariable" @menu="openMenu"
      />
      <DataTable
        v-else-if="view === 'status'" ref="table" v-model:selected="selected" :columns="statusColumns" :rows="statusRows" :row-key="(s) => s.name" :loading="loading"
        sort-key="name" @menu="openMenu"
      />
      <DataTable
        v-else-if="view === 'commands'" ref="table" v-model:selected="selected" :columns="cmdColumns" :rows="cmdRows" :row-key="(c) => c.name" :loading="loading"
        sort-key="total" :sort-desc="true" @menu="openMenu"
      />
      <div v-else class="procs">
        <DataTable
          ref="table" v-model:selected="selected" class="ptable" :columns="procColumns" :rows="procRows" :row-key="(p) => String(p.id)" :loading="loading"
          sort-key="id" @menu="openMenu"
        />
        <div v-if="focusedProcess?.info" class="pinfo">
          <div class="pinfo-head">
            <span class="muted">Processo {{ focusedProcess.id }} · {{ focusedProcess.user }}@{{ focusedProcess.host }} · {{ focusedProcess.db || 'sem database' }}</span>
            <button class="btn small" @click="explain(focusedProcess)">EXPLAIN</button>
          </div>
          <pre class="mono selectable">{{ focusedProcess.info }}</pre>
        </div>
      </div>
    </div>
    <ContextMenu v-if="menu" :x="menu.x" :y="menu.y" :items="menu.items" @close="menu = null" />
  </div>
</template>

<style scoped>
.host {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}
.head {
  padding: 8px 10px 0;
  border-bottom: 1px solid var(--border);
  background: var(--bg-elev);
  flex: none;
}
.title {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 6px;
}
.subtabs {
  display: flex;
  gap: 2px;
  overflow-x: auto;
}
.subtabs button {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 12px;
  border: none;
  border-bottom: 2px solid transparent;
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  white-space: nowrap;
}
.subtabs button.active {
  color: var(--text);
  border-bottom-color: var(--accent);
}
.n {
  font-size: 10px;
  padding: 0 5px;
  border-radius: 8px;
  background: var(--bg-active);
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 5px 8px;
  border-bottom: 1px solid var(--border);
  flex: none;
  flex-wrap: wrap;
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
  width: 300px;
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
.auto {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--text-muted);
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
.procs {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.ptable {
  flex: 1;
  min-height: 0;
}
.pinfo {
  flex: none;
  max-height: 35%;
  display: flex;
  flex-direction: column;
  border-top: 1px solid var(--border-strong);
}
.pinfo-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 8px;
  font-size: 12px;
}
.pinfo pre {
  margin: 0;
  padding: 6px 10px;
  overflow: auto;
  white-space: pre-wrap;
  font-size: 12px;
}
</style>

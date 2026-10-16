<script setup lang="ts">
// HeidiSQL "Table tools": maintenance, find text on server and the bulk
// table editor over a selection of databases and tables.
import { computed, ref, watch } from 'vue'
import Modal from '../common/Modal.vue'
import Icon from '../common/Icon.vue'
import DataTable, { type Column } from '../common/DataTable.vue'
import { useUiStore } from '../../stores/ui'
import { useTasksStore } from '../../stores/tasks'
import { useConnectionsStore } from '../../stores/connections'
import { useMetadataStore } from '../../stores/metadata'
import { useSettingsStore } from '../../stores/settings'
import { useTabsStore } from '../../stores/tabs'
import { backend } from '../../api/backend'
import { toAppError } from '../../api/errors'
import type { BulkSpec, DatabaseInfo, MaintenanceRow, SearchHit, SearchSpec, TableInfo, TableRef } from '../../api/types'
import { reportResults, withConfirmation } from '../../composables/objectActions'
import { useDdlOptions } from './ddl'

const ui = useUiStore()
const tasks = useTasksStore()
const conns = useConnectionsStore()
const meta = useMetadataStore()
const settings = useSettingsStore()
const tabs = useTabsStore()
const st = computed(() => ui.toolsDialog)
const conn = computed(() => st.value?.connectionId ?? '')
const tab = ref<'maintenance' | 'search' | 'bulk'>('maintenance')
const { options: ddl, charsets, collationsOf } = useDdlOptions(() => conn.value)

interface DbSel { name: string; checked: boolean; open: boolean; tables: Set<string> }
const dbs = ref<DbSel[]>([])
const allDbs = ref<string[]>([])

watch(st, async (s) => {
  if (!s) return
  tab.value = s.tab
  taskId.value = ''
  bulkSql.value = []
  const list = await meta.databases(s.connectionId).catch(() => [] as DatabaseInfo[])
  allDbs.value = list.map((d) => d.name)
  const wanted = new Set([...s.databases, ...(s.tables ?? []).map((t) => t.database)])
  dbs.value = list.filter((d) => settings.prefs.showSystemSchemas || !d.system || wanted.has(d.name)).map((d) => ({
    name: d.name, checked: wanted.has(d.name), open: wanted.size === 1 && wanted.has(d.name), tables: new Set<string>(),
  }))
  for (const d of dbs.value.filter((x) => x.checked)) {
    await loadTables(d, (s.tables ?? []).filter((t) => t.database === d.name).map((t) => t.table))
  }
}, { immediate: true })

function tablesOf(d: DbSel): TableInfo[] {
  return (meta.entry<TableInfo[]>(conn.value, 'tables', d.name)?.data ?? []).filter((t) => t.type === 'table' || (tab.value === 'search' && t.type === 'view'))
}
async function loadTables(d: DbSel, only?: string[]) {
  await meta.tables(conn.value, d.name).catch(() => undefined)
  const list = tablesOf(d)
  d.tables = new Set(only?.length ? only : list.map((t) => t.name))
}
async function toggleDb(d: DbSel) {
  d.checked = !d.checked
  if (d.checked && !d.tables.size) await loadTables(d)
}
function toggleTable(d: DbSel, t: string) {
  const s = new Set(d.tables)
  if (s.has(t)) s.delete(t)
  else s.add(t)
  d.tables = s
}
// Views only take part in the text search: entering it with every table of
// a database selected selects its views too.
watch(tab, (t) => {
  if (t !== 'search') return
  for (const d of dbs.value.filter((x) => x.checked)) {
    const all = meta.entry<TableInfo[]>(conn.value, 'tables', d.name)?.data ?? []
    if (all.filter((x) => x.type === 'table').every((x) => d.tables.has(x.name))) d.tables = new Set([...d.tables, ...all.filter((x) => x.type === 'view').map((x) => x.name)])
  }
})
const selectedTables = computed<TableRef[]>(() => dbs.value.filter((d) => d.checked).flatMap((d) => [...d.tables].filter((t) => tablesOf(d).some((x) => x.name === t)).map((t) => ({ database: d.name, table: t }))))

// ---- task output ----
const taskId = ref('')
const task = computed(() => tasks.tasks.find((t) => t.id === taskId.value))
const running = computed(() => task.value?.status === 'running')

// ---- maintenance ----
const ops = ['CHECK', 'ANALYZE', 'CHECKSUM', 'OPTIMIZE', 'REPAIR'] as const
const op = ref<(typeof ops)[number]>('CHECK')
const opOptions: Record<string, string[]> = {
  CHECK: ['QUICK', 'FAST', 'MEDIUM', 'EXTENDED', 'CHANGED', 'FOR UPGRADE'],
  ANALYZE: [], CHECKSUM: ['QUICK', 'EXTENDED'], OPTIMIZE: [], REPAIR: ['QUICK', 'EXTENDED', 'USE_FRM'],
}
const opHelp: Record<string, string> = {
  CHECK: 'Verifica erros nas tabelas.',
  ANALYZE: 'Atualiza as estatísticas de distribuição das chaves (usadas pelo otimizador).',
  CHECKSUM: 'Calcula a soma de verificação do conteúdo (compara cópias de tabelas).',
  OPTIMIZE: 'Reorganiza o armazenamento e desfragmenta (reconstrói a tabela no InnoDB).',
  REPAIR: 'Repara tabelas corrompidas (MyISAM, Aria, ARCHIVE, CSV).',
}
const chosen = ref(new Set<string>())
watch(op, () => (chosen.value = new Set()))
function toggleOpt(o: string) {
  const s = new Set(chosen.value)
  if (s.has(o)) s.delete(o)
  else {
    if (op.value === 'CHECKSUM') s.clear()
    s.add(o)
  }
  chosen.value = s
}
const maintRows = computed(() => (task.value?.kind === 'maintenance' ? ((task.value.data as MaintenanceRow[] | undefined) ?? []) : []))
const maintCols: Column<MaintenanceRow>[] = [
  { key: 'table', label: 'Tabela', width: 220 },
  { key: 'op', label: 'Operação', width: 90 },
  { key: 'msgType', label: 'Tipo', width: 80, cls: (r) => (/error/i.test(r.msgType) ? 'bad' : /warning/i.test(r.msgType) ? 'warn' : undefined) },
  { key: 'message', label: 'Mensagem', width: 320 },
]
async function runMaintenance() {
  if (!selectedTables.value.length) return ui.toast('warning', 'Selecione as tabelas.')
  try {
    const id = await withConfirmation((c) => backend.startMaintenance(conn.value, op.value, [...chosen.value], selectedTables.value, c))
    if (id) {
      taskId.value = id
      tasks.track(id)
    }
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}

// ---- find text ----
const search = ref<SearchSpec>({ text: '', match: 'contains', caseSensitive: false, types: 'all' })
const hits = computed(() => (task.value?.kind === 'search' ? ((task.value.data as SearchHit[] | undefined) ?? []) : []))
const hitCols: Column<SearchHit>[] = [
  { key: 'database', label: 'Database', width: 160 },
  { key: 'table', label: 'Tabela', width: 200 },
  { key: 'rows', label: 'Linhas encontradas', width: 130, align: 'right', text: (h) => h.rows.toLocaleString() },
]
async function runSearch() {
  const databases: string[] = []
  const tables: TableRef[] = []
  for (const d of dbs.value.filter((x) => x.checked)) {
    const all = tablesOf(d)
    if (all.length && all.every((t) => d.tables.has(t.name))) databases.push(d.name)
    else tables.push(...[...d.tables].map((t) => ({ database: d.name, table: t })))
  }
  if (!databases.length && !tables.length) return ui.toast('warning', 'Selecione databases ou tabelas.')
  try {
    taskId.value = await backend.startSearch(conn.value, databases, tables, search.value)
    tasks.track(taskId.value)
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}
function openHit(h: SearchHit) {
  tabs.openObject({ kind: 'table', connectionId: conn.value, database: h.database, objectName: h.table, view: 'data', dataFilter: h.filter })
  ui.toolsDialog = null
}

// ---- bulk editor ----
const bulk = ref<BulkSpec>({ moveTo: '', engine: '', convertCharset: '', convertCollation: '', defaultCollation: '', resetAutoIncrement: false })
const bulkSql = ref<string[]>([])
const bulkError = ref('')
const allCollations = computed(() => charsets.value.flatMap((c) => c.collations ?? []).sort())
watch([bulk, selectedTables], async () => {
  bulkError.value = ''
  bulkSql.value = []
  if (tab.value !== 'bulk' || !selectedTables.value.length) return
  try {
    bulkSql.value = await backend.previewBulkEdit(conn.value, selectedTables.value, bulk.value)
  } catch (e) {
    bulkError.value = toAppError(e).message
  }
}, { deep: true })
const busy = ref(false)
async function applyBulk() {
  busy.value = true
  try {
    const res = await withConfirmation((c) => backend.applyBulkEdit(conn.value, selectedTables.value, bulk.value, c))
    if (!res) return
    reportResults(res, `${selectedTables.value.length} tabela(s) alterada(s).`)
    for (const d of new Set(selectedTables.value.map((t) => t.database))) await meta.invalidate(conn.value, d)
    if (bulk.value.moveTo) await meta.invalidate(conn.value, bulk.value.moveTo)
    ui.toolsDialog = null
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Modal v-if="st" :title="`Ferramentas de tabela — ${conns.byId.get(conn)?.name ?? ''}`" :width="960" :close-on-backdrop="false" @close="ui.toolsDialog = null">
    <div class="tools">
      <div class="objects">
        <div class="lbl">Databases e tabelas ({{ selectedTables.length }})</div>
        <div class="tree">
          <div v-for="d in dbs" :key="d.name">
            <div class="row">
              <button class="icon-btn mini" :aria-label="d.open ? 'Recolher' : 'Expandir'" @click="d.open = !d.open; d.open && !tablesOf(d).length && loadTables(d)">
                <Icon :name="d.open ? 'chevronDown' : 'chevronRight'" :size="11" />
              </button>
              <label class="check"><input type="checkbox" :checked="d.checked" @change="toggleDb(d)" /> <Icon name="database" :size="12" /> {{ d.name }}</label>
              <span v-if="d.checked" class="faint small">{{ d.tables.size }} de {{ tablesOf(d).length }}</span>
            </div>
            <div v-if="d.open" class="kids">
              <div class="row small">
                <button class="linkbtn" @click="d.tables = new Set(tablesOf(d).map((t) => t.name))">todas</button>
                <button class="linkbtn" @click="d.tables = new Set()">nenhuma</button>
              </div>
              <label v-for="t in tablesOf(d)" :key="t.name" class="check small"><input type="checkbox" :checked="d.tables.has(t.name)" :disabled="!d.checked" @change="toggleTable(d, t.name)" /> <Icon :name="t.type === 'table' ? 'table' : 'view'" :size="11" /> {{ t.name }}</label>
            </div>
          </div>
        </div>
      </div>
      <div class="main">
        <div class="tabs" role="tablist">
          <button v-for="t in [['maintenance', 'Manutenção'], ['search', 'Buscar texto'], ['bulk', 'Edição em massa']] as const" :key="t[0]" role="tab" :aria-selected="tab === t[0]" :class="{ on: tab === t[0] }" @click="tab = t[0]">{{ t[1] }}</button>
        </div>

        <div v-if="tab === 'maintenance'" class="pane">
          <div class="row wrap">
            <label v-for="o in ops" :key="o" class="check"><input v-model="op" type="radio" :value="o" /> {{ o }}</label>
          </div>
          <p class="faint small">{{ opHelp[op] }}</p>
          <div v-if="opOptions[op]!.length" class="row wrap">
            <span class="small">Opções:</span>
            <label v-for="o in opOptions[op]" :key="o" class="check small"><input type="checkbox" :checked="chosen.has(o)" @change="toggleOpt(o)" /> {{ o }}</label>
          </div>
          <DataTable class="grow" :columns="maintCols" :rows="maintRows" :row-key="(r) => r.table + r.op + r.msgType + r.message" :loading="running" empty-text="Execute para ver as mensagens do servidor." />
        </div>

        <div v-else-if="tab === 'search'" class="pane">
          <div class="row">
            <input v-model="search.text" class="input grow-x" placeholder="Texto a procurar" @keydown.enter="runSearch" />
          </div>
          <div class="row wrap">
            <select v-model="search.match" class="select" aria-label="Tipo de busca">
              <option value="contains">contém</option>
              <option value="exact">igual a</option>
              <option value="starts">começa com</option>
              <option value="ends">termina com</option>
              <option value="regex">expressão regular</option>
            </select>
            <select v-model="search.types" class="select" aria-label="Tipos de coluna">
              <option value="all">todas as colunas</option>
              <option value="text">texto</option>
              <option value="numeric">numéricas</option>
              <option value="date">data e hora</option>
              <option value="binary">binárias</option>
            </select>
            <label class="check"><input v-model="search.caseSensitive" type="checkbox" /> diferenciar maiúsculas</label>
          </div>
          <p class="faint small">Em colunas numéricas só há igualdade. Duplo clique abre a tabela filtrada.</p>
          <DataTable class="grow" :columns="hitCols" :rows="hits" :row-key="(h) => h.database + '.' + h.table" :loading="running" empty-text="Nenhuma ocorrência." @activate="openHit" />
          <div v-if="task && task.kind === 'search'" class="faint small">{{ task.status === 'running' ? task.message : task.result }}</div>
        </div>

        <div v-else class="pane">
          <div class="grid2">
            <label class="field"><span>Mover para o database</span>
              <select v-model="bulk.moveTo" class="select"><option value="">(não mover)</option><option v-for="d in allDbs" :key="d" :value="d">{{ d }}</option></select>
            </label>
            <label class="field"><span>Engine</span>
              <select v-model="bulk.engine" class="select"><option value="">(manter)</option><option v-for="e in ddl?.engines ?? []" :key="e" :value="e">{{ e }}</option></select>
            </label>
            <label class="field"><span>Converter dados para o charset</span>
              <select v-model="bulk.convertCharset" class="select" @change="bulk.convertCollation = ''"><option value="">(não converter)</option><option v-for="c in charsets" :key="c.name" :value="c.name">{{ c.name }}</option></select>
            </label>
            <label class="field"><span>… com a collation</span>
              <select v-model="bulk.convertCollation" class="select" :disabled="!bulk.convertCharset"><option value="">(padrão do charset)</option><option v-for="c in collationsOf(bulk.convertCharset)" :key="c" :value="c">{{ c }}</option></select>
            </label>
            <label class="field"><span>Collation padrão (sem converter dados)</span>
              <select v-model="bulk.defaultCollation" class="select"><option value="">(manter)</option><option v-for="c in allCollations" :key="c" :value="c">{{ c }}</option></select>
            </label>
            <label class="check"><input v-model="bulk.resetAutoIncrement" type="checkbox" /> reiniciar AUTO_INCREMENT</label>
          </div>
          <div class="lbl">Instruções</div>
          <pre class="sql mono selectable grow">{{ bulkError || bulkSql.join(';\n') }}</pre>
        </div>
      </div>
    </div>
    <template #footer>
      <button class="btn" @click="ui.toolsDialog = null">Fechar</button>
      <button v-if="running" class="btn" @click="tasks.cancel(taskId)">Cancelar tarefa</button>
      <button v-if="tab === 'maintenance'" class="btn primary" :disabled="running || !selectedTables.length" @click="runMaintenance"><Icon name="wrench" :size="13" /> Executar {{ op }}</button>
      <button v-else-if="tab === 'search'" class="btn primary" :disabled="running || !search.text" @click="runSearch"><Icon name="search" :size="13" /> Buscar</button>
      <button v-else class="btn primary" :disabled="busy || !bulkSql.length" @click="applyBulk"><span v-if="busy" class="spinner" /> Aplicar</button>
    </template>
  </Modal>
</template>

<style scoped>
.tools {
  display: grid;
  grid-template-columns: 260px 1fr;
  gap: 12px;
  height: 480px;
}
.objects,
.main,
.pane {
  display: flex;
  flex-direction: column;
  min-height: 0;
  gap: 6px;
}
.pane {
  flex: 1;
}
.lbl {
  font-size: 12px;
  color: var(--text-muted);
}
.tree {
  flex: 1;
  overflow: auto;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 4px;
}
.row {
  display: flex;
  align-items: center;
  gap: 6px;
}
.wrap {
  flex-wrap: wrap;
}
.kids {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 2px 0 6px 28px;
}
.small {
  font-size: 12px;
}
.faint {
  color: var(--text-faint);
  margin: 0;
}
.mini {
  width: 20px;
  height: 20px;
}
.linkbtn {
  border: none;
  background: none;
  color: var(--accent);
  cursor: pointer;
  padding: 0 4px;
  font-size: 11px;
}
.tabs {
  display: flex;
  gap: 2px;
  border-bottom: 1px solid var(--border);
}
.tabs button {
  border: none;
  background: none;
  padding: 6px 12px;
  cursor: pointer;
  color: var(--text-muted);
  border-bottom: 2px solid transparent;
}
.tabs button.on {
  color: var(--text);
  border-bottom-color: var(--accent);
}
.grow {
  flex: 1;
  min-height: 0;
}
.grow-x {
  flex: 1;
}
.grid2 {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px 12px;
  align-items: end;
}
.sql {
  margin: 0;
  padding: 8px;
  overflow: auto;
  background: var(--bg-sunken);
  border-radius: var(--radius-sm);
  font-size: 12px;
  white-space: pre-wrap;
}
:deep(.bad) {
  color: var(--danger);
}
:deep(.warn) {
  color: var(--warning);
}
</style>

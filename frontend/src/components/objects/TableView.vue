<script setup lang="ts">
import { computed, defineAsyncComponent, ref, watch } from 'vue'
import type { Tab } from '../../stores/tabs'
import { useTabsStore } from '../../stores/tabs'
import { useConnectionsStore } from '../../stores/connections'
import { useMetadataStore } from '../../stores/metadata'
import { useUiStore } from '../../stores/ui'
import { backend } from '../../api/backend'
import { toAppError } from '../../api/errors'
import type { CheckInfo, ColumnInfo, ForeignKeyInfo, IndexInfo, ObjectRef, TableStatus, TriggerInfo } from '../../api/types'
import DataTable, { type Column } from '../common/DataTable.vue'
import CodeEditor from '../common/CodeEditor.vue'
import ContextMenu, { type MenuItem } from '../common/ContextMenu.vue'
import Icon from '../common/Icon.vue'
import { formatBytes, formatCount, formatServerTime } from '../../utils/format'
import { useObjectActions } from '../../composables/objectActions'
import { tableStatement, type StatementKind } from '../../composables/templates'

const TableData = defineAsyncComponent(() => import('./TableData.vue'))
const TableEditor = defineAsyncComponent(() => import('./TableEditor.vue'))
const ViewEditor = defineAsyncComponent(() => import('./ViewEditor.vue'))

const props = defineProps<{ tab: Tab }>()
const tabs = useTabsStore()
const conns = useConnectionsStore()
const meta = useMetadataStore()
const ui = useUiStore()
const actions = useObjectActions()

const conn = computed(() => props.tab.connectionId ?? '')
const db = computed(() => props.tab.database)
const name = computed(() => props.tab.objectName ?? '')
const isView = computed(() => props.tab.kind === 'view')
const obj = computed<ObjectRef>(() => ({ kind: isView.value ? 'view' : 'table', database: db.value, name: name.value }))

type View = 'data' | 'structure' | 'definition' | 'ddl' | 'info'
const isNewObj = computed(() => !props.tab.objectName)
const view = computed<View>(() => {
  if (isNewObj.value) return isView.value ? 'definition' : 'structure'
  const v = props.tab.view as View
  const allowed: View[] = isView.value ? ['data', 'structure', 'definition', 'ddl'] : ['data', 'structure', 'ddl', 'info']
  return allowed.includes(v) ? v : 'data'
})
function setView(v: View) {
  const t = tabs.find(props.tab.id)
  if (t) t.view = v
}

const columns = ref<ColumnInfo[]>([])
const indexes = ref<IndexInfo[]>([])
const fks = ref<ForeignKeyInfo[]>([])
const refs = ref<ForeignKeyInfo[]>([])
const checks = ref<CheckInfo[]>([])
const ddl = ref('')
const loading = ref(false)
const error = ref('')
const menu = ref<{ x: number; y: number; items: MenuItem[] } | null>(null)

const status = computed(() => meta.entry<TableStatus[]>(conn.value, 'status', db.value)?.data.find((t) => t.name === name.value))
const triggers = computed(() => (meta.entry<TriggerInfo[]>(conn.value, 'triggers', db.value)?.data ?? []).filter((t) => t.table === name.value))

async function load() {
  if (isNewObj.value) return
  if (conns.state(conn.value).status !== 'connected' && !(await conns.connect(conn.value))) return
  loading.value = true
  error.value = ''
  try {
    const [c, i, f, r, k, d] = await Promise.all([
      meta.columns(conn.value, db.value, name.value, true),
      isView.value ? Promise.resolve([]) : meta.indexes(conn.value, db.value, name.value, true),
      isView.value ? Promise.resolve([]) : backend.foreignKeys(conn.value, db.value, name.value),
      isView.value ? Promise.resolve([]) : backend.referencingKeys(conn.value, db.value, name.value),
      isView.value ? Promise.resolve([]) : backend.checkConstraints(conn.value, db.value, name.value),
      backend.createStatement(conn.value, db.value, obj.value.kind, name.value),
      meta.tableStatus(conn.value, db.value).catch(() => undefined),
      meta.triggers(conn.value, db.value).catch(() => undefined),
    ])
    columns.value = c
    indexes.value = i
    fks.value = f
    refs.value = r
    checks.value = k
    ddl.value = d
  } catch (e) {
    error.value = toAppError(e).message
  } finally {
    loading.value = false
  }
}
watch(() => [conn.value, db.value, name.value], load, { immediate: true })

// Reload after DDL elsewhere (metadata:invalidated reloads the columns entry).
watch(() => meta.entry<ColumnInfo[]>(conn.value, 'cols', db.value, name.value)?.data, (c) => {
  if (c && c !== columns.value && !loading.value) void load()
})

const colColumns: Column<ColumnInfo>[] = [
  { key: 'position', label: '#', width: 44, align: 'right' },
  { key: 'name', label: 'Nome', width: 180, cls: (c) => (c.key === 'PRI' ? 'diff-cell' : undefined) },
  { key: 'columnType', label: 'Tipo', width: 200, mono: true },
  { key: 'nullable', label: 'Nulo', width: 60, align: 'center', text: (c) => (c.nullable ? 'sim' : 'não') },
  { key: 'default', label: 'Padrão', width: 160, mono: true, text: (c) => (c.default === undefined || c.default === null ? (c.nullable ? 'NULL' : '') : c.default) },
  { key: 'key', label: 'Chave', width: 60 },
  { key: 'extra', label: 'Extra', width: 200 },
  { key: 'comment', label: 'Comentário', width: 260 },
]
const idxColumns: Column<IndexInfo>[] = [
  { key: 'name', label: 'Índice', width: 200 },
  { key: 'kind', label: 'Tipo', width: 110, text: (i) => (i.primary ? 'PRIMARY' : i.type === 'FULLTEXT' || i.type === 'SPATIAL' ? i.type : i.unique ? 'UNIQUE' : 'KEY') },
  { key: 'columns', label: 'Colunas', width: 320, text: (i) => i.columns.join(', ') },
  { key: 'type', label: 'Algoritmo', width: 100 },
]
const fkColumns: Column<ForeignKeyInfo>[] = [
  { key: 'name', label: 'Chave estrangeira', width: 200 },
  { key: 'columns', label: 'Colunas', width: 180, text: (f) => f.columns.join(', ') },
  { key: 'ref', label: 'Referência', width: 280, text: (f) => `${f.refDatabase !== db.value ? f.refDatabase + '.' : ''}${f.refTable} (${f.refColumns.join(', ')})` },
  { key: 'onUpdate', label: 'ON UPDATE', width: 110 },
  { key: 'onDelete', label: 'ON DELETE', width: 110 },
]
const refColumns: Column<ForeignKeyInfo>[] = [
  { key: 'table', label: 'Tabela', width: 220, text: (f) => (f.database !== db.value ? `${f.database}.${f.table}` : f.table) },
  { key: 'name', label: 'Chave estrangeira', width: 200 },
  { key: 'columns', label: 'Colunas', width: 180, text: (f) => f.columns.join(', ') },
  { key: 'refColumns', label: 'Referencia', width: 180, text: (f) => f.refColumns.join(', ') },
  { key: 'onDelete', label: 'ON DELETE', width: 110 },
]
const checkColumns: Column<CheckInfo>[] = [
  { key: 'name', label: 'Restrição', width: 200 },
  { key: 'clause', label: 'Expressão', width: 500, mono: true },
]
const trgColumns: Column<TriggerInfo>[] = [
  { key: 'name', label: 'Trigger', width: 200 },
  { key: 'when', label: 'Quando', width: 160, text: (t) => `${t.timing} ${t.event}` },
  { key: 'statement', label: 'Corpo', width: 500, mono: true, text: (t) => t.statement.replace(/\s+/g, ' ') },
]

const info = computed(() => {
  const s = status.value
  if (!s) return []
  const rows: [string, string][] = [
    ['Tipo', s.type], ['Engine', s.engine ?? ''], ['Formato de linha', s.rowFormat ?? ''], ['Linhas (estimativa)', formatCount(s.rows)],
    ['Média por linha', formatBytes(s.avgRowLength)], ['Tamanho dos dados', formatBytes(s.dataLength)], ['Tamanho dos índices', formatBytes(s.indexLength)],
    ['Espaço livre', formatBytes(s.dataFree)], ['Tamanho máximo', formatBytes(s.maxDataLength)], ['Próximo AUTO_INCREMENT', s.autoIncrement ?? ''],
    ['Collation', s.collation ?? ''], ['Criada em', formatServerTime(s.createTime)], ['Atualizada em', formatServerTime(s.updateTime)],
    ['Verificada em', formatServerTime(s.checkTime)], ['Checksum', s.checksum ?? ''], ['Opções', s.createOptions ?? ''], ['Comentário', s.comment ?? ''],
  ]
  return rows.filter(([, v]) => v !== '')
})

function generate(kind: StatementKind) {
  tabs.open({ title: `${kind.toUpperCase()} ${name.value}`, connectionId: conn.value, database: db.value, sql: tableStatement(kind, db.value, name.value, columns.value) })
}

function showMore(e: MouseEvent) {
  const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
  const items: MenuItem[] = [
    { label: 'Gerar SELECT', action: () => generate('select') },
    { label: 'Gerar INSERT', action: () => generate('insert'), disabled: isView.value },
    { label: 'Gerar UPDATE', action: () => generate('update'), disabled: isView.value },
    { label: 'Gerar DELETE', action: () => generate('delete'), disabled: isView.value },
    { separator: true, label: '' },
    { label: 'Copiar código CREATE', icon: 'copy', action: () => actions.copyDdl(conn.value, obj.value) },
    { label: 'Código CREATE em nova aba', action: () => actions.ddlInEditor(conn.value, obj.value) },
    { separator: true, label: '' },
    { label: 'Renomear…', icon: 'edit', action: () => actions.rename(conn.value, obj.value) },
    { label: 'Esvaziar tabela…', danger: true, disabled: isView.value, action: () => actions.truncate(conn.value, db.value, [name.value]) },
    { label: `Excluir ${isView.value ? 'view' : 'tabela'}…`, icon: 'trash', danger: true, action: () => actions.drop(conn.value, [obj.value]) },
  ]
  menu.value = { x: r.left, y: r.bottom + 2, items }
}

async function copyDdl() {
  await navigator.clipboard.writeText(ddl.value + ';').then(() => ui.toast('success', 'Copiado.', 1500)).catch(() => undefined)
}
</script>

<template>
  <div class="tv">
    <header class="head">
      <div class="title">
        <Icon :name="isView ? 'view' : 'table'" :size="16" />
        <strong>{{ name || (isView ? 'Nova view' : 'Nova tabela') }}</strong>
        <span class="muted">{{ db }}</span>
        <span v-if="status && !isView" class="muted">· {{ status.engine }} · ≈{{ formatCount(status.rows) }} linhas · {{ formatBytes((status.dataLength ?? 0) + (status.indexLength ?? 0)) }}</span>
        <span v-if="status?.comment" class="muted ellipsis">· {{ status.comment }}</span>
        <span class="spacer" />
        <button class="btn small" @click="tabs.open({ connectionId: conn, database: db })"><Icon name="terminal" :size="12" /> Consulta</button>
        <button v-if="!isNewObj" class="icon-btn" title="Mais ações" aria-label="Mais ações" @click="showMore"><Icon name="more" /></button>
      </div>
      <nav v-if="!isNewObj" class="subtabs" role="tablist">
        <button role="tab" :class="{ active: view === 'data' }" :aria-selected="view === 'data'" @click="setView('data')"><Icon name="table" :size="13" /> Dados</button>
        <button role="tab" :class="{ active: view === 'structure' }" :aria-selected="view === 'structure'" @click="setView('structure')"><Icon name="column" :size="13" /> {{ isView ? 'Colunas' : 'Estrutura' }}</button>
        <button v-if="isView" role="tab" :class="{ active: view === 'definition' }" :aria-selected="view === 'definition'" @click="setView('definition')"><Icon name="edit" :size="13" /> Definição</button>
        <button role="tab" :class="{ active: view === 'ddl' }" :aria-selected="view === 'ddl'" @click="setView('ddl')"><Icon name="file" :size="13" /> Código CREATE</button>
        <button v-if="!isView" role="tab" :class="{ active: view === 'info' }" :aria-selected="view === 'info'" @click="setView('info')"><Icon name="info" :size="13" /> Informações</button>
      </nav>
    </header>

    <div v-if="error" class="banner err"><Icon name="alert" :size="14" /> {{ error }}</div>

    <div class="content">
      <TableData v-if="view === 'data'" :tab="tab" :columns="columns" :indexes="indexes" :foreign-keys="fks" />

      <ViewEditor v-else-if="view === 'definition'" :tab="tab" @saved="load" />

      <template v-else-if="view === 'structure'">
        <TableEditor v-if="!isView" :tab="tab" :columns="columns" :indexes="indexes" :foreign-keys="fks" :checks="checks" :status="status" :ddl="ddl" @saved="load" />
        <div v-else class="sections">
          <section class="sec grow">
            <h3>Colunas <span class="n">{{ columns.length }}</span></h3>
            <DataTable :columns="colColumns" :rows="columns" :row-key="(c) => c.name" sort-key="position" :loading="loading" />
          </section>
        </div>
        <div v-if="!isView && (refs.length || triggers.length)" class="extras">
          <section v-if="refs.length" class="sec">
            <h3>Referenciada por <span class="n">{{ refs.length }}</span></h3>
            <DataTable :columns="refColumns" :rows="refs" :row-key="(f) => f.database + '.' + f.table + '.' + f.name" @activate="(f) => actions.open(conn, { kind: 'table', database: f.database, name: f.table })" />
          </section>
          <section v-if="triggers.length" class="sec">
            <h3>Triggers <span class="n">{{ triggers.length }}</span></h3>
            <DataTable :columns="trgColumns" :rows="triggers" :row-key="(t) => t.name" @activate="(t) => actions.open(conn, { kind: 'trigger', database: db, name: t.name })" />
          </section>
        </div>
      </template>

      <div v-else-if="view === 'ddl'" class="ddl">
        <div class="ddl-bar">
          <button class="btn small" @click="copyDdl"><Icon name="copy" :size="12" /> Copiar</button>
          <button class="btn small" @click="actions.ddlInEditor(conn, obj)"><Icon name="terminal" :size="12" /> Abrir em nova aba</button>
          <button class="icon-btn" title="Recarregar" aria-label="Recarregar" @click="load"><Icon name="refresh" :size="14" /></button>
        </div>
        <CodeEditor :model-value="ddl" readonly />
      </div>

      <div v-else class="info selectable">
        <table>
          <tr v-for="[k, v] in info" :key="k"><th>{{ k }}</th><td>{{ v }}</td></tr>
        </table>
        <template v-if="indexes.length || fks.length || checks.length">
          <h3>Índices</h3>
          <div class="mini"><DataTable :columns="idxColumns" :rows="indexes" :row-key="(i) => i.name" /></div>
          <template v-if="fks.length">
            <h3>Chaves estrangeiras</h3>
            <div class="mini"><DataTable :columns="fkColumns" :rows="fks" :row-key="(f) => f.name" /></div>
          </template>
          <template v-if="checks.length">
            <h3>Restrições CHECK</h3>
            <div class="mini"><DataTable :columns="checkColumns" :rows="checks" :row-key="(c) => c.name" /></div>
          </template>
        </template>
      </div>
    </div>
    <ContextMenu v-if="menu" :x="menu.x" :y="menu.y" :items="menu.items" @close="menu = null" />
  </div>
</template>

<style scoped>
.tv {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}
.head {
  padding: 6px 10px 0;
  border-bottom: 1px solid var(--border);
  background: var(--bg-elev);
  flex: none;
}
.title {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
  min-width: 0;
}
.spacer {
  flex: 1;
}
.subtabs {
  display: flex;
  gap: 2px;
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
}
.subtabs button.active {
  color: var(--text);
  border-bottom-color: var(--accent);
}
.banner.err {
  display: flex;
  gap: 8px;
  align-items: center;
  padding: 6px 10px;
  background: var(--danger-soft);
  color: var(--danger);
  font-size: 12px;
}
.content {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.sections {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.sec {
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.sec.grow {
  flex: 1;
}
.sec h3,
.info h3 {
  margin: 0;
  padding: 6px 10px;
  font-size: 12px;
  font-weight: 600;
  border-bottom: 1px solid var(--border);
  background: var(--bg-elev);
}
.extras {
  flex: none;
  max-height: 40%;
  display: flex;
  border-top: 1px solid var(--border-strong);
}
.extras .sec {
  flex: 1;
  height: 180px;
  border-right: 1px solid var(--border);
}
.n {
  font-size: 10px;
  padding: 0 5px;
  border-radius: 8px;
  background: var(--bg-active);
  font-weight: 400;
}
.ddl {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.ddl-bar {
  display: flex;
  gap: 8px;
  padding: 5px 8px;
  border-bottom: 1px solid var(--border);
}
.info {
  flex: 1;
  overflow: auto;
  padding-bottom: 16px;
}
.info table {
  border-collapse: collapse;
  margin: 10px;
}
.info th {
  text-align: left;
  font-weight: 500;
  color: var(--text-muted);
  padding: 3px 16px 3px 0;
  white-space: nowrap;
}
.info td {
  padding: 3px 0;
}
.mini {
  height: 160px;
}
</style>

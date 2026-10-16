<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Modal from '../common/Modal.vue'
import Icon from '../common/Icon.vue'
import { useUiStore } from '../../stores/ui'
import { useTasksStore } from '../../stores/tasks'
import { useConnectionsStore } from '../../stores/connections'
import { useMetadataStore } from '../../stores/metadata'
import { useSettingsStore } from '../../stores/settings'
import { backend } from '../../api/backend'
import { toAppError } from '../../api/errors'
import type { DatabaseInfo, DumpOptions, DumpSelection, DumpTarget, TableInfo } from '../../api/types'
import { withConfirmation } from '../../composables/objectActions'

const ui = useUiStore()
const tasks = useTasksStore()
const conns = useConnectionsStore()
const meta = useMetadataStore()
const settings = useSettingsStore()
const st = computed(() => ui.dumpDialog)
const conn = computed(() => st.value?.connectionId ?? '')

interface DbSel { name: string; checked: boolean; open: boolean; tables: Set<string>; routines: boolean; events: boolean; triggers: boolean }
const dbs = ref<DbSel[]>([])
const opts = ref<Omit<DumpOptions, 'selections'>>({
  dropDatabase: false, createDatabase: true, dropObjects: true, createObjects: true, data: 'insert', maxInsertKB: 1024,
  comments: true, wrapTransactions: false, removeAutoIncrement: false, removeDefiner: false, targetDatabase: '',
})
const target = ref<DumpTarget>({ kind: 'file', connectionId: '', database: '', continueOnError: false })
const busy = ref(false)

watch(st, async (s) => {
  if (!s) return
  target.value = { kind: 'file', connectionId: s.connectionId, database: '', continueOnError: false }
  const list = await meta.databases(s.connectionId).catch(() => [] as DatabaseInfo[])
  dbs.value = list.filter((d) => settings.prefs.showSystemSchemas || !d.system || s.databases.includes(d.name)).map((d) => ({
    name: d.name, checked: s.databases.includes(d.name), open: s.databases.length === 1 && s.databases[0] === d.name,
    tables: new Set<string>(), routines: true, events: true, triggers: true,
  }))
  for (const d of dbs.value.filter((x) => x.checked)) await loadTables(d, s.tables)
}, { immediate: true })

function tablesOf(d: DbSel): TableInfo[] {
  return meta.entry<TableInfo[]>(conn.value, 'tables', d.name)?.data ?? []
}
async function loadTables(d: DbSel, only?: string[]) {
  const list = await meta.tables(conn.value, d.name).catch(() => [] as TableInfo[])
  d.tables = new Set(only?.length ? only.filter((t) => list.some((x) => x.name === t)) : list.map((t) => t.name))
  if (only?.length) d.routines = d.events = d.triggers = false
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

const selections = computed<DumpSelection[]>(() => dbs.value.filter((d) => d.checked).map((d) => {
  const all = tablesOf(d)
  const allTables = all.length > 0 && all.every((t) => d.tables.has(t.name))
  return { database: d.name, tables: [...d.tables], routines: d.routines, events: d.events, triggers: d.triggers, allObjects: allTables && d.routines && d.events && d.triggers }
}))
const otherConns = computed(() => conns.profiles.filter((p) => conns.state(p.id).status === 'connected'))

const command = computed(() => {
  const p = conns.byId.get(conn.value)
  if (!p) return ''
  const args = ['mysqldump', `-h ${p.host}`, `-P ${p.port}`, `-u ${p.user}`, '-p', '--single-transaction', '--default-character-set=utf8mb4']
  const o = opts.value
  if (!o.createObjects) args.push('--no-create-info')
  if (!o.dropObjects) args.push('--skip-add-drop-table')
  if (o.dropDatabase) args.push('--add-drop-database')
  if (o.data === 'none') args.push('--no-data')
  if (o.data === 'replace') args.push('--replace')
  if (o.data === 'insert-ignore') args.push('--insert-ignore')
  const sel = selections.value
  if (sel.some((s) => s.routines)) args.push('--routines')
  if (sel.some((s) => s.events)) args.push('--events')
  if (!sel.some((s) => s.triggers)) args.push('--skip-triggers')
  if (sel.length === 1 && !sel[0]!.allObjects && sel[0]!.tables.length) args.push(sel[0]!.database, ...sel[0]!.tables)
  else args.push('--databases', ...sel.map((s) => s.database))
  return args.join(' ')
})

async function start() {
  if (!selections.value.length) {
    ui.toast('warning', 'Selecione pelo menos um database.')
    return
  }
  busy.value = true
  try {
    const t = { ...target.value }
    if (t.kind === 'database' && !t.connectionId) t.connectionId = conn.value
    const id = await withConfirmation((c) => backend.startDump(conn.value, { ...opts.value, selections: selections.value }, t, c))
    if (id) {
      tasks.track(id, () => {
        if (t.kind === 'database') void meta.invalidate(t.connectionId, '')
      })
      ui.dumpDialog = null
    }
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Modal v-if="st" :title="`Exportar como SQL — ${conns.byId.get(conn)?.name ?? ''}`" :width="860" :close-on-backdrop="false" @close="ui.dumpDialog = null">
    <div class="dump">
      <div class="objects">
        <div class="lbl">Databases e objetos</div>
        <div class="tree">
          <div v-for="d in dbs" :key="d.name" class="db">
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
              <label class="check small"><input v-model="d.routines" type="checkbox" :disabled="!d.checked" /> procedures e functions</label>
              <label class="check small"><input v-model="d.triggers" type="checkbox" :disabled="!d.checked" /> triggers</label>
              <label class="check small"><input v-model="d.events" type="checkbox" :disabled="!d.checked" /> eventos</label>
            </div>
          </div>
        </div>
      </div>
      <div class="opts">
        <fieldset>
          <legend>Database</legend>
          <label class="check"><input v-model="opts.dropDatabase" type="checkbox" /> DROP DATABASE</label>
          <label class="check"><input v-model="opts.createDatabase" type="checkbox" /> CREATE DATABASE</label>
        </fieldset>
        <fieldset>
          <legend>Tabelas e objetos</legend>
          <label class="check"><input v-model="opts.dropObjects" type="checkbox" /> DROP antes de criar</label>
          <label class="check"><input v-model="opts.createObjects" type="checkbox" /> CREATE</label>
          <label class="check"><input v-model="opts.removeAutoIncrement" type="checkbox" /> remover AUTO_INCREMENT=n</label>
          <label class="check"><input v-model="opts.removeDefiner" type="checkbox" /> remover DEFINER</label>
        </fieldset>
        <fieldset>
          <legend>Dados</legend>
          <select v-model="opts.data" class="select">
            <option value="none">sem dados</option>
            <option value="insert">INSERT</option>
            <option value="insert-ignore">INSERT IGNORE</option>
            <option value="replace">REPLACE</option>
            <option value="delete-insert">DELETE + INSERT</option>
          </select>
          <label class="field"><span>Tamanho máximo de cada INSERT (KB, 0 = uma linha por INSERT)</span><input v-model.number="opts.maxInsertKB" type="number" min="0" class="input" /></label>
          <label class="check"><input v-model="opts.wrapTransactions" type="checkbox" /> dados de cada tabela numa transação</label>
          <label class="check"><input v-model="opts.comments" type="checkbox" /> comentários</label>
        </fieldset>
        <fieldset>
          <legend>Destino</legend>
          <label class="check"><input v-model="target.kind" type="radio" value="file" /> arquivo .sql</label>
          <label class="check"><input v-model="target.kind" type="radio" value="gzip" /> arquivo .sql.gz (compactado)</label>
          <label class="check"><input v-model="target.kind" type="radio" value="directory" /> pasta (um arquivo por objeto)</label>
          <label class="check"><input v-model="target.kind" type="radio" value="database" :disabled="selections.length > 1" /> copiar para outro database</label>
          <template v-if="target.kind === 'database'">
            <select v-model="target.connectionId" class="select"><option v-for="p in otherConns" :key="p.id" :value="p.id">{{ p.name }}</option></select>
            <input v-model="target.database" class="input mono" placeholder="database de destino (criado se não existir)" />
            <label class="check"><input v-model="target.continueOnError" type="checkbox" /> continuar após erros</label>
          </template>
        </fieldset>
      </div>
    </div>
    <details class="cmd">
      <summary>Comando mysqldump equivalente</summary>
      <code class="mono selectable">{{ command }}</code>
    </details>
    <template #footer>
      <span class="hint">Leitura num snapshot consistente (como --single-transaction).</span>
      <button class="btn" @click="ui.dumpDialog = null">Cancelar</button>
      <button class="btn primary" :disabled="busy || !selections.length" @click="start"><span v-if="busy" class="spinner" /> Exportar</button>
    </template>
  </Modal>
</template>

<style scoped>
.dump {
  display: grid;
  grid-template-columns: 1fr 1.1fr;
  gap: 12px;
  min-height: 360px;
}
.objects {
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.lbl {
  font-size: 12px;
  color: var(--text-muted);
  margin-bottom: 4px;
}
.tree {
  flex: 1;
  max-height: 460px;
  overflow: auto;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 4px;
}
.row {
  display: flex;
  align-items: center;
  gap: 4px;
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
.opts {
  display: flex;
  flex-direction: column;
  gap: 8px;
  overflow: auto;
  max-height: 480px;
}
fieldset {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin: 0;
  padding: 6px 10px 8px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
}
legend {
  font-size: 12px;
  color: var(--text-muted);
  padding: 0 4px;
}
.cmd {
  margin-top: 10px;
  font-size: 12px;
}
.cmd code {
  display: block;
  margin-top: 4px;
  padding: 6px;
  background: var(--bg-sunken);
  border-radius: var(--radius-sm);
  word-break: break-all;
}
.hint {
  margin-right: auto;
  color: var(--text-faint);
  font-size: 11px;
  align-self: center;
}
</style>

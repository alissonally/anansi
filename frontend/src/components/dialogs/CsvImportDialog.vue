<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Modal from '../common/Modal.vue'
import Icon from '../common/Icon.vue'
import { useUiStore } from '../../stores/ui'
import { useTasksStore } from '../../stores/tasks'
import { useMetadataStore } from '../../stores/metadata'
import { backend } from '../../api/backend'
import { toAppError } from '../../api/errors'
import type { ColumnDef, ColumnInfo, CsvFormat, CsvPreview, CsvTarget, DatabaseInfo, TableDef, TableInfo } from '../../api/types'
import { withConfirmation } from '../../composables/objectActions'
import { formatBytes } from '../../utils/format'

const ui = useUiStore()
const tasks = useTasksStore()
const meta = useMetadataStore()
const st = computed(() => ui.csvDialog)
const conn = computed(() => st.value?.connectionId ?? '')

const preview = ref<CsvPreview | null>(null)
const format = ref<CsvFormat>({ separator: ';', encloser: '"', escape: '', encoding: 'utf-8', skipLines: 1, nullMarker: '\\N', emptyNull: false })
const db = ref('')
const table = ref('')
const newTable = ref(false)
const newName = ref('')
const mapping = ref<string[]>([])
const target = ref<Omit<CsvTarget, 'database' | 'table' | 'columns' | 'numeric'>>({ mode: 'insert', batchRows: 500, atomic: true, thousands: '', decimal: '' })
const truncate = ref(false)
const serverSide = ref(false)
const busy = ref(false)
const error = ref('')

watch(st, async (s) => {
  preview.value = null
  error.value = ''
  if (!s) return
  db.value = s.database
  table.value = s.table ?? ''
  try {
    const p = await backend.pickCsvFile()
    if (!p) {
      ui.csvDialog = null
      return
    }
    preview.value = p
    format.value = { ...p.detected.format, nullMarker: p.detected.format.nullMarker || '\\N' }
    if (format.value.separator === '\t') format.value.separator = '\\t'
    newName.value = p.name.replace(/\.[^.]+$/, '').replace(/[^\p{L}\p{N}_]+/gu, '_').toLowerCase()
    newTable.value = !table.value
    void meta.databases(s.connectionId).catch(() => undefined)
    if (db.value) void meta.tables(s.connectionId, db.value).catch(() => undefined)
    remap()
  } catch (e) {
    error.value = toAppError(e).message
  }
}, { immediate: true })

let timer: number | undefined
watch(format, () => {
  window.clearTimeout(timer)
  timer = window.setTimeout(async () => {
    const p = preview.value
    if (!p) return
    try {
      const fresh = await backend.previewCsv(p.path, format.value)
      preview.value = { ...p, detected: { ...p.detected, sample: fresh.detected.sample } }
      remap()
    } catch (e) {
      error.value = toAppError(e).message
    }
  }, 300)
}, { deep: true })

const dbs = computed(() => (meta.entry<DatabaseInfo[]>(conn.value, 'dbs')?.data ?? []).map((d) => d.name))
const tables = computed(() => (meta.entry<TableInfo[]>(conn.value, 'tables', db.value)?.data ?? []).filter((t) => t.type === 'table').map((t) => t.name))
const targetCols = computed<ColumnInfo[]>(() => (table.value && !newTable.value ? meta.entry<ColumnInfo[]>(conn.value, 'cols', db.value, table.value)?.data ?? [] : []))
watch(db, (d) => { if (d) void meta.tables(conn.value, d).catch(() => undefined) })
watch([table, newTable], async () => {
  if (table.value && !newTable.value) await meta.columns(conn.value, db.value, table.value).catch(() => undefined)
  remap()
})

const fileCols = computed(() => {
  const d = preview.value?.detected
  const n = Math.max(d?.columns?.length ?? 0, ...(d?.sample ?? []).map((r) => r.length))
  return Array.from({ length: n }, (_, i) => d?.header?.[i] ?? `coluna${i + 1}`)
})

function remap() {
  const names = newTable.value ? fileCols.value : targetCols.value.map((c) => c.name)
  mapping.value = fileCols.value.map((f, i) => names.find((n) => n.toLowerCase() === f.toLowerCase()) ?? (newTable.value ? f : names[i] ?? ''))
}

function guessToColumn(i: number): ColumnDef {
  const g = preview.value?.detected.columns?.[i]
  return {
    origName: '', name: mapping.value[i] || fileCols.value[i]!, type: g?.type ?? 'VARCHAR', length: g?.length ?? (g ? '' : '255'), unsigned: !!g?.unsigned, zerofill: false,
    nullable: true, defaultKind: 'null', default: '', autoIncrement: false, onUpdate: '', comment: '', charset: '', collation: '', generated: '', generatedKind: '', invisible: false, srid: '',
  }
}

const numericTypes = /^(tinyint|smallint|mediumint|int|integer|bigint|decimal|numeric|float|double|real)$/i
async function start() {
  error.value = ''
  const p = preview.value
  if (!p || !db.value) return
  let tbl = table.value
  busy.value = true
  try {
    let colTypes: Record<string, string> = Object.fromEntries(targetCols.value.map((c) => [c.name, c.dataType]))
    if (newTable.value) {
      tbl = newName.value.trim()
      const cols = fileCols.value.map((_, i) => guessToColumn(i)).filter((_, i) => mapping.value[i])
      const def: TableDef = { database: db.value, name: tbl, origName: '', columns: cols, indexes: [], foreignKeys: [], checks: [], mariadb: false,
        options: { engine: '', collation: '', comment: '', rowFormat: '', autoIncrement: '', avgRowLength: '', maxRows: '', checksum: false, partition: '', convertData: false } }
      const res = await withConfirmation((c) => backend.saveTableDef(conn.value, def, c))
      if (!res) return
      const bad = res.find((r) => r.error)
      if (bad) {
        error.value = `${bad.error!.message} — ${bad.sql}`
        return
      }
      colTypes = Object.fromEntries(cols.map((c) => [c.name, c.type]))
      await meta.invalidate(conn.value, db.value)
    }
    const columns = mapping.value
    const req = {
      path: p.path, format: format.value, truncate: truncate.value, serverSide: serverSide.value,
      target: { ...target.value, database: db.value, table: tbl, columns, numeric: columns.filter(Boolean).map((c) => numericTypes.test(colTypes[c] ?? '')) },
    }
    const id = await withConfirmation((c) => backend.importCsv(conn.value, req, c))
    if (id) {
      tasks.track(id, () => void meta.invalidate(conn.value, db.value))
      ui.csvDialog = null
    }
  } catch (e) {
    error.value = toAppError(e).message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Modal v-if="st && preview" :title="`Importar ${preview.name}`" :width="900" :close-on-backdrop="false" @close="ui.csvDialog = null">
    <div class="csv">
      <p class="faint small">{{ preview.path }} · {{ formatBytes(preview.size) }}</p>
      <div class="fmt">
        <label class="field"><span>Separador</span>
          <select v-model="format.separator" class="select"><option value=",">vírgula ,</option><option value=";">ponto e vírgula ;</option><option value="\t">tabulação</option><option value="|">barra |</option></select>
        </label>
        <label class="field"><span>Delimitador de texto</span><select v-model="format.encloser" class="select"><option value="&quot;">"</option><option value="'">'</option><option value="">nenhum</option></select></label>
        <label class="field"><span>Escape</span><select v-model="format.escape" class="select"><option value="">aspas dobradas ("")</option><option value="\">barra invertida (\)</option></select></label>
        <label class="field"><span>Codificação</span><select v-model="format.encoding" class="select"><option value="utf-8">UTF-8</option><option value="latin1">Latin-1 / Windows-1252</option></select></label>
        <label class="field"><span>Ignorar linhas iniciais</span><input v-model.number="format.skipLines" type="number" min="0" class="input" /></label>
        <label class="field"><span>Marcador de NULL</span><input v-model="format.nullMarker" class="input mono" /></label>
        <label class="check"><input v-model="format.emptyNull" type="checkbox" /> vazio = NULL</label>
      </div>
      <div class="sample">
        <table>
          <thead><tr><th v-for="(c, i) in fileCols" :key="i">{{ c }}<div class="faint small">{{ preview.detected.columns?.[i]?.type }}{{ preview.detected.columns?.[i]?.length ? `(${preview.detected.columns[i]!.length})` : '' }}</div></th></tr></thead>
          <tbody>
            <tr v-for="(r, ri) in preview.detected.sample ?? []" :key="ri"><td v-for="(c, ci) in fileCols" :key="ci" :class="{ null: r[ci] === null }">{{ r[ci] === null ? 'NULL' : r[ci] }}</td></tr>
          </tbody>
        </table>
      </div>
      <div class="dest">
        <label class="field"><span>Database</span><select v-model="db" class="select"><option v-for="d in dbs" :key="d" :value="d">{{ d }}</option></select></label>
        <label class="check"><input v-model="newTable" type="checkbox" /> criar tabela nova</label>
        <label v-if="newTable" class="field"><span>Nome da tabela</span><input v-model="newName" class="input mono" /></label>
        <label v-else class="field"><span>Tabela</span><select v-model="table" class="select"><option v-for="t in tables" :key="t" :value="t">{{ t }}</option></select></label>
      </div>
      <div class="map">
        <div class="lbl">Colunas do arquivo → {{ newTable ? 'colunas da nova tabela (tipos detectados)' : 'colunas da tabela' }}</div>
        <div v-for="(c, i) in fileCols" :key="i" class="maprow">
          <span class="mono ellipsis">{{ c }}</span>
          <Icon name="chevronRight" :size="11" />
          <input v-if="newTable" v-model="mapping[i]" class="input compact mono" placeholder="(ignorar)" />
          <select v-else v-model="mapping[i]" class="select compact">
            <option value="">(ignorar)</option>
            <option v-for="tc in targetCols" :key="tc.name" :value="tc.name">{{ tc.name }} — {{ tc.columnType }}</option>
          </select>
        </div>
      </div>
      <div class="opts">
        <label class="field"><span>Chaves duplicadas</span><select v-model="target.mode" class="select"><option value="insert">erro (INSERT)</option><option value="ignore">ignorar (INSERT IGNORE)</option><option value="replace">substituir (REPLACE)</option></select></label>
        <label class="field"><span>Números locais: milhar / decimal</span>
          <span class="pair"><input v-model="target.thousands" class="input mono" placeholder="." maxlength="1" /><input v-model="target.decimal" class="input mono" placeholder="," maxlength="1" /></span>
        </label>
        <label class="check"><input v-model="target.atomic" type="checkbox" :disabled="serverSide" /> tudo ou nada (uma transação)</label>
        <label class="check"><input v-model="truncate" type="checkbox" :disabled="newTable" /> esvaziar a tabela antes (TRUNCATE)</label>
        <label class="check" title="Mais rápido, mas exige local_infile=ON no servidor; o arquivo é liberado só para esta importação"><input v-model="serverSide" type="checkbox" /> o servidor lê o arquivo (LOAD DATA LOCAL INFILE)</label>
      </div>
      <p v-if="error" class="err"><Icon name="alert" :size="12" /> {{ error }}</p>
    </div>
    <template #footer>
      <button class="btn" @click="ui.csvDialog = null">Cancelar</button>
      <button class="btn primary" :disabled="busy || !db || (!newTable && !table) || !mapping.some(Boolean)" @click="start"><span v-if="busy" class="spinner" /> Importar</button>
    </template>
  </Modal>
</template>

<style scoped>
.csv {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.small {
  font-size: 11px;
  margin: 0;
}
.fmt,
.dest,
.opts {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 8px;
  align-items: end;
}
.sample {
  max-height: 220px;
  overflow: auto;
  border: 1px solid var(--border);
  border-radius: var(--radius);
}
.sample table {
  border-collapse: collapse;
  font-size: 12px;
  font-family: var(--font-mono);
}
.sample th,
.sample td {
  padding: 2px 8px;
  border: 1px solid var(--border);
  white-space: nowrap;
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
  text-align: left;
}
.sample th {
  position: sticky;
  top: 0;
  background: var(--bg-elev);
  font-family: var(--font-ui);
}
td.null {
  color: var(--null);
  font-style: italic;
}
.map {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 4px 12px;
}
.lbl {
  grid-column: 1 / -1;
  font-size: 12px;
  color: var(--text-muted);
}
.maprow {
  display: grid;
  grid-template-columns: 1fr auto 1.4fr;
  align-items: center;
  gap: 6px;
  font-size: 12px;
}
.pair {
  display: flex;
  gap: 4px;
}
.err {
  margin: 0;
  color: var(--danger);
  font-size: 12px;
}
</style>

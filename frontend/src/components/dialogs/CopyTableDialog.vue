<script setup lang="ts">
// HeidiSQL "Create table copy": structure parts, columns and filtered data.
import { computed, ref, watch } from 'vue'
import Modal from '../common/Modal.vue'
import { useUiStore } from '../../stores/ui'
import { useMetadataStore } from '../../stores/metadata'
import { useTabsStore } from '../../stores/tabs'
import { backend } from '../../api/backend'
import { toAppError } from '../../api/errors'
import type { ColumnInfo, CopyTableSpec, DatabaseInfo } from '../../api/types'
import { reportResults, withConfirmation } from '../../composables/objectActions'

const ui = useUiStore()
const meta = useMetadataStore()
const tabs = useTabsStore()
const st = computed(() => ui.copyTableDialog)
const conn = computed(() => st.value?.connectionId ?? '')
const spec = ref<CopyTableSpec>(blank())
const cols = ref<ColumnInfo[]>([])
const picked = ref(new Set<string>())
const dbs = ref<string[]>([])
const preview = ref<string[]>([])
const error = ref('')
const busy = ref(false)

function blank(): CopyTableSpec {
  return { database: '', table: '', targetDatabase: '', targetTable: '', columns: [], indexes: true, foreignKeys: true, checks: true, data: true, filter: '', dropExisting: false }
}

watch(st, async (s) => {
  if (!s) return
  spec.value = { ...blank(), database: s.database, table: s.table, targetDatabase: s.database, targetTable: s.table + '_copia' }
  dbs.value = (await meta.databases(s.connectionId).catch(() => [] as DatabaseInfo[])).map((d) => d.name)
  cols.value = await meta.columns(s.connectionId, s.database, s.table).catch(() => [])
  picked.value = new Set(cols.value.map((c) => c.name))
}, { immediate: true })

function toggle(c: string) {
  const s = new Set(picked.value)
  if (s.has(c)) s.delete(c)
  else s.add(c)
  picked.value = s
}
const request = computed<CopyTableSpec>(() => ({ ...spec.value, columns: picked.value.size === cols.value.length ? [] : cols.value.map((c) => c.name).filter((c) => picked.value.has(c)) }))

let timer: number | undefined
watch(request, (r) => {
  window.clearTimeout(timer)
  timer = window.setTimeout(async () => {
    try {
      preview.value = await backend.previewCopyTable(conn.value, r)
      error.value = ''
    } catch (e) {
      preview.value = []
      error.value = toAppError(e).message
    }
  }, 200)
}, { deep: true })

async function run() {
  busy.value = true
  try {
    const r = request.value
    const res = await withConfirmation((c) => backend.copyTable(conn.value, r, c))
    if (!res) return
    if (!reportResults(res, `Tabela copiada para ${r.targetDatabase}.${r.targetTable}.`)) {
      await meta.invalidate(conn.value, r.targetDatabase)
      tabs.openObject({ kind: 'table', connectionId: conn.value, database: r.targetDatabase, objectName: r.targetTable })
      ui.copyTableDialog = null
    }
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Modal v-if="st" :title="`Copiar tabela ${st.database}.${st.table}`" :width="760" :close-on-backdrop="false" @close="ui.copyTableDialog = null">
    <div class="copy">
      <div class="left">
        <div class="row">
          <label class="field"><span>Database</span>
            <select v-model="spec.targetDatabase" class="select"><option v-for="d in dbs" :key="d" :value="d">{{ d }}</option></select>
          </label>
          <label class="field grow"><span>Nome da cópia</span><input v-model="spec.targetTable" class="input mono" /></label>
        </div>
        <fieldset>
          <legend>Estrutura</legend>
          <label class="check"><input v-model="spec.indexes" type="checkbox" /> índices (a chave primária sempre vai)</label>
          <label class="check"><input v-model="spec.foreignKeys" type="checkbox" /> chaves estrangeiras</label>
          <label class="check"><input v-model="spec.checks" type="checkbox" /> restrições CHECK</label>
          <label class="check"><input v-model="spec.dropExisting" type="checkbox" /> substituir se já existir (DROP TABLE)</label>
        </fieldset>
        <fieldset>
          <legend>Dados</legend>
          <label class="check"><input v-model="spec.data" type="checkbox" /> copiar as linhas</label>
          <label class="field"><span>Filtro (WHERE, opcional)</span><input v-model="spec.filter" class="input mono" :disabled="!spec.data" placeholder="ex.: criado_em >= '2024-01-01'" /></label>
        </fieldset>
      </div>
      <fieldset class="cols">
        <legend>Colunas ({{ picked.size }} de {{ cols.length }})</legend>
        <div class="list">
          <label v-for="c in cols" :key="c.name" class="check small"><input type="checkbox" :checked="picked.has(c.name)" @change="toggle(c.name)" /> <span class="mono">{{ c.name }}</span> <span class="faint">{{ c.columnType }}</span></label>
        </div>
      </fieldset>
    </div>
    <div class="lbl">Instruções</div>
    <pre class="sql mono selectable" :class="{ err: error }">{{ error || preview.join(';\n') }}</pre>
    <template #footer>
      <span class="hint">Nomes de FKs e CHECKs são renomeados (são únicos por database).</span>
      <button class="btn" @click="ui.copyTableDialog = null">Cancelar</button>
      <button class="btn primary" :disabled="busy || !!error || !preview.length" @click="run"><span v-if="busy" class="spinner" /> Copiar</button>
    </template>
  </Modal>
</template>

<style scoped>
.copy {
  display: grid;
  grid-template-columns: 1.2fr 1fr;
  gap: 12px;
}
.left {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.row {
  display: flex;
  gap: 8px;
}
.grow {
  flex: 1;
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
.cols .list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 260px;
  overflow: auto;
}
.small {
  font-size: 12px;
}
.faint {
  color: var(--text-faint);
}
.lbl {
  margin-top: 10px;
  font-size: 12px;
  color: var(--text-muted);
}
.sql {
  margin: 4px 0 0;
  padding: 8px;
  max-height: 160px;
  overflow: auto;
  background: var(--bg-sunken);
  border-radius: var(--radius-sm);
  font-size: 12px;
  white-space: pre-wrap;
}
.sql.err {
  color: var(--danger);
}
.hint {
  margin-right: auto;
  color: var(--text-faint);
  font-size: 11px;
  align-self: center;
}
</style>

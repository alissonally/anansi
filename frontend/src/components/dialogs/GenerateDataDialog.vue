<script setup lang="ts">
// HeidiSQL "Generate data": random rows for testing.
import { computed, ref, watch } from 'vue'
import Modal from '../common/Modal.vue'
import { useUiStore } from '../../stores/ui'
import { useTasksStore } from '../../stores/tasks'
import { useMetadataStore } from '../../stores/metadata'
import { backend } from '../../api/backend'
import { toAppError } from '../../api/errors'
import type { ColumnInfo } from '../../api/types'
import { withConfirmation } from '../../composables/objectActions'

const ui = useUiStore()
const tasks = useTasksStore()
const meta = useMetadataStore()
const st = computed(() => ui.generateDialog)
const rows = ref(1000)
const nullPercent = ref(10)
const cols = ref<ColumnInfo[]>([])
const busy = ref(false)

watch(st, async (s) => {
  if (!s) return
  cols.value = await meta.columns(s.connectionId, s.database, s.table).catch(() => [])
}, { immediate: true })

function skipped(c: ColumnInfo): string {
  const ex = (c.extra ?? '').toLowerCase()
  if (ex.includes('auto_increment')) return 'auto incremento'
  if (ex.includes('generated') && !ex.includes('default_generated')) return 'gerada'
  if (/geometry|point|linestring|polygon/i.test(c.dataType)) return 'espacial (não gerada)'
  return ''
}

async function run() {
  const s = st.value
  if (!s) return
  busy.value = true
  try {
    const id = await withConfirmation((c) => backend.startGenerateData(s.connectionId, { database: s.database, table: s.table, rows: rows.value, nullPercent: nullPercent.value }, c))
    if (id) {
      tasks.track(id, () => void meta.invalidate(s.connectionId, s.database))
      ui.generateDialog = null
    }
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Modal v-if="st" :title="`Gerar dados — ${st.database}.${st.table}`" :width="560" @close="ui.generateDialog = null">
    <div class="gen">
      <div class="row">
        <label class="field"><span>Linhas</span><input v-model.number="rows" type="number" min="1" max="1000000" class="input" /></label>
        <label class="field"><span>NULL nas colunas anuláveis (%)</span><input v-model.number="nullPercent" type="number" min="0" max="100" class="input" /></label>
      </div>
      <div class="list">
        <div v-for="c in cols" :key="c.name" class="col small">
          <span class="mono">{{ c.name }}</span>
          <span class="faint">{{ c.columnType }}</span>
          <span v-if="skipped(c)" class="tag">{{ skipped(c) }}</span>
        </div>
      </div>
      <p class="faint small">
        Valores aleatórios conforme o tipo (textos respeitam o tamanho, ENUM/SET usam os valores definidos). Usa INSERT IGNORE com as chaves
        estrangeiras desligadas: linhas que violariam uma chave única são descartadas.
      </p>
    </div>
    <template #footer>
      <button class="btn" @click="ui.generateDialog = null">Cancelar</button>
      <button class="btn primary" :disabled="busy || rows < 1" @click="run"><span v-if="busy" class="spinner" /> Gerar</button>
    </template>
  </Modal>
</template>

<style scoped>
.gen {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.row {
  display: flex;
  gap: 12px;
}
.list {
  max-height: 240px;
  overflow: auto;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 4px 8px;
}
.col {
  display: flex;
  gap: 8px;
  padding: 2px 0;
}
.small {
  font-size: 12px;
}
.faint {
  color: var(--text-faint);
  margin: 0;
}
.tag {
  margin-left: auto;
  color: var(--text-muted);
  font-size: 11px;
}
</style>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Modal from '../common/Modal.vue'
import { useUiStore } from '../../stores/ui'
import { useTasksStore } from '../../stores/tasks'
import { backend } from '../../api/backend'
import { toAppError } from '../../api/errors'
import type { ExportFormat, ExportOptions } from '../../api/types'
import { formatCount } from '../../utils/format'

const ui = useUiStore()
const tasks = useTasksStore()
const st = computed(() => ui.exportDialog)

const formats: { id: ExportFormat; label: string; group: string }[] = [
  { id: 'csv', label: 'CSV (texto delimitado)', group: 'Texto' },
  { id: 'excel', label: 'CSV para Excel (; com BOM)', group: 'Texto' },
  { id: 'tsv', label: 'TSV (tabulação)', group: 'Texto' },
  { id: 'json', label: 'JSON', group: 'Dados' },
  { id: 'jsonl', label: 'JSON Lines', group: 'Dados' },
  { id: 'xml', label: 'XML', group: 'Dados' },
  { id: 'php', label: 'Array PHP', group: 'Dados' },
  { id: 'html', label: 'Tabela HTML', group: 'Documentos' },
  { id: 'markdown', label: 'Markdown', group: 'Documentos' },
  { id: 'latex', label: 'LaTeX', group: 'Documentos' },
  { id: 'textile', label: 'Wiki (Textile)', group: 'Documentos' },
  { id: 'jira', label: 'Wiki (Jira)', group: 'Documentos' },
  { id: 'sql-insert', label: 'SQL INSERT', group: 'SQL' },
  { id: 'sql-insert-ignore', label: 'SQL INSERT IGNORE', group: 'SQL' },
  { id: 'sql-replace', label: 'SQL REPLACE', group: 'SQL' },
  { id: 'sql-delete-insert', label: 'SQL DELETE + INSERT', group: 'SQL' },
  { id: 'sql-update', label: 'SQL UPDATE', group: 'SQL' },
]
const groups = [...new Set(formats.map((f) => f.group))]

const KEY = 'dbstudio:export-options'
function defaults(): ExportOptions {
  return { format: 'csv', separator: ';', encloser: '"', lineTerminator: '\\n', nullText: '\\N', header: true, removeLineBreaks: false, table: '', query: '', skipAutoIncrement: false, rowsPerInsert: 100 }
}
const opts = ref<ExportOptions>(defaults())
const scope = ref<'selected' | 'loaded' | 'full'>('loaded')
const dest = ref<'clipboard' | 'file'>('file')
const busy = ref(false)

watch(st, (s) => {
  if (!s) return
  try {
    opts.value = { ...defaults(), ...(JSON.parse(localStorage.getItem(KEY) ?? '{}') as Partial<ExportOptions>) }
  } catch {
    opts.value = defaults()
  }
  opts.value.table = s.table ?? ''
  opts.value.query = s.query ?? ''
  const sel = s.selected?.().length ?? 0
  scope.value = sel > 1 ? 'selected' : 'loaded'
}, { immediate: true })

const isSql = computed(() => opts.value.format.startsWith('sql'))
const isDelimited = computed(() => ['csv', 'tsv', 'excel'].includes(opts.value.format))
const counts = computed(() => ({ selected: st.value?.selected?.().length ?? 0, loaded: st.value?.rows().length ?? 0 }))
const needsKey = computed(() => (opts.value.format === 'sql-update' || opts.value.format === 'sql-delete-insert') && !st.value?.columns.some((c) => c.key))
watch(scope, (s) => { if (s === 'full') dest.value = 'file' })

const unescape = (s: string) => s.replace(/\\t/g, '\t').replace(/\\r/g, '\r').replace(/\\n/g, '\n')
function payload(): ExportOptions {
  const o = { ...opts.value }
  o.separator = unescape(o.separator)
  o.lineTerminator = unescape(o.lineTerminator)
  return o
}

async function run() {
  const s = st.value
  if (!s) return
  try {
    localStorage.setItem(KEY, JSON.stringify({ ...opts.value, table: '', query: '' }))
  } catch {
    // ignore
  }
  busy.value = true
  try {
    const o = payload()
    if (scope.value === 'full' && s.full) {
      const id = s.full.kind === 'query'
        ? await backend.exportQueryToFile(s.full.connectionId, s.full.database, s.full.sql, o, s.title)
        : await backend.exportTableToFile(s.full.connectionId, s.full.spec, o)
      if (id) {
        tasks.track(id)
        ui.exportDialog = null
      }
      return
    }
    const rows = scope.value === 'selected' ? (s.selected?.() ?? []) : s.rows()
    if (dest.value === 'clipboard') {
      const text = await backend.formatRows(s.columns, rows, o)
      await navigator.clipboard.writeText(text)
      ui.toast('success', `${formatCount(rows.length)} linhas copiadas como ${formats.find((f) => f.id === o.format)?.label}.`)
    } else {
      const path = await backend.exportRowsToFile(s.columns, rows, o, s.title)
      if (!path) return
      ui.toast('success', `Exportado para ${path}`)
    }
    ui.exportDialog = null
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Modal v-if="st" :title="`Exportar — ${st.title}`" :width="560" @close="ui.exportDialog = null">
    <div class="form">
      <label class="field"><span>Formato</span>
        <select v-model="opts.format" class="select">
          <optgroup v-for="g in groups" :key="g" :label="g">
            <option v-for="f in formats.filter((x) => x.group === g)" :key="f.id" :value="f.id">{{ f.label }}</option>
          </optgroup>
        </select>
      </label>
      <fieldset>
        <legend>Linhas</legend>
        <label class="check"><input v-model="scope" type="radio" value="selected" :disabled="counts.selected < 1" /> Selecionadas ({{ formatCount(counts.selected) }})</label>
        <label class="check"><input v-model="scope" type="radio" value="loaded" /> Carregadas na grade ({{ formatCount(counts.loaded) }})</label>
        <label v-if="st.full" class="check" title="A consulta é executada de novo e todas as linhas vão direto para o arquivo, sem limite"><input v-model="scope" type="radio" value="full" /> Todas — executar de novo no servidor (sem limite)</label>
      </fieldset>
      <fieldset>
        <legend>Destino</legend>
        <label class="check"><input v-model="dest" type="radio" value="file" /> Arquivo</label>
        <label class="check"><input v-model="dest" type="radio" value="clipboard" :disabled="scope === 'full'" /> Área de transferência</label>
      </fieldset>
      <div v-if="isDelimited" class="grid">
        <label class="field"><span>Separador</span><input v-model="opts.separator" class="input mono" placeholder="; , \t" /></label>
        <label class="field"><span>Delimitador de texto</span><input v-model="opts.encloser" class="input mono" maxlength="1" /></label>
        <label class="field"><span>Fim de linha</span>
          <select v-model="opts.lineTerminator" class="select"><option value="\n">LF (Unix)</option><option value="\r\n">CRLF (Windows)</option><option value="\r">CR</option></select>
        </label>
        <label class="field"><span>Texto do NULL</span><input v-model="opts.nullText" class="input mono" /></label>
      </div>
      <div v-if="isSql" class="grid">
        <label class="field wide"><span>Tabela de destino</span><input v-model="opts.table" class="input mono" placeholder="db.tabela" /></label>
        <label v-if="opts.format !== 'sql-update' && opts.format !== 'sql-delete-insert'" class="field"><span>Linhas por INSERT</span><input v-model.number="opts.rowsPerInsert" type="number" min="1" class="input" /></label>
        <label class="check wide"><input v-model="opts.skipAutoIncrement" type="checkbox" /> Omitir colunas AUTO_INCREMENT</label>
      </div>
      <p v-if="needsKey" class="warn">O resultado não tem colunas-chave conhecidas: o WHERE usará todas as colunas.</p>
      <label v-if="!isSql && opts.format !== 'json' && opts.format !== 'jsonl'" class="check"><input v-model="opts.header" type="checkbox" :disabled="!isDelimited" /> Incluir nomes das colunas</label>
      <label class="check"><input v-model="opts.removeLineBreaks" type="checkbox" /> Remover quebras de linha dos valores</label>
    </div>
    <template #footer>
      <button class="btn" @click="ui.exportDialog = null">Cancelar</button>
      <button class="btn primary" :disabled="busy" @click="run"><span v-if="busy" class="spinner" /> {{ dest === 'clipboard' ? 'Copiar' : 'Exportar…' }}</button>
    </template>
  </Modal>
</template>

<style scoped>
.form {
  display: flex;
  flex-direction: column;
  gap: 10px;
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
.grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
}
.wide {
  grid-column: span 2;
}
.warn {
  margin: 0;
  color: var(--warning);
  font-size: 12px;
}
</style>

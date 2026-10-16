<script setup lang="ts">
// Editing of a query result that maps to a single table with a key
// (HeidiSQL "editable query results"). Changes are saved in one
// transaction and applied to the loaded rows without re-running the query.
import { computed, ref, watch } from 'vue'
import { useTabsStore, type Tab, type ResultSet } from '../../stores/tabs'
import { useUiStore } from '../../stores/ui'
import { backend } from '../../api/backend'
import { toAppError } from '../../api/errors'
import type { Cell, CellValue, EditableInfo, RowEdit } from '../../api/types'
import EditableGrid from './EditableGrid.vue'
import CellEditorDialog from './CellEditorDialog.vue'
import Icon from '../common/Icon.vue'
import { currentValue, editorKindOf, isCellValue, makeRow, parseEnumValues, rowEdit, rowIsDirty, type GridColumn, type GridRow } from '../../utils/grid'
import { withConfirmation } from '../../composables/objectActions'

const props = defineProps<{ tab: Tab; result: ResultSet; info: EditableInfo }>()
const emit = defineEmits<{ close: [] }>()
const ui = useUiStore()
const tabs = useTabsStore()
/** The result as stored in the tabs store (mutated after saving). */
function storeResult(): ResultSet | undefined {
  return tabs.find(props.tab.id)?.results.find((r) => r === props.result || r.sql === props.result.sql)
}

const cols = computed<GridColumn[]>(() => props.result.columns.map((rc, i) => {
  const src = props.info.columns?.[i] ?? ''
  const ci = src ? (props.info.info ?? []).find((c) => c.name === src) : undefined
  const editor = ci ? editorKindOf(ci) : 'text'
  return {
    ...rc, name: src || rc.name, info: ci, editor,
    options: editor === 'enum' || editor === 'set' ? parseEnumValues(ci!.columnType) : undefined,
    editable: !!ci && !/VIRTUAL|STORED/i.test(ci.extra ?? ''),
  }
}))
const visible = computed(() => props.result.columns.map((_, i) => i))
const rows = ref<GridRow[]>([])

function sync() {
  const have = rows.value.length
  for (let i = have; i < props.result.rows.length; i++) {
    const trunc: number[] = []
    props.result.columns.forEach((_, c) => props.result.truncated.has(`${i}:${c}`) && trunc.push(c))
    rows.value.push(makeRow(props.result.rows[i]!, trunc))
  }
}
watch(() => props.result.rows.length, sync, { immediate: true })

const pending = computed(() => rows.value.filter(rowIsDirty).length)
const saving = ref(false)
const failure = ref<{ message: string; sql: string } | null>(null)
const editor = ref<{ row: number; col: number } | null>(null)
const grid = ref<InstanceType<typeof EditableGrid> | null>(null)

function setCell(row: number, col: number, v: CellValue) {
  const r = rows.value[row]
  if (!r || r.deleted) return
  const edits = new Map(r.edits)
  const o = r.orig?.[col] ?? null
  if (v.kind === 'value' && o !== null && String(o) === v.text && !r.truncated.has(col)) edits.delete(col)
  else edits.set(col, v)
  r.edits = edits
}

function onAction(name: string) {
  const sel = grid.value?.selectedRows() ?? []
  if (name === 'delete') for (const i of sel) { const r = rows.value[i]; if (r) r.deleted = !r.deleted }
  if (name === 'undo') for (const i of sel) { const r = rows.value[i]; if (r) { r.edits = new Map(); r.deleted = false } }
  if (name === 'null') {
    const c = grid.value?.cursor()
    if (c && cols.value[c.col]?.info?.nullable) setCell(c.row, c.col, { kind: 'null', text: '' })
  }
  if (name === 'save') void save()
}

function cellOf(v: CellValue | Cell): Cell {
  if (!isCellValue(v)) return v
  if (v.kind === 'null') return null
  if (v.kind === 'hex') return '0x' + v.text.toUpperCase()
  return v.text
}

async function save() {
  if (!pending.value || saving.value) return
  const edits: RowEdit[] = []
  const order: number[] = []
  rows.value.forEach((r, i) => {
    const e = rowEdit(r, cols.value, props.info.key)
    if (e) {
      edits.push(e)
      order.push(i)
    }
  })
  if (!edits.length) return
  if (edits.some((e) => e.values.some((v) => v.value.kind === 'default' || v.value.kind === 'expression'))) {
    ui.toast('info', 'Valores DEFAULT/expressão são gravados, mas o resultado mostra o texto digitado até a consulta ser executada de novo.', 6000)
  }
  saving.value = true
  failure.value = null
  try {
    const conn = props.tab.connectionId ?? ''
    const res = await withConfirmation((c) => backend.applyRowEdits(conn, props.info.database, props.info.table, edits, c))
    if (!res) return
    if (!res.applied) {
      const list = res.results ?? []
      const i = list.findIndex((r) => r.error)
      failure.value = { message: list[i]?.error?.message ?? 'falha', sql: list[i]?.sql ?? '' }
      return
    }
    // Apply locally: edited values become the loaded values; deleted rows go away.
    const target = storeResult()
    const dead = new Set<number>()
    order.forEach((ri) => {
      const r = rows.value[ri]!
      if (r.deleted) {
        dead.add(ri)
        return
      }
      const vals = [...(r.orig ?? [])]
      for (const [c, v] of r.edits) vals[c] = cellOf(v)
      if (target) target.rows[ri] = vals
      r.orig = vals
      for (const c of r.edits.keys()) r.truncated.delete(c)
      r.edits = new Map()
    })
    if (dead.size) {
      const keep = (_: unknown, i: number) => !dead.has(i)
      if (target) target.rows = target.rows.filter(keep)
      rows.value = rows.value.filter(keep)
    }
    ui.toast('success', `${edits.length} ${edits.length === 1 ? 'alteração salva' : 'alterações salvas'} em ${props.info.table}.`, 2500)
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  } finally {
    saving.value = false
  }
}

async function close() {
  if (pending.value && !(await ui.confirm({ title: 'Alterações não salvas', message: 'Descartar as alterações deste resultado?', confirmLabel: 'Descartar', danger: true }))) return
  emit('close')
}

function editorSave(v: CellValue) {
  const e = editor.value
  if (e) setCell(e.row, e.col, v)
  editor.value = null
}
</script>

<template>
  <div class="qre">
    <div class="bar">
      <Icon name="edit" :size="12" />
      <span>Editando <strong>{{ info.database }}.{{ info.table }}</strong> pela chave {{ (info.key.columns ?? []).join(', ') }} · colunas calculadas ficam somente leitura</span>
      <span class="spacer" />
      <button class="btn small" title="Excluir linhas selecionadas (Ctrl+Delete)" @click="onAction('delete')"><Icon name="trash" :size="11" /> Excluir</button>
      <button class="btn small" :class="{ primary: pending }" :disabled="!pending || saving" title="Ctrl+S" @click="save"><Icon name="save" :size="11" /> Salvar<template v-if="pending"> ({{ pending }})</template></button>
      <button class="btn small" @click="close">Fechar edição</button>
    </div>
    <div v-if="failure" class="err selectable"><Icon name="alert" :size="12" /> Nada foi salvo: {{ failure.message }} <code class="mono">{{ failure.sql }}</code></div>
    <div class="body">
      <EditableGrid
        ref="grid" :columns="cols" :visible="visible" :rows="rows" @commit="setCell" @open-editor="(r, c) => (editor = { row: r, col: c })"
        @action="onAction" @menu="() => undefined"
      />
    </div>
    <CellEditorDialog
      v-if="editor && cols[editor.col]" :column="cols[editor.col]!" :value="currentValue(rows[editor.row]!, editor.col)" :read-only="!cols[editor.col]!.editable"
      :partial="rows[editor.row]!.truncated.has(editor.col)" @save="editorSave" @close="editor = null"
    />
  </div>
</template>

<style scoped>
.qre {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
  font-size: 12px;
  background: var(--warning-soft);
  border-bottom: 1px solid var(--border);
}
.spacer {
  flex: 1;
}
.err {
  padding: 4px 8px;
  font-size: 12px;
  background: var(--danger-soft);
}
.body {
  flex: 1;
  min-height: 0;
}
</style>

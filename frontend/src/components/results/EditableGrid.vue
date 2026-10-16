<script setup lang="ts">
// Editable, virtualized grid used by the table data tab (and editable query
// results). It edits cells inline and reports changes to the parent, which
// keeps them pending until saved.
import { computed, nextTick, ref, watch } from 'vue'
import { useVirtualizer } from '@tanstack/vue-virtual'
import type { CellValue, LookupValue, SortSpec } from '../../api/types'
import { useUiStore } from '../../stores/ui'
import { currentValue, displayValue, isCellValue, type GridColumn, type GridRow } from '../../utils/grid'
import { groupDigits, unixToLocal } from '../../utils/display'
import { useSettingsStore } from '../../stores/settings'

const props = withDefaults(defineProps<{
  columns: GridColumn[]
  /** Indexes of the columns to show, in order. */
  visible: number[]
  rows: GridRow[]
  sort?: SortSpec[]
  readOnly?: boolean
  hasMore?: boolean
  loadingMore?: boolean
  lookups?: Record<string, LookupValue[]>
  /** Column names shown as UNIX timestamps (display only). */
  timestampCols?: Set<string>
}>(), { sort: () => [], readOnly: false, hasMore: false, loadingMore: false, lookups: () => ({}), timestampCols: () => new Set<string>() })

const emit = defineEmits<{
  sort: [column: string, additive: boolean]
  loadMore: []
  commit: [row: number, col: number, value: CellValue]
  openEditor: [row: number, col: number]
  needLookup: [col: number]
  menu: [e: MouseEvent, row: number, col: number]
  action: [name: 'insert' | 'duplicate' | 'delete' | 'null' | 'save' | 'refresh' | 'undo']
}>()
const ui = useUiStore()
const settings = useSettingsStore()

const ROW_H = 26
const scrollEl = ref<HTMLElement | null>(null)
const widths = ref<Record<number, number>>({})
const cursor = ref<{ row: number; col: number } | null>(null) // col = column index (not visible position)
const selected = ref(new Set<number>())
const anchor = ref<number | null>(null)
const editing = ref<{ row: number; col: number; text: string } | null>(null)
const editorEl = ref<HTMLInputElement | HTMLSelectElement | null>(null)
const NULL_OPTION = '__anansi_null__'
function setEditorEl(el: unknown) {
  editorEl.value = (el as HTMLInputElement | HTMLSelectElement | null) ?? null
}

const rowNumWidth = computed(() => Math.max(52, String(props.rows.length).length * 8 + 28))

function measureCol(ci: number): number {
  const c = props.columns[ci]!
  let max = c.name.length + Math.ceil(c.type.length * 0.7) + 3
  for (const r of props.rows.slice(0, 200)) {
    const v = currentValue(r, ci)
    const len = Math.min(displayValue(v).text.length, 60)
    if (len > max) max = len
  }
  return Math.min(settings.prefs.gridMaxColumnWidth, Math.max(64, Math.ceil(max * 7.4) + 22))
}

function widthOf(ci: number) {
  return widths.value[ci] ?? (widths.value[ci] = measureCol(ci))
}
watch(() => props.columns, () => (widths.value = {}))

const template = computed(() => `${rowNumWidth.value}px ${props.visible.map((ci) => `${widthOf(ci)}px`).join(' ')}`)
const totalWidth = computed(() => rowNumWidth.value + props.visible.reduce((a, ci) => a + widthOf(ci), 0))

const virtualizer = useVirtualizer(computed(() => ({
  count: props.rows.length,
  getScrollElement: () => scrollEl.value,
  estimateSize: () => ROW_H,
  overscan: 20,
})))
const items = computed(() => virtualizer.value.getVirtualItems())

function onScroll() {
  const el = scrollEl.value
  if (el && props.hasMore && !props.loadingMore && el.scrollTop + el.clientHeight > el.scrollHeight - ROW_H * 30) emit('loadMore')
}

// ---- column resize ----
function startResize(e: MouseEvent, ci: number) {
  e.preventDefault()
  e.stopPropagation()
  const x0 = e.clientX
  const w0 = widthOf(ci)
  const move = (ev: MouseEvent) => (widths.value = { ...widths.value, [ci]: Math.max(40, Math.min(1400, w0 + ev.clientX - x0)) })
  const up = () => { window.removeEventListener('mousemove', move); window.removeEventListener('mouseup', up) }
  window.addEventListener('mousemove', move)
  window.addEventListener('mouseup', up)
}

function sortState(name: string): { dir: 'asc' | 'desc'; pos: number } | null {
  const i = props.sort.findIndex((s) => s.column === name)
  if (i < 0) return null
  return { dir: props.sort[i]!.desc ? 'desc' : 'asc', pos: i + 1 }
}

// ---- selection ----
function focusGrid() {
  scrollEl.value?.focus({ preventScroll: true })
}

function selectRow(row: number, e?: MouseEvent | KeyboardEvent) {
  if (e?.shiftKey && anchor.value !== null) {
    const [a, b] = anchor.value < row ? [anchor.value, row] : [row, anchor.value]
    const s = new Set<number>()
    for (let i = a; i <= b; i++) s.add(i)
    selected.value = s
  } else if (e && (e.ctrlKey || e.metaKey) && e instanceof MouseEvent) {
    const s = new Set(selected.value)
    if (s.has(row)) s.delete(row)
    else s.add(row)
    selected.value = s
    anchor.value = row
  } else {
    selected.value = new Set([row])
    anchor.value = row
  }
}

function onCellDown(e: MouseEvent, row: number, ci: number) {
  if (editing.value && (editing.value.row !== row || editing.value.col !== ci)) commitEdit()
  if (e.button === 2 && selected.value.has(row)) {
    cursor.value = { row, col: ci }
    return
  }
  cursor.value = { row, col: ci }
  selectRow(row, e)
  focusGrid()
}

function onHeaderRowDown(e: MouseEvent, row: number) {
  if (editing.value) commitEdit()
  cursor.value = { row, col: cursor.value?.col ?? props.visible[0] ?? 0 }
  selectRow(row, e)
  focusGrid()
}

function ensureVisible(row: number, ci?: number) {
  virtualizer.value.scrollToIndex(row, { align: 'auto' })
  if (ci === undefined || !scrollEl.value) return
  // Horizontal: keep the cursor column in view.
  let left = rowNumWidth.value
  for (const v of props.visible) {
    if (v === ci) break
    left += widthOf(v)
  }
  const el = scrollEl.value
  const w = widthOf(ci)
  if (left < el.scrollLeft + rowNumWidth.value) el.scrollLeft = left - rowNumWidth.value
  else if (left + w > el.scrollLeft + el.clientWidth) el.scrollLeft = left + w - el.clientWidth
}

function moveCursor(dRow: number, dCol: number, e?: KeyboardEvent) {
  if (!props.rows.length || !props.visible.length) return
  const cur = cursor.value ?? { row: 0, col: props.visible[0]! }
  const row = Math.max(0, Math.min(props.rows.length - 1, cur.row + dRow))
  const vpos = Math.max(0, Math.min(props.visible.length - 1, props.visible.indexOf(cur.col) + dCol))
  const col = props.visible[vpos]!
  cursor.value = { row, col }
  if (dRow !== 0 || !selected.value.has(row)) selectRow(row, e?.shiftKey ? e : undefined)
  ensureVisible(row, col)
}

// ---- editing ----
function canEdit(row: number, ci: number): boolean {
  const r = props.rows[row]
  const c = props.columns[ci]
  return !props.readOnly && !!r && !!c && c.editable && !r.deleted
}

/** Editors that need the dialog (long, binary, structured or truncated). */
function needsDialog(row: number, ci: number): boolean {
  const c = props.columns[ci]!
  const r = props.rows[row]!
  if (r.truncated.has(ci) && !r.edits.has(ci)) return true
  if (['longtext', 'json', 'binary', 'set', 'spatial'].includes(c.editor)) return true
  const v = currentValue(r, ci)
  const t = isCellValue(v) ? v.text : v === null ? '' : String(v)
  return t.length > 200 || t.includes('\n')
}

function startEdit(row: number, ci: number, initial?: string) {
  if (!canEdit(row, ci)) return
  if (needsDialog(row, ci)) {
    emit('openEditor', row, ci)
    return
  }
  const c = props.columns[ci]!
  const v = currentValue(props.rows[row]!, ci)
  let text: string
  if (initial !== undefined) text = initial
  else if (isCellValue(v)) text = v.kind === 'value' || v.kind === 'hex' ? v.text : ''
  else text = v === null ? '' : String(v)
  if (c.fk) emit('needLookup', ci)
  if ((c.editor === 'enum' || c.editor === 'bool') && initial === undefined && (v === null || (isCellValue(v) && v.kind === 'null'))) text = NULL_OPTION
  editing.value = { row, col: ci, text }
  void nextTick(() => {
    const el = editorEl.value
    el?.focus()
    if (el instanceof HTMLInputElement) {
      if (initial !== undefined) el.setSelectionRange(el.value.length, el.value.length)
      else el.select()
    }
  })
}

function commitEdit() {
  const ed = editing.value
  if (!ed) return
  editing.value = null
  const r = props.rows[ed.row]
  const c = props.columns[ed.col]
  if (!r || !c) return
  let value: CellValue = { kind: 'value', text: ed.text }
  if ((c.editor === 'enum' || c.editor === 'bool') && ed.text === NULL_OPTION) value = { kind: 'null', text: '' }
  emit('commit', ed.row, ed.col, value)
}

function cancelEdit() {
  editing.value = null
  focusGrid()
}

function onEditorKey(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    cancelEdit()
  } else if (e.key === 'Enter' && !e.ctrlKey) {
    e.preventDefault()
    e.stopPropagation()
    commitEdit()
    focusGrid()
    moveCursor(e.shiftKey ? -1 : 1, 0)
  } else if (e.key === 'Tab') {
    e.preventDefault()
    e.stopPropagation()
    commitEdit()
    focusGrid()
    moveCursor(0, e.shiftKey ? -1 : 1)
  }
}

// ---- clipboard ----
function cellText(row: number, ci: number): string {
  const v = currentValue(props.rows[row]!, ci)
  if (isCellValue(v)) return v.kind === 'null' ? 'NULL' : v.kind === 'hex' ? '0x' + v.text : v.text
  return v === null ? 'NULL' : String(v)
}

function tsvField(s: string) {
  return /[\t\n\r"]/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s
}

async function copy() {
  let text: string
  if (selected.value.size > 1) {
    const rows = [...selected.value].sort((a, b) => a - b)
    text = rows.map((r) => props.visible.map((ci) => tsvField(cellText(r, ci))).join('\t')).join('\n')
  } else if (cursor.value) {
    text = cellText(cursor.value.row, cursor.value.col)
  } else return
  try {
    await navigator.clipboard.writeText(text)
    ui.toast('success', selected.value.size > 1 ? `${selected.value.size} linhas copiadas.` : 'Valor copiado.', 1500)
  } catch {
    ui.toast('error', 'Não foi possível acessar a área de transferência.')
  }
}

/** Pastes text; TSV fills several cells starting at the cursor. */
async function paste() {
  const cur = cursor.value
  if (!cur || props.readOnly) return
  let text: string
  try {
    text = await navigator.clipboard.readText()
  } catch {
    ui.toast('error', 'Não foi possível ler a área de transferência.')
    return
  }
  const lines = text.replace(/\r\n/g, '\n').replace(/\n$/, '').split('\n')
  const grid = lines.length > 1 || lines[0]!.includes('\t') ? lines.map((l) => l.split('\t')) : [[text]]
  const startV = props.visible.indexOf(cur.col)
  let count = 0
  grid.forEach((cells, dr) => {
    const row = cur.row + dr
    if (row >= props.rows.length) return
    cells.forEach((t, dc) => {
      const ci = props.visible[startV + dc]
      if (ci === undefined || !canEdit(row, ci)) return
      emit('commit', row, ci, t === 'NULL' || t === '\\N' ? { kind: 'null', text: '' } : { kind: 'value', text: t })
      count++
    })
  })
  if (count > 1) ui.toast('info', `${count} células coladas.`, 2000)
}

function onKey(e: KeyboardEvent) {
  if (editing.value) return
  const mod = e.ctrlKey || e.metaKey
  const k = e.key
  const cur = cursor.value
  if (mod && k.toLowerCase() === 'c') { e.preventDefault(); void copy(); return }
  if (mod && k.toLowerCase() === 'v') { e.preventDefault(); void paste(); return }
  if (mod && k.toLowerCase() === 's') { e.preventDefault(); emit('action', 'save'); return }
  if (mod && k.toLowerCase() === 'z') { e.preventDefault(); emit('action', 'undo'); return }
  if (mod && k.toLowerCase() === 'd') { e.preventDefault(); emit('action', 'duplicate'); return }
  if (mod && k.toLowerCase() === 'a') {
    e.preventDefault()
    selected.value = new Set(props.rows.map((_, i) => i))
    return
  }
  if (mod && e.shiftKey && k.toLowerCase() === 'n') { e.preventDefault(); emit('action', 'null'); return }
  if (mod && k === 'Delete') { e.preventDefault(); emit('action', 'delete'); return }
  if (k === 'Insert') { e.preventDefault(); emit('action', 'insert'); return }
  if (k === 'F5') { e.preventDefault(); emit('action', 'refresh'); return }
  switch (k) {
    case 'ArrowDown': e.preventDefault(); moveCursor(1, 0, e); return
    case 'ArrowUp': e.preventDefault(); moveCursor(-1, 0, e); return
    case 'ArrowLeft': e.preventDefault(); moveCursor(0, -1); return
    case 'ArrowRight': e.preventDefault(); moveCursor(0, 1); return
    case 'Tab': e.preventDefault(); moveCursor(0, e.shiftKey ? -1 : 1); return
    case 'PageDown': e.preventDefault(); moveCursor(20, 0, e); return
    case 'PageUp': e.preventDefault(); moveCursor(-20, 0, e); return
    case 'Home': e.preventDefault(); moveCursor(mod ? -1e9 : 0, -1e9); return
    case 'End': e.preventDefault(); moveCursor(mod ? 1e9 : 0, 1e9); return
    case 'Enter':
    case 'F2':
      e.preventDefault()
      if (cur) startEdit(cur.row, cur.col)
      return
    case 'Delete':
    case 'Backspace':
      if (cur && canEdit(cur.row, cur.col) && !needsDialog(cur.row, cur.col)) {
        e.preventDefault()
        startEdit(cur.row, cur.col, '')
      }
      return
  }
  // Typing starts editing with the typed character, like a spreadsheet.
  if (cur && k.length === 1 && !mod && !e.altKey && canEdit(cur.row, cur.col) && !needsDialog(cur.row, cur.col)) {
    const c = props.columns[cur.col]!
    if (c.editor === 'enum' || c.editor === 'bool') return
    e.preventDefault()
    startEdit(cur.row, cur.col, k)
  }
}

function onMenu(e: MouseEvent, row: number, ci: number) {
  e.preventDefault()
  if (editing.value) commitEdit()
  if (!selected.value.has(row)) selectRow(row)
  cursor.value = { row, col: ci }
  emit('menu', e, row, ci)
}

function cellDisplay(row: GridRow, ci: number) {
  const d = displayValue(currentValue(row, ci))
  const col = props.columns[ci]!
  if (!d.cls && col.numeric) {
    if (props.timestampCols.has(col.name)) {
      const ts = unixToLocal(d.text)
      if (ts) return { text: ts, cls: 'ts' }
    }
    if (settings.prefs.gridThousands) return { text: groupDigits(d.text), cls: '' }
  }
  const t = d.text.length > 500 ? d.text.slice(0, 500) + '…' : d.text
  return { text: t.replace(/\r?\n/g, ' ↵ '), cls: d.cls }
}

function lookupLabel(ci: number, v: string): string {
  const c = props.columns[ci]
  if (!c?.fk) return ''
  const hit = props.lookups[c.name]?.find((l) => String(l.value) === v)
  return hit?.label ?? ''
}

defineExpose({
  selectedRows: () => [...selected.value].sort((a, b) => a - b),
  cursor: () => cursor.value,
  setCursor: (row: number, col: number) => {
    cursor.value = { row, col }
    selected.value = new Set([row])
    anchor.value = row
    void nextTick(() => { ensureVisible(row, col); focusGrid() })
  },
  clearSelection: () => { selected.value = new Set(); cursor.value = null },
  focus: focusGrid,
  startEdit,
  copy,
})
</script>

<template>
  <div
    ref="scrollEl" class="grid" tabindex="0" role="grid" :aria-rowcount="rows.length + 1" :aria-colcount="visible.length + 1" :aria-readonly="readOnly"
    @scroll.passive="onScroll" @keydown="onKey"
  >
    <div class="inner" :style="{ width: totalWidth + 'px', height: virtualizer.getTotalSize() + ROW_H + 'px' }">
      <div class="header" role="row" :style="{ gridTemplateColumns: template, width: totalWidth + 'px' }">
        <div class="hcell rownum" role="columnheader">#</div>
        <div
          v-for="ci in visible" :key="ci" class="hcell" role="columnheader" :class="{ num: columns[ci]!.numeric, sorted: !!sortState(columns[ci]!.name) }"
          :aria-sort="sortState(columns[ci]!.name)?.dir === 'desc' ? 'descending' : sortState(columns[ci]!.name) ? 'ascending' : 'none'"
          :title="`${columns[ci]!.name} — ${columns[ci]!.info?.columnType ?? columns[ci]!.type}${columns[ci]!.info?.nullable === false ? ' NOT NULL' : ''}${columns[ci]!.info?.comment ? '\n' + columns[ci]!.info!.comment : ''}\nClique: ordenar · Shift+clique: ordenação múltipla`"
          @click="emit('sort', columns[ci]!.name, $event.shiftKey)"
        >
          <span v-if="columns[ci]!.info?.key === 'PRI'" class="pk" title="Chave primária">🔑</span>
          <span v-else-if="columns[ci]!.fk" class="fkmark" title="Chave estrangeira">↗</span>
          <span class="ellipsis name">{{ columns[ci]!.name }}</span>
          <span class="type">{{ (columns[ci]!.info?.dataType ?? columns[ci]!.type).toLowerCase() }}</span>
          <span v-if="sortState(columns[ci]!.name)" class="sort">{{ sortState(columns[ci]!.name)!.dir === 'desc' ? '▼' : '▲' }}<sub v-if="sort.length > 1">{{ sortState(columns[ci]!.name)!.pos }}</sub></span>
          <span class="resize" @mousedown="startResize($event, ci)" @click.stop @dblclick.stop="widths = { ...widths, [ci]: measureCol(ci) }" />
        </div>
      </div>
      <div
        v-for="it in items" :key="rows[it.index]!.uid" class="row" role="row"
        :class="{ odd: it.index % 2 === 1, sel: selected.has(it.index), deleted: rows[it.index]!.deleted, inserted: rows[it.index]!.orig === null, failed: !!rows[it.index]!.error }"
        :style="{ transform: `translateY(${it.start + ROW_H}px)`, gridTemplateColumns: template, width: totalWidth + 'px' }"
      >
        <div class="cell rownum" role="rowheader" :title="rows[it.index]!.error" @mousedown="onHeaderRowDown($event, it.index)">
          <span v-if="rows[it.index]!.deleted" class="mark del">×</span>
          <span v-else-if="rows[it.index]!.orig === null" class="mark new">+</span>
          <span v-else-if="rows[it.index]!.edits.size" class="mark mod">✎</span>
          {{ it.index + 1 }}
        </div>
        <div
          v-for="ci in visible" :key="ci" class="cell" role="gridcell"
          :class="[cellDisplay(rows[it.index]!, ci).cls, {
            num: columns[ci]!.numeric, bin: columns[ci]!.binary, cur: cursor?.row === it.index && cursor?.col === ci,
            modified: rows[it.index]!.edits.has(ci), editing: editing?.row === it.index && editing?.col === ci,
          }]"
          :aria-selected="cursor?.row === it.index && cursor?.col === ci"
          @mousedown="onCellDown($event, it.index, ci)" @dblclick="startEdit(it.index, ci)" @contextmenu="onMenu($event, it.index, ci)"
        >
          <template v-if="editing && editing.row === it.index && editing.col === ci">
            <select
              v-if="columns[ci]!.editor === 'enum' || columns[ci]!.editor === 'bool'" :ref="setEditorEl" v-model="editing.text" class="ed"
              @keydown="onEditorKey" @blur="commitEdit" @change="commitEdit(); focusGrid()"
            >
              <option v-if="columns[ci]!.info?.nullable" :value="NULL_OPTION">NULL</option>
              <template v-if="columns[ci]!.editor === 'bool'"><option value="1">1</option><option value="0">0</option></template>
              <option v-for="o in columns[ci]!.options" v-else :key="o" :value="o">{{ o }}</option>
            </select>
            <template v-else>
              <input :ref="setEditorEl" v-model="editing.text" class="ed" :list="columns[ci]!.fk ? `lk-${ci}` : undefined" spellcheck="false" @keydown="onEditorKey" @blur="commitEdit" />
              <datalist v-if="columns[ci]!.fk" :id="`lk-${ci}`">
                <option v-for="l in lookups[columns[ci]!.name] ?? []" :key="String(l.value)" :value="String(l.value)">{{ l.label }}</option>
              </datalist>
            </template>
          </template>
          <template v-else>
            <span class="v">{{ cellDisplay(rows[it.index]!, ci).text }}</span>
            <span v-if="columns[ci]!.fk && lookupLabel(ci, cellText(it.index, ci))" class="lk">{{ lookupLabel(ci, cellText(it.index, ci)) }}</span>
            <span v-if="rows[it.index]!.truncated.has(ci) && !rows[it.index]!.edits.has(ci)" class="trunc" title="Valor parcial: abra o editor para carregar o valor completo">…</span>
          </template>
        </div>
      </div>
    </div>
    <div v-if="hasMore" class="more">
      <span v-if="loadingMore" class="spinner" /> <span v-else class="faint">role para carregar mais</span>
    </div>
  </div>
</template>

<style scoped>
.grid {
  position: relative;
  height: 100%;
  overflow: auto;
  font-family: var(--font-mono);
  font-size: 12px;
  outline: none;
  background: var(--bg);
}
.grid:focus-visible {
  box-shadow: inset 0 0 0 1px var(--accent);
}
.inner {
  position: relative;
}
.header {
  position: sticky;
  top: 0;
  z-index: 2;
  display: grid;
  height: 26px;
  background: var(--bg-elev);
  border-bottom: 1px solid var(--border-strong);
}
.hcell {
  position: relative;
  display: flex;
  align-items: baseline;
  gap: 5px;
  padding: 0 8px;
  line-height: 26px;
  border-right: 1px solid var(--border);
  font-family: var(--font-ui);
  font-weight: 600;
  overflow: hidden;
  cursor: pointer;
}
.hcell.sorted {
  color: var(--accent);
}
.hcell .name {
  min-width: 0;
}
.hcell .type {
  font-weight: 400;
  font-size: 10px;
  color: var(--text-faint);
  white-space: nowrap;
}
.pk,
.fkmark {
  font-size: 10px;
  color: var(--warning);
}
.sort {
  font-size: 9px;
}
.resize {
  position: absolute;
  top: 0;
  right: -3px;
  width: 7px;
  height: 100%;
  cursor: col-resize;
  z-index: 1;
}
.row {
  position: absolute;
  top: 0;
  left: 0;
  display: grid;
  height: 26px;
}
.row.odd {
  background: var(--grid-stripe);
}
.row:hover {
  background: var(--bg-hover);
}
.row.sel {
  background: var(--selection);
}
.row.deleted .cell:not(.rownum) {
  text-decoration: line-through;
  color: var(--danger);
  opacity: 0.7;
}
.row.inserted {
  background: var(--success-soft);
}
.row.failed .rownum {
  color: var(--danger);
  font-weight: 700;
}
.cell {
  position: relative;
  padding: 0 8px;
  line-height: 26px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  border-right: 1px solid var(--border);
  border-bottom: 1px solid var(--border);
  cursor: default;
}
.cell.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.cell.bin {
  color: var(--text-muted);
}
.cell.ts {
  color: var(--accent-text);
}
.cell.isnull {
  color: var(--null);
  font-style: italic;
}
.cell.isdefault,
.cell.isexpr {
  color: var(--accent);
  font-style: italic;
}
.cell.modified {
  background: var(--warning-soft);
}
.cell.cur {
  box-shadow: inset 0 0 0 2px var(--accent);
}
.cell.editing {
  padding: 0;
  overflow: visible;
}
.ed {
  width: 100%;
  height: 100%;
  padding: 0 7px;
  border: none;
  outline: 2px solid var(--accent);
  outline-offset: -2px;
  background: var(--bg-input);
  color: var(--text);
  font: inherit;
  user-select: text;
}
.rownum {
  position: sticky;
  left: 0;
  z-index: 1;
  text-align: right;
  color: var(--text-faint);
  background: var(--bg-elev);
  cursor: pointer;
}
.hcell.rownum {
  z-index: 3;
  cursor: default;
}
.mark {
  float: left;
  font-weight: 700;
}
.mark.del {
  color: var(--danger);
}
.mark.new {
  color: var(--success);
}
.mark.mod {
  color: var(--warning);
}
.lk {
  margin-left: 6px;
  color: var(--text-faint);
  font-family: var(--font-ui);
}
.trunc {
  margin-left: 4px;
  color: var(--warning);
  font-weight: 700;
}
.more {
  position: sticky;
  left: 0;
  display: flex;
  justify-content: center;
  padding: 4px;
  font-family: var(--font-ui);
}
</style>

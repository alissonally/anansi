<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useVirtualizer } from '@tanstack/vue-virtual'
import type { Cell, ResultColumn } from '../../api/types'
import type { ResultSet } from '../../stores/tabs'
import { useUiStore } from '../../stores/ui'
import ContextMenu, { type MenuItem } from '../common/ContextMenu.vue'
import CellViewer from './CellViewer.vue'
import { cellToText, rowsToTsv, rowToInsert } from '../../utils/cells'
import { groupDigits } from '../../utils/display'
import { useSettingsStore } from '../../stores/settings'

const props = defineProps<{ result: ResultSet; tableHint?: string }>()
const emit = defineEmits<{ loadMore: [] }>()
const ui = useUiStore()
const settings = useSettingsStore()

const ROW_H = 26
const scrollEl = ref<HTMLElement | null>(null)
const widths = ref<number[]>([])
const selected = ref<{ row: number; col: number } | null>(null)
const viewer = ref<{ col: number; row: number } | null>(null)
const menu = ref<{ x: number; y: number; items: MenuItem[] } | null>(null)

const rowNumWidth = computed(() => Math.max(44, String(props.result.rows.length).length * 8 + 20))

// Local sort of the loaded rows (the query itself is not re-run).
const sortState = ref<{ col: number; desc: boolean } | null>(null)
const order = computed<number[]>(() => {
  const idx = props.result.rows.map((_, i) => i)
  const s = sortState.value
  if (!s) return idx
  const col = props.result.columns[s.col]
  const num = col?.numeric
  const dir = s.desc ? -1 : 1
  return idx.sort((a, b) => {
    const x = props.result.rows[a]![s.col] ?? null
    const y = props.result.rows[b]![s.col] ?? null
    if (x === y) return a - b
    if (x === null) return 1
    if (y === null) return -1
    if (num) {
      const d = Number(x) - Number(y)
      if (!Number.isNaN(d) && d !== 0) return d * dir
    }
    return String(x).localeCompare(String(y), 'pt-BR', { numeric: true }) * dir || a - b
  })
})
function toggleSort(col: number) {
  const s = sortState.value
  sortState.value = !s || s.col !== col ? { col, desc: false } : s.desc ? null : { col, desc: true }
  selected.value = null
}
/** Row of the original result shown at display position i. */
function rowAt(i: number) {
  return props.result.rows[order.value[i] ?? i]
}

function measure(cols: ResultColumn[], rows: Cell[][]): number[] {
  const sample = rows.slice(0, 200)
  return cols.map((c, i) => {
    // Header shows the name plus the type label.
    let max = c.name.length + Math.ceil(c.type.length * 0.75) + 3
    for (const r of sample) {
      const v = r[i]
      const len = v === null || v === undefined ? 4 : Math.min(String(v).length, 60)
      if (len > max) max = len
    }
    return Math.min(settings.prefs.gridMaxColumnWidth, Math.max(64, Math.ceil(max * 7.4) + 22))
  })
}

watch(
  () => props.result,
  (r) => {
    widths.value = measure(r.columns, r.rows)
    selected.value = null
    void nextTick(() => scrollEl.value?.scrollTo({ top: 0, left: 0 }))
  },
  { immediate: true },
)

const template = computed(() => `${rowNumWidth.value}px ${widths.value.map((w) => `${w}px`).join(' ')}`)
const totalWidth = computed(() => rowNumWidth.value + widths.value.reduce((a, b) => a + b, 0))

const virtualizer = useVirtualizer(
  computed(() => ({
    count: props.result.rows.length,
    getScrollElement: () => scrollEl.value,
    estimateSize: () => ROW_H,
    overscan: 20,
  })),
)
const items = computed(() => virtualizer.value.getVirtualItems())
const totalHeight = computed(() => virtualizer.value.getTotalSize())

function onScroll() {
  const el = scrollEl.value
  if (!el) return
  if (props.result.hasMore && !props.result.loadingMore && el.scrollTop + el.clientHeight > el.scrollHeight - ROW_H * 30) {
    emit('loadMore')
  }
}

// ---- column resize ----
function startResize(e: MouseEvent, col: number) {
  e.preventDefault()
  e.stopPropagation()
  const startX = e.clientX
  const start = widths.value[col] ?? 100
  const move = (ev: MouseEvent) => {
    const w = Math.max(40, Math.min(1200, start + ev.clientX - startX))
    widths.value.splice(col, 1, w)
  }
  const up = () => {
    window.removeEventListener('mousemove', move)
    window.removeEventListener('mouseup', up)
  }
  window.addEventListener('mousemove', move)
  window.addEventListener('mouseup', up)
}

function autoFit(col: number) {
  const c = props.result.columns[col]
  if (!c) return
  widths.value.splice(col, 1, measure([c], props.result.rows.map((r) => [r[col] ?? null]))[0] ?? 100)
}

// ---- selection, keyboard and clipboard ----
function select(row: number, col: number) {
  selected.value = { row, col }
  scrollEl.value?.focus({ preventScroll: true })
}

function ensureVisible(row: number) {
  virtualizer.value.scrollToIndex(row, { align: 'auto' })
}

async function copy(text: string, what: string) {
  try {
    await navigator.clipboard.writeText(text)
    ui.toast('success', `${what} copiado.`, 2000)
  } catch {
    ui.toast('error', 'Não foi possível acessar a área de transferência.')
  }
}

function onKey(e: KeyboardEvent) {
  const s = selected.value
  const rows = props.result.rows.length
  const cols = props.result.columns.length
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'c' && s) {
    e.preventDefault()
    const row = rowAt(s.row)
    if (!row) return
    if (e.shiftKey) void copy(rowsToTsv(props.result.columns, [row], false), 'Linha')
    else void copy(cellToText(row[s.col] ?? null), 'Valor')
    return
  }
  if (!s) {
    if (['ArrowDown', 'ArrowUp', 'ArrowLeft', 'ArrowRight'].includes(e.key) && rows > 0) {
      e.preventDefault()
      select(0, 0)
    }
    return
  }
  let { row, col } = s
  switch (e.key) {
    case 'ArrowDown': row = Math.min(rows - 1, row + 1); break
    case 'ArrowUp': row = Math.max(0, row - 1); break
    case 'ArrowLeft': col = Math.max(0, col - 1); break
    case 'ArrowRight': col = Math.min(cols - 1, col + 1); break
    case 'PageDown': row = Math.min(rows - 1, row + 20); break
    case 'PageUp': row = Math.max(0, row - 20); break
    case 'Home': if (e.ctrlKey) row = 0; col = 0; break
    case 'End': if (e.ctrlKey) row = rows - 1; col = cols - 1; break
    case 'Enter': viewer.value = { row, col }; e.preventDefault(); return
    default: return
  }
  e.preventDefault()
  selected.value = { row, col }
  ensureVisible(row)
}

function openMenu(e: MouseEvent, row: number, col: number) {
  e.preventDefault()
  select(row, col)
  const r = rowAt(row)
  if (!r) return
  const cols = props.result.columns
  menu.value = {
    x: e.clientX,
    y: e.clientY,
    items: [
      { label: 'Ver valor', icon: 'view', action: () => (viewer.value = { row, col }), shortcut: 'Enter' },
      { label: 'Copiar valor', icon: 'copy', action: () => copy(cellToText(r[col] ?? null), 'Valor'), shortcut: 'Ctrl+C' },
      { label: 'Copiar linha (TSV)', action: () => copy(rowsToTsv(cols, [r], false), 'Linha'), shortcut: 'Ctrl+Shift+C' },
      { label: 'Copiar linha como INSERT', action: () => copy(rowToInsert(props.tableHint || 'tabela', cols, r), 'INSERT') },
      { separator: true, label: '' },
      { label: 'Copiar todas as linhas carregadas (TSV)', action: () => copy(rowsToTsv(cols, props.result.rows, true), 'Resultado') },
      { label: 'Ajustar largura da coluna', action: () => autoFit(col) },
    ],
  }
}

function display(v: Cell | undefined, numeric = false): string {
  if (v === null || v === undefined) return ''
  const s = String(v)
  if (numeric && settings.prefs.gridThousands) return groupDigits(s)
  return s.length > 500 ? s.slice(0, 500) + '…' : s.replace(/\r?\n/g, ' ↵ ')
}
</script>

<template>
  <div
    ref="scrollEl" class="grid" tabindex="0" role="grid" :aria-rowcount="result.rows.length + 1" :aria-colcount="result.columns.length + 1"
    @scroll.passive="onScroll" @keydown="onKey"
  >
    <div class="inner" :style="{ width: totalWidth + 'px', height: totalHeight + ROW_H + 'px' }">
      <div class="header" role="row" aria-rowindex="1" :style="{ gridTemplateColumns: template, width: totalWidth + 'px' }">
        <div class="hcell rownum" role="columnheader">#</div>
        <div
          v-for="(c, i) in result.columns" :key="i" class="hcell" :class="{ num: c.numeric, sorted: sortState?.col === i }" role="columnheader"
          :aria-sort="sortState?.col === i ? (sortState.desc ? 'descending' : 'ascending') : 'none'"
          :title="`${c.name} — ${c.type}${c.nullable === false ? ' NOT NULL' : ''}\nClique para ordenar as linhas carregadas`" @click="toggleSort(i)"
        >
          <span class="ellipsis name">{{ c.name }}</span>
          <span class="type">{{ c.type.toLowerCase() }}</span>
          <span v-if="sortState?.col === i" class="sortmark">{{ sortState.desc ? '▼' : '▲' }}</span>
          <span class="resize" @mousedown="startResize($event, i)" @dblclick.stop="autoFit(i)" />
        </div>
      </div>
      <div
        v-for="it in items" :key="it.index" class="row" :class="{ odd: it.index % 2 === 1 }" role="row" :aria-rowindex="it.index + 2"
        :style="{ transform: `translateY(${it.start + ROW_H}px)`, gridTemplateColumns: template, width: totalWidth + 'px' }"
      >
        <div class="cell rownum" role="rowheader">{{ it.index + 1 }}</div>
        <div
          v-for="(c, ci) in result.columns" :key="ci" class="cell" role="gridcell"
          :class="{ num: c.numeric, bin: c.binary, sel: selected?.row === it.index && selected?.col === ci, isnull: rowAt(it.index)?.[ci] === null }"
          :aria-selected="selected?.row === it.index && selected?.col === ci"
          @mousedown="select(it.index, ci)" @dblclick="viewer = { row: it.index, col: ci }" @contextmenu="openMenu($event, it.index, ci)"
        >
          <template v-if="rowAt(it.index)?.[ci] === null">NULL</template>
          <template v-else>{{ display(rowAt(it.index)?.[ci], c.numeric) }}</template>
          <span v-if="result.truncated.has(`${order[it.index] ?? it.index}:${ci}`)" class="trunc" title="Valor truncado para exibição">…</span>
        </div>
      </div>
    </div>
  </div>
  <CellViewer
    v-if="viewer && result.columns[viewer.col]" :column="result.columns[viewer.col]!" :value="rowAt(viewer.row)?.[viewer.col] ?? null"
    :truncated="result.truncated.has(`${order[viewer.row] ?? viewer.row}:${viewer.col}`)" @close="viewer = null"
  />
  <ContextMenu v-if="menu" :x="menu.x" :y="menu.y" :items="menu.items" @close="menu = null" />
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
  gap: 6px;
  padding: 0 8px;
  line-height: 26px;
  border-right: 1px solid var(--border);
  font-family: var(--font-ui);
  font-weight: 600;
  overflow: hidden;
}
.hcell {
  cursor: pointer;
}
.hcell.sorted {
  color: var(--accent);
}
.sortmark {
  font-size: 9px;
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
.cell {
  padding: 0 8px;
  line-height: 26px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  border-right: 1px solid var(--border);
  border-bottom: 1px solid var(--border);
  user-select: none;
  cursor: default;
}
.cell.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.cell.bin {
  color: var(--text-muted);
}
.cell.isnull {
  color: var(--null);
  font-style: italic;
}
.cell.sel {
  background: var(--selection);
  box-shadow: inset 0 0 0 1px var(--accent);
}
.rownum {
  position: sticky;
  left: 0;
  z-index: 1;
  text-align: right;
  color: var(--text-faint);
  background: var(--bg-elev);
  font-family: var(--font-mono);
  font-weight: 400;
}
.hcell.rownum {
  z-index: 3;
}
.trunc {
  margin-left: 4px;
  color: var(--warning);
  font-weight: 700;
}
</style>

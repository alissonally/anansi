<script setup lang="ts" generic="T">
// Virtualized list table used by the server, database and structure views:
// sortable and resizable columns, optional proportional bars (like
// HeidiSQL's size bars), multi-selection with keyboard and copy as TSV.
import { computed, ref, watch } from 'vue'
import { useVirtualizer } from '@tanstack/vue-virtual'
import { useUiStore } from '../../stores/ui'

export interface Column<R> {
  key: string
  label: string
  width?: number
  align?: 'left' | 'right' | 'center'
  /** Display text. Defaults to String(row[key]). */
  text?: (row: R) => string
  /** Value used to sort; defaults to the raw row[key]. */
  sort?: (row: R) => number | string | null | undefined
  /** 0..1 fraction painted as a bar behind the cell. */
  bar?: (row: R) => number
  /** Extra class per cell. */
  cls?: (row: R) => string | undefined
  title?: (row: R) => string | undefined
  mono?: boolean
}

const props = withDefaults(defineProps<{
  columns: Column<T>[]
  rows: T[]
  rowKey: (row: T) => string
  selected?: Set<string>
  /** Initial sort: column key and direction. */
  sortKey?: string
  sortDesc?: boolean
  emptyText?: string
  loading?: boolean
}>(), { selected: undefined, sortKey: '', sortDesc: false, emptyText: 'Nada para exibir.', loading: false })

const emit = defineEmits<{
  'update:selected': [value: Set<string>]
  activate: [row: T]
  menu: [e: MouseEvent, row: T | null, selected: T[]]
}>()
const ui = useUiStore()

const ROW_H = 24
const scrollEl = ref<HTMLElement | null>(null)
const sort = ref({ key: props.sortKey, desc: props.sortDesc })
const widths = ref<Record<string, number>>({})
const internalSel = ref(new Set<string>())
const anchor = ref<string | null>(null)
const focusKey = ref<string | null>(null)

const selection = computed(() => props.selected ?? internalSel.value)
function setSelection(s: Set<string>) {
  internalSel.value = s
  emit('update:selected', s)
}

function raw(row: T, key: string): unknown {
  return (row as Record<string, unknown>)[key]
}

function cellText(c: Column<T>, row: T): string {
  if (c.text) return c.text(row)
  const v = raw(row, c.key)
  return v === null || v === undefined ? '' : String(v)
}

const sorted = computed(() => {
  const col = props.columns.find((c) => c.key === sort.value.key)
  if (!col) return props.rows
  const val = (r: T) => (col.sort ? col.sort(r) : (raw(r, col.key) as number | string | null | undefined))
  const dir = sort.value.desc ? -1 : 1
  return [...props.rows].sort((a, b) => {
    const x = val(a)
    const y = val(b)
    if (x === y) return 0
    if (x === null || x === undefined || x === '') return 1
    if (y === null || y === undefined || y === '') return -1
    if (typeof x === 'number' && typeof y === 'number') return (x - y) * dir
    return String(x).localeCompare(String(y), 'pt-BR', { numeric: true, sensitivity: 'base' }) * dir
  })
})

function toggleSort(key: string) {
  sort.value = sort.value.key === key ? { key, desc: !sort.value.desc } : { key, desc: false }
}

function widthOf(c: Column<T>) {
  return widths.value[c.key] ?? c.width ?? 140
}
const template = computed(() => props.columns.map((c) => `${widthOf(c)}px`).join(' ') + ' 1fr')
const totalWidth = computed(() => props.columns.reduce((a, c) => a + widthOf(c), 0))

const virtualizer = useVirtualizer(computed(() => ({
  count: sorted.value.length,
  getScrollElement: () => scrollEl.value,
  estimateSize: () => ROW_H,
  overscan: 16,
})))
const items = computed(() => virtualizer.value.getVirtualItems())

watch(() => props.rows, () => {
  // Drop selected keys that disappeared.
  const keys = new Set(props.rows.map(props.rowKey))
  const kept = new Set([...selection.value].filter((k) => keys.has(k)))
  if (kept.size !== selection.value.size) setSelection(kept)
})

function startResize(e: MouseEvent, c: Column<T>) {
  e.preventDefault()
  e.stopPropagation()
  const x0 = e.clientX
  const w0 = widthOf(c)
  const move = (ev: MouseEvent) => (widths.value = { ...widths.value, [c.key]: Math.max(40, w0 + ev.clientX - x0) })
  const up = () => { window.removeEventListener('mousemove', move); window.removeEventListener('mouseup', up) }
  window.addEventListener('mousemove', move)
  window.addEventListener('mouseup', up)
}

function onRowDown(e: MouseEvent, row: T, index: number) {
  const key = props.rowKey(row)
  if (e.button === 2 && selection.value.has(key)) return
  if (e.shiftKey && anchor.value) {
    const keys = sorted.value.map(props.rowKey)
    const a = keys.indexOf(anchor.value)
    const [from, to] = a < index ? [a, index] : [index, a]
    setSelection(new Set(keys.slice(Math.max(0, from), to + 1)))
  } else if (e.ctrlKey || e.metaKey) {
    const s = new Set(selection.value)
    if (s.has(key)) s.delete(key)
    else s.add(key)
    setSelection(s)
    anchor.value = key
  } else {
    setSelection(new Set([key]))
    anchor.value = key
  }
  focusKey.value = key
  scrollEl.value?.focus({ preventScroll: true })
}

function selectedRows(): T[] {
  return sorted.value.filter((r) => selection.value.has(props.rowKey(r)))
}

function onMenu(e: MouseEvent, row: T | null) {
  e.preventDefault()
  emit('menu', e, row, selectedRows())
}

function tsv(rows: T[]): string {
  const esc = (s: string) => (/[\t\n"]/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s)
  return [props.columns.map((c) => esc(c.label)).join('\t'), ...rows.map((r) => props.columns.map((c) => esc(cellText(c, r))).join('\t'))].join('\n')
}

async function copySelection() {
  const rows = selectedRows()
  if (!rows.length) return
  try {
    await navigator.clipboard.writeText(tsv(rows))
    ui.toast('success', `${rows.length} ${rows.length === 1 ? 'linha copiada' : 'linhas copiadas'}.`, 2000)
  } catch {
    ui.toast('error', 'Não foi possível acessar a área de transferência.')
  }
}

function onKey(e: KeyboardEvent) {
  const keys = sorted.value.map(props.rowKey)
  if (!keys.length) return
  const mod = e.ctrlKey || e.metaKey
  if (mod && e.key.toLowerCase() === 'a') {
    e.preventDefault()
    setSelection(new Set(keys))
    return
  }
  if (mod && e.key.toLowerCase() === 'c') {
    e.preventDefault()
    void copySelection()
    return
  }
  if (e.key === 'Enter') {
    const r = sorted.value.find((x) => props.rowKey(x) === focusKey.value)
    if (r) emit('activate', r)
    return
  }
  const cur = focusKey.value ? keys.indexOf(focusKey.value) : -1
  let next: number
  if (e.key === 'ArrowDown') next = Math.min(keys.length - 1, cur + 1)
  else if (e.key === 'ArrowUp') next = Math.max(0, cur - 1)
  else if (e.key === 'PageDown') next = Math.min(keys.length - 1, cur + 20)
  else if (e.key === 'PageUp') next = Math.max(0, cur - 20)
  else if (e.key === 'Home') next = 0
  else if (e.key === 'End') next = keys.length - 1
  else return
  e.preventDefault()
  const key = keys[next]!
  if (e.shiftKey && anchor.value) {
    const a = keys.indexOf(anchor.value)
    const [from, to] = a < next ? [a, next] : [next, a]
    setSelection(new Set(keys.slice(from, to + 1)))
  } else {
    setSelection(new Set([key]))
    anchor.value = key
  }
  focusKey.value = key
  virtualizer.value.scrollToIndex(next, { align: 'auto' })
}

defineExpose({ copySelection, selectedRows, tsv: () => tsv(sorted.value) })
</script>

<template>
  <div ref="scrollEl" class="dt" tabindex="0" role="grid" :aria-rowcount="sorted.length + 1" @keydown="onKey" @contextmenu.self="onMenu($event, null)">
    <div class="inner" :style="{ minWidth: totalWidth + 'px', height: virtualizer.getTotalSize() + ROW_H + 'px' }">
      <div class="head" role="row" :style="{ gridTemplateColumns: template }">
        <div
          v-for="c in columns" :key="c.key" class="hc" role="columnheader" :class="[c.align, { sorted: sort.key === c.key }]"
          :aria-sort="sort.key === c.key ? (sort.desc ? 'descending' : 'ascending') : 'none'" @click="toggleSort(c.key)"
        >
          <span class="ellipsis">{{ c.label }}</span>
          <span v-if="sort.key === c.key" class="arrow">{{ sort.desc ? '▼' : '▲' }}</span>
          <span class="rs" @mousedown="startResize($event, c)" @click.stop />
        </div>
        <div class="hc fill" />
      </div>
      <div
        v-for="it in items" :key="rowKey(sorted[it.index]!)" class="r" role="row"
        :class="{ sel: selection.has(rowKey(sorted[it.index]!)), odd: it.index % 2 === 1, focus: focusKey === rowKey(sorted[it.index]!) }"
        :style="{ transform: `translateY(${it.start + ROW_H}px)`, gridTemplateColumns: template }"
        @mousedown="onRowDown($event, sorted[it.index]!, it.index)" @dblclick="emit('activate', sorted[it.index]!)" @contextmenu="onMenu($event, sorted[it.index]!)"
      >
        <div
          v-for="c in columns" :key="c.key" class="c" role="gridcell" :class="[c.align, c.cls?.(sorted[it.index]!), { mono: c.mono }]"
          :title="c.title?.(sorted[it.index]!) ?? cellText(c, sorted[it.index]!)"
        >
          <span v-if="c.bar && c.bar(sorted[it.index]!) > 0" class="bar" :style="{ width: Math.max(0, Math.min(1, c.bar(sorted[it.index]!))) * 100 + '%' }" />
          <span class="t">{{ cellText(c, sorted[it.index]!) }}</span>
        </div>
        <div class="c fill" />
      </div>
    </div>
    <div v-if="loading && !rows.length" class="empty"><span class="spinner" /> Carregando…</div>
    <div v-else-if="!rows.length" class="empty">{{ emptyText }}</div>
  </div>
</template>

<style scoped>
.dt {
  position: relative;
  height: 100%;
  overflow: auto;
  outline: none;
  font-size: 12px;
  background: var(--bg);
}
.dt:focus-visible {
  box-shadow: inset 0 0 0 1px var(--accent);
}
.inner {
  position: relative;
}
.head {
  position: sticky;
  top: 0;
  z-index: 2;
  display: grid;
  height: 24px;
  background: var(--bg-elev);
  border-bottom: 1px solid var(--border-strong);
}
.hc {
  position: relative;
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 0 8px;
  font-weight: 600;
  border-right: 1px solid var(--border);
  cursor: pointer;
  overflow: hidden;
}
.hc.fill {
  cursor: default;
  border-right: none;
}
.hc.right {
  justify-content: flex-end;
}
.hc.sorted {
  color: var(--accent);
}
.arrow {
  font-size: 8px;
}
.rs {
  position: absolute;
  right: -3px;
  top: 0;
  width: 7px;
  height: 100%;
  cursor: col-resize;
}
.r {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  display: grid;
  height: 24px;
  cursor: default;
}
.r.odd {
  background: var(--grid-stripe);
}
.r:hover {
  background: var(--bg-hover);
}
.r.sel {
  background: var(--selection);
}
.r.focus {
  box-shadow: inset 2px 0 0 var(--accent);
}
.c {
  position: relative;
  padding: 0 8px;
  line-height: 24px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  border-right: 1px solid var(--border);
}
.c.fill {
  border-right: none;
}
.c.right {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.c.center {
  text-align: center;
}
.c.mono {
  font-family: var(--font-mono);
}
.c .t {
  position: relative;
}
.bar {
  position: absolute;
  left: 0;
  top: 4px;
  bottom: 4px;
  background: var(--accent-soft);
  border-right: 1px solid var(--accent);
  pointer-events: none;
}
.empty {
  position: absolute;
  inset: 24px 0 0;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  color: var(--text-muted);
  pointer-events: none;
}
:deep(.muted-cell) {
  color: var(--text-faint);
}
:deep(.warn-cell) {
  color: var(--warning);
}
:deep(.diff-cell) {
  color: var(--accent);
  font-weight: 600;
}
</style>

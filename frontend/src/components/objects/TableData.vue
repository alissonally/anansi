<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import type { Tab } from '../../stores/tabs'
import { useTabsStore } from '../../stores/tabs'
import { useMetadataStore } from '../../stores/metadata'
import { useUiStore } from '../../stores/ui'
import { useSettingsStore } from '../../stores/settings'
import { timestampKey } from '../../utils/display'
import { backend } from '../../api/backend'
import { toAppError } from '../../api/errors'
import type { Cell, CellValue, ColumnInfo, RowEdit, ForeignKeyInfo, IndexInfo, LookupValue, SortSpec, TableKey, TableStatus } from '../../api/types'
import EditableGrid from '../results/EditableGrid.vue'
import CellEditorDialog from '../results/CellEditorDialog.vue'
import ContextMenu, { type MenuItem } from '../common/ContextMenu.vue'
import Icon from '../common/Icon.vue'
import {
  cellToValue, combineFilter, currentValue, editorKindOf, insertableExpressions, isCellValue, makeRow, parseEnumValues,
  quickFilter, rowEdit, rowIsDirty, searchFilter, type GridColumn, type GridRow, type QuickOp,
} from '../../utils/grid'
import { formatDuration } from '../../utils/text'
import { formatCount } from '../../utils/format'
import { withConfirmation } from '../../composables/objectActions'
import { rowToInsert } from '../../utils/cells'
import { forceQuoteIdent } from '../../utils/text'

const props = defineProps<{ tab: Tab; columns: ColumnInfo[]; indexes: IndexInfo[]; foreignKeys: ForeignKeyInfo[] }>()
const tabs = useTabsStore()
const meta = useMetadataStore()
const ui = useUiStore()

const PAGE = 1000
const SHOW_ALL = 100_000
const PARTIAL = 1000

const conn = computed(() => props.tab.connectionId ?? '')
const db = computed(() => props.tab.database)
const table = computed(() => props.tab.objectName ?? '')
const settings = useSettingsStore()
// Integer columns shown as UNIX timestamps (HeidiSQL "Display as UNIX timestamp"), remembered in the preferences.
const timestampCols = computed(() => {
  const prefix = timestampKey(conn.value, db.value, table.value, '')
  return new Set((settings.prefs.timestampColumns ?? []).filter((k) => k.startsWith(prefix)).map((k) => k.slice(prefix.length)))
})
async function toggleTimestamp(col: string) {
  const key = timestampKey(conn.value, db.value, table.value, col)
  const list = settings.prefs.timestampColumns ?? []
  try {
    await settings.save({ ...settings.prefs, timestampColumns: list.includes(key) ? list.filter((k) => k !== key) : [...list, key] })
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}
const readOnly = computed(() => props.tab.kind === 'view')

// ---- per-table settings remembered across sessions (HeidiSQL option) ----
interface Saved { filter: string; sort: SortSpec[]; hidden: string[]; history: string[] }
const storeKey = computed(() => `anansi:grid:${conn.value}:${db.value}.${table.value}`)
function readSaved(): Saved {
  try {
    const s = JSON.parse(localStorage.getItem(storeKey.value) ?? '{}') as Partial<Saved>
    return { filter: s.filter ?? '', sort: s.sort ?? [], hidden: s.hidden ?? [], history: s.history ?? [] }
  } catch {
    return { filter: '', sort: [], hidden: [], history: [] }
  }
}
const saved = readSaved()
const filter = ref(props.tab.dataFilter ?? saved.filter)
const appliedFilter = ref(filter.value)
const sort = ref<SortSpec[]>(saved.sort)
const hidden = ref(new Set(saved.hidden))
const history = ref<string[]>(saved.history)
const search = ref('')
function persist() {
  try {
    localStorage.setItem(storeKey.value, JSON.stringify({ filter: appliedFilter.value, sort: sort.value, hidden: [...hidden.value], history: history.value.slice(0, 20) }))
  } catch {
    // storage unavailable
  }
}

// ---- data ----
const gridCols = ref<GridColumn[]>([])
const rows = ref<GridRow[]>([])
const key = ref<TableKey>({ kind: 'none', columns: [] })
const hasMore = ref(false)
const loading = ref(false)
const loadingMore = ref(false)
const error = ref('')
const lastSql = ref('')
const duration = ref(0)
const exactCount = ref<number | null>(null)
const counting = ref(false)
const lookups = ref<Record<string, LookupValue[]>>({})
const grid = ref<InstanceType<typeof EditableGrid> | null>(null)
const menu = ref<{ x: number; y: number; items: MenuItem[] } | null>(null)
const editor = ref<{ row: number; col: number; value: CellValue | Cell; loading: boolean; error: string; partial: boolean } | null>(null)
const saveError = ref<{ message: string; sql: string } | null>(null)
const saving = ref(false)

const status = computed(() => meta.entry<TableStatus[]>(conn.value, 'status', db.value)?.data.find((t) => t.name === table.value))
const visibleNames = computed(() => props.columns.filter((c) => !hidden.value.has(c.name)).map((c) => c.name))
const visible = computed(() => gridCols.value.map((c, i) => (hidden.value.has(c.name) ? -1 : i)).filter((i) => i >= 0))
const pending = computed(() => rows.value.filter(rowIsDirty).length)
const loadedCount = computed(() => rows.value.filter((r) => r.orig !== null).length)

watch(pending, (n) => {
  const t = tabs.find(props.tab.id)
  if (t) t.objectDirty = n > 0
})

function fkOf(name: string) {
  const f = props.foreignKeys.find((k) => k.columns.length === 1 && k.columns[0] === name)
  return f ? { database: f.refDatabase, table: f.refTable, column: f.refColumns[0]! } : undefined
}

function buildColumns(resultCols: { name: string; type: string; numeric: boolean; binary: boolean; nullable?: boolean }[], partial: boolean[], info: ColumnInfo[]): GridColumn[] {
  return resultCols.map((rc, i) => {
    const ci = info.find((c) => c.name === rc.name)
    const editor = ci ? editorKindOf(ci) : 'text'
    const generated = /VIRTUAL|STORED/i.test(ci?.extra ?? '')
    return {
      ...rc, info: ci, editor, partial: partial[i] ?? false, fk: fkOf(rc.name),
      options: editor === 'enum' || editor === 'set' ? parseEnumValues(ci!.columnType) : undefined,
      editable: !readOnly.value && !!ci && !generated,
    }
  })
}

async function confirmDiscard(what: string): Promise<boolean> {
  if (!pending.value) return true
  return ui.confirm({
    title: 'Alterações não salvas',
    message: `Há ${pending.value} ${pending.value === 1 ? 'linha alterada' : 'linhas alteradas'} ainda não salvas. ${what} descarta essas alterações.`,
    confirmLabel: 'Descartar e continuar',
    danger: true,
  })
}

async function load(opts: { keepCount?: boolean; confirm?: boolean } = {}) {
  if (opts.confirm !== false && !(await confirmDiscard('Recarregar'))) return
  loading.value = true
  error.value = ''
  saveError.value = null
  const limit = opts.keepCount ? Math.max(PAGE, loadedCount.value) : PAGE
  try {
    const d = await backend.browseTable(props.tab.id, conn.value, {
      database: db.value, table: table.value, columns: hidden.value.size ? visibleNames.value : [], filter: appliedFilter.value,
      sort: sort.value, offset: 0, limit, partialLength: PARTIAL,
    })
    key.value = d.key
    gridCols.value = buildColumns(d.columns, d.partial, d.info.length ? d.info : props.columns)
    const trunc = new Map<number, number[]>()
    for (const [r, c] of d.truncatedCells ?? []) trunc.set(r, [...(trunc.get(r) ?? []), c])
    rows.value = (d.rows ?? []).map((r, i) => makeRow(r, trunc.get(i)))
    hasMore.value = d.hasMore
    lastSql.value = d.sql
    duration.value = d.durationMs
  } catch (e) {
    const err = toAppError(e)
    error.value = err.message
    if (err.code === 'cancelled') error.value = 'Carregamento cancelado.'
  } finally {
    loading.value = false
  }
}

async function loadMore(all = false) {
  if (!hasMore.value || loadingMore.value || loading.value) return
  loadingMore.value = true
  try {
    const d = await backend.browseTable(props.tab.id, conn.value, {
      database: db.value, table: table.value, columns: hidden.value.size ? visibleNames.value : [], filter: appliedFilter.value,
      sort: sort.value, offset: loadedCount.value, limit: all ? SHOW_ALL - loadedCount.value : PAGE, partialLength: PARTIAL,
    })
    const trunc = new Map<number, number[]>()
    for (const [r, c] of d.truncatedCells ?? []) trunc.set(r, [...(trunc.get(r) ?? []), c])
    rows.value.push(...(d.rows ?? []).map((r, i) => makeRow(r, trunc.get(i))))
    hasMore.value = d.hasMore && loadedCount.value < SHOW_ALL
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  } finally {
    loadingMore.value = false
  }
}

function cancelLoad() {
  void backend.cancelBrowse(props.tab.id)
}

// Reload when the tab is (re)targeted or a "follow foreign key" filter arrives.
watch(() => [conn.value, db.value, table.value], () => void load({ confirm: false }), { immediate: true })
watch(() => props.tab.dataFilter, async (f) => {
  if (f === undefined) return
  const t = tabs.find(props.tab.id)
  if (t) t.dataFilter = undefined
  if (!(await confirmDiscard('Aplicar o filtro'))) return
  filter.value = f
  applyFilter(false)
})
onBeforeUnmount(cancelLoad)

// ---- filter, search, sort, columns ----
function applyFilter(confirm = true) {
  appliedFilter.value = filter.value.trim()
  exactCount.value = null
  if (appliedFilter.value) history.value = [appliedFilter.value, ...history.value.filter((h) => h !== appliedFilter.value)].slice(0, 20)
  persist()
  void load({ confirm })
}

function clearFilter() {
  filter.value = ''
  search.value = ''
  applyFilter()
}

function applySearch() {
  const f = searchFilter(search.value, gridCols.value.length ? gridCols.value : props.columns.map((c) => ({ name: c.name, type: c.dataType, numeric: false, binary: false, editor: editorKindOf(c), editable: false })))
  filter.value = f
  applyFilter()
}

function filterKey(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    applyFilter()
  }
}

async function onSort(column: string, additive: boolean) {
  if (!(await confirmDiscard('Ordenar'))) return
  const cur = sort.value.find((s) => s.column === column)
  let next: SortSpec[]
  if (additive) {
    next = cur ? (cur.desc ? sort.value.filter((s) => s.column !== column) : sort.value.map((s) => (s.column === column ? { ...s, desc: true } : s))) : [...sort.value, { column, desc: false }]
  } else {
    next = !cur || sort.value.length > 1 ? [{ column, desc: false }] : cur.desc ? [] : [{ column, desc: true }]
  }
  sort.value = next
  persist()
  void load({ confirm: false })
}

async function sortBy(column: string, desc: boolean) {
  if (!(await confirmDiscard('Ordenar'))) return
  sort.value = [{ column, desc }]
  persist()
  void load({ confirm: false })
}

async function resetSort() {
  if (!(await confirmDiscard('Remover a ordenação'))) return
  sort.value = []
  persist()
  void load({ confirm: false })
}

const columnsMenu = ref<{ x: number; y: number } | null>(null)
const columnFilter = ref('')
function toggleColumn(name: string) {
  const h = new Set(hidden.value)
  if (h.has(name)) h.delete(name)
  else if (props.columns.length - h.size > 1) h.add(name)
  hidden.value = h
}
async function applyColumns() {
  columnsMenu.value = null
  if (!(await confirmDiscard('Mudar as colunas'))) return
  persist()
  void load({ confirm: false })
}

async function countExact() {
  counting.value = true
  try {
    exactCount.value = await backend.countTableRows(props.tab.id, conn.value, db.value, table.value, appliedFilter.value)
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  } finally {
    counting.value = false
  }
}

// ---- editing ----
function sameAsOriginal(row: GridRow, col: number, v: CellValue): boolean {
  if (!row.orig || row.truncated.has(col)) return false
  const o = row.orig[col] ?? null
  if (v.kind === 'null') return o === null
  if (o === null) return false
  const c = gridCols.value[col]!
  if (v.kind === 'hex') return c.binary && String(o).toUpperCase() === '0X' + v.text.toUpperCase()
  return v.kind === 'value' && String(o) === v.text
}

function setCell(row: number, col: number, v: CellValue) {
  const r = rows.value[row]
  if (!r || r.deleted) return
  const edits = new Map(r.edits)
  if (sameAsOriginal(r, col, v)) edits.delete(col)
  else edits.set(col, v)
  r.edits = edits
  r.error = undefined
}

function onCommit(row: number, col: number, v: CellValue) {
  const c = gridCols.value[col]
  if (c?.binary && v.kind === 'value') {
    // Text typed into a binary cell: accept 0x… hex.
    const t = v.text.trim()
    if (/^0x[0-9a-f]*$/i.test(t)) v = { kind: 'hex', text: t.slice(2) }
  }
  setCell(row, col, v)
}

function keyWhere(r: GridRow) {
  return rowEdit({ ...r, edits: new Map(), deleted: true }, gridCols.value, key.value)?.where ?? []
}

async function openEditor(row: number, col: number) {
  const r = rows.value[row]
  if (!r) return
  const needsFull = r.truncated.has(col) && !r.edits.has(col) && r.orig !== null
  editor.value = { row, col, value: currentValue(r, col), loading: needsFull, error: '', partial: needsFull }
  if (!needsFull) return
  try {
    const vals = await backend.fetchRowValues(conn.value, db.value, table.value, keyWhere(r), [gridCols.value[col]!.name])
    if (editor.value?.row === row && editor.value.col === col) {
      editor.value = { ...editor.value, value: vals[0] ?? null, loading: false, partial: false }
      // Remember the full value so saving an unchanged value is a no-op.
      const orig = [...r.orig!]
      orig[col] = vals[0] ?? null
      r.orig = orig
      r.truncated = new Set([...r.truncated].filter((x) => x !== col))
    }
  } catch (e) {
    if (editor.value) editor.value = { ...editor.value, loading: false, error: toAppError(e).message }
  }
}

function insertRow(copyFrom?: GridRow) {
  if (readOnly.value) return
  const cur = grid.value?.cursor()
  const at = cur ? cur.row + 1 : rows.value.length
  const r = makeRow(null)
  if (copyFrom) {
    gridCols.value.forEach((c, i) => {
      const auto = /auto_increment/i.test(c.info?.extra ?? '')
      if (!c.editable || auto || copyFrom.truncated.has(i)) return
      const v = currentValue(copyFrom, i)
      if (isCellValue(v)) {
        if (v.kind !== 'default') r.edits.set(i, v)
      } else r.edits.set(i, cellToValue(v, c))
    })
    if (copyFrom.truncated.size) ui.toast('warning', 'Valores parciais (textos/binários longos) não foram copiados.')
  }
  rows.value.splice(at, 0, r)
  const first = visible.value.find((i) => gridCols.value[i]!.editable && !/auto_increment/i.test(gridCols.value[i]!.info?.extra ?? '')) ?? visible.value[0]
  void nextTick(() => grid.value?.setCursor(at, first ?? 0))
}

function selectedRows(): number[] {
  const sel = grid.value?.selectedRows() ?? []
  if (sel.length) return sel
  const c = grid.value?.cursor()
  return c ? [c.row] : []
}

function deleteRows() {
  if (readOnly.value) return
  const sel = selectedRows()
  const remove = new Set<number>()
  for (const i of sel) {
    const r = rows.value[i]
    if (!r) continue
    if (r.orig === null) remove.add(i)
    else r.deleted = !r.deleted
  }
  if (remove.size) rows.value = rows.value.filter((_, i) => !remove.has(i))
}

function undoRows(all = false) {
  const sel = new Set(all ? rows.value.map((_, i) => i) : selectedRows())
  rows.value = rows.value.filter((r, i) => !(sel.has(i) && r.orig === null))
  rows.value.forEach((r, i) => {
    if (all || sel.has(i)) {
      r.edits = new Map()
      r.deleted = false
      r.error = undefined
    }
  })
}

function setNull() {
  const c = grid.value?.cursor()
  if (!c) return
  const col = gridCols.value[c.col]
  if (!col?.editable || col.info?.nullable === false) {
    ui.toast('warning', 'Esta coluna não aceita NULL.')
    return
  }
  setCell(c.row, c.col, { kind: 'null', text: '' })
}

async function save() {
  if (!pending.value || saving.value) return
  const order: number[] = []
  const edits: RowEdit[] = []
  for (const [i, r] of rows.value.entries()) {
    const e = rowEdit(r, gridCols.value, key.value)
    if (e) {
      edits.push(e)
      order.push(i)
    }
  }
  if (!edits.length) {
    undoRows(true)
    return
  }
  if (key.value.kind === 'none' && edits.some((e) => e.op !== 'insert')) {
    const ok = await ui.confirm({
      title: 'Tabela sem chave',
      message: 'A tabela não tem chave primária nem índice único de colunas NOT NULL. As linhas serão identificadas por todos os valores, com LIMIT 1: se existirem linhas idênticas, qualquer uma delas pode ser alterada.',
      confirmLabel: 'Salvar mesmo assim',
    })
    if (!ok) return
  }
  saving.value = true
  saveError.value = null
  try {
    const res = await withConfirmation((c) => backend.applyRowEdits(conn.value, db.value, table.value, edits, c))
    if (!res) return
    if (res.applied) {
      ui.toast('success', `${edits.length} ${edits.length === 1 ? 'alteração salva' : 'alterações salvas'}.`, 2500)
      const warn = (res.results ?? []).filter((r) => r.rowsAffected === 0)
      if (warn.length) ui.toast('warning', `${warn.length} ${warn.length === 1 ? 'instrução não alterou' : 'instruções não alteraram'} nenhuma linha (valor igual, ou linha alterada/removida por outra sessão).`, 8000)
      await load({ keepCount: true, confirm: false })
      return
    }
    const results = res.results ?? []
    const failedAt = results.findIndex((r) => r.error)
    const f = results[failedAt]
    const row = rows.value[order[failedAt] ?? -1]
    if (row && f?.error) row.error = f.error.message
    saveError.value = { message: f?.error?.message ?? 'falha ao salvar', sql: f?.sql ?? '' }
    if (row) grid.value?.setCursor(order[failedAt]!, visible.value[0] ?? 0)
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  } finally {
    saving.value = false
  }
}

async function showSql() {
  const edits = rows.value.map((r) => rowEdit(r, gridCols.value, key.value)).filter((e) => e !== null)
  if (!edits.length) {
    ui.toast('info', 'Nenhuma alteração pendente.')
    return
  }
  try {
    const stmts = await backend.previewRowEdits(conn.value, db.value, table.value, edits)
    const ok = await ui.confirm({
      title: 'SQL das alterações pendentes',
      message: 'Estas instruções serão executadas numa única transação (os valores são enviados como parâmetros).',
      items: stmts.map((s) => ({ text: s + ';' })),
      confirmLabel: 'Abrir em nova aba',
      cancelLabel: 'Fechar',
    })
    if (ok) tabs.open({ title: `Alterações ${table.value}`, connectionId: conn.value, database: db.value, sql: `START TRANSACTION;\n${stmts.map((s) => s + ';').join('\n')}\nCOMMIT;\n` })
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}

function onAction(name: 'insert' | 'duplicate' | 'delete' | 'null' | 'save' | 'refresh' | 'undo') {
  switch (name) {
    case 'insert': insertRow(); break
    case 'duplicate': {
      const c = grid.value?.cursor()
      const r = c ? rows.value[c.row] : undefined
      if (r) insertRow(r)
      break
    }
    case 'delete': deleteRows(); break
    case 'null': setNull(); break
    case 'save': void save(); break
    case 'refresh': void load({ keepCount: true }); break
    case 'undo': undoRows(); break
  }
}

async function needLookup(col: number) {
  const c = gridCols.value[col]
  if (!c?.fk || lookups.value[c.name]) return
  lookups.value = { ...lookups.value, [c.name]: [] }
  try {
    const vals = await backend.lookupValues(conn.value, c.fk.database, c.fk.table, c.fk.column, '')
    lookups.value = { ...lookups.value, [c.name]: vals }
  } catch {
    // dropdown stays empty
  }
}
// Labels for FK columns are useful even before editing.
watch(gridCols, (cols) => cols.forEach((c, i) => c.fk && void needLookup(i)))

// ---- context menu ----
function quick(op: QuickOp, col: number, rowsIdx: number[], and: boolean) {
  const c = gridCols.value[col]!
  const vals = rowsIdx.map((i) => {
    const v = currentValue(rows.value[i]!, col)
    return isCellValue(v) ? (v.kind === 'null' ? null : v.text) : v
  })
  filter.value = combineFilter(appliedFilter.value, quickFilter(op, c, vals), and)
  applyFilter()
}

async function moreValues(col: number, and: boolean) {
  const c = gridCols.value[col]!
  try {
    const vals = await backend.distinctValues(conn.value, db.value, table.value, c.name, appliedFilter.value)
    if (!vals.length) {
      ui.toast('info', 'Nenhum valor encontrado.')
      return
    }
    const r = await ui.prompt({
      title: `Valores mais frequentes de ${c.name}`, label: 'Filtrar por', confirmLabel: 'Filtrar',
      choices: vals.map((v, i) => ({ value: String(i), label: `${v.value === null ? 'NULL' : String(v.value).slice(0, 80)}  (${formatCount(v.count)})` })),
    })
    if (!r) return
    const v = vals[Number(r.value)]
    if (!v) return
    filter.value = combineFilter(appliedFilter.value, quickFilter('=', c, [v.value]), and)
    applyFilter()
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}

async function promptFilter(op: QuickOp, col: number, and: boolean) {
  const c = gridCols.value[col]!
  const r = await ui.prompt({ title: `Filtrar ${c.name} ${op === 'like' ? 'contendo' : op}`, label: 'Valor', confirmLabel: 'Filtrar' })
  if (!r) return
  filter.value = combineFilter(appliedFilter.value, quickFilter(op, c, [r.value]), and)
  applyFilter()
}

function followFk(row: number, col: number) {
  const c = gridCols.value[col]!
  const v = currentValue(rows.value[row]!, col)
  if (!c.fk || v === null || isCellValue(v)) return
  const f = quickFilter('=', { ...c, name: c.fk.column }, [v])
  tabs.openObject({ kind: 'table', connectionId: conn.value, database: c.fk.database, objectName: c.fk.table, view: 'data', dataFilter: f })
}

async function copyAs(kind: 'insert' | 'tsv' | 'where', idx: number[]) {
  const cols = gridCols.value
  const vis = visible.value
  const tbl = `${forceQuoteIdent(db.value)}.${forceQuoteIdent(table.value)}`
  let text: string
  if (kind === 'insert') {
    text = idx.map((i) => rowToInsert(tbl, vis.map((ci) => cols[ci]!), vis.map((ci) => {
      const v = currentValue(rows.value[i]!, ci)
      return isCellValue(v) ? (v.kind === 'null' ? null : v.kind === 'hex' ? '0x' + v.text : v.text) : v
    }))).join('\n')
  } else if (kind === 'where') {
    text = idx.map((i) => keyWhere(rows.value[i]!).map((w) => `${forceQuoteIdent(w.column)} ${w.value.kind === 'null' ? 'IS NULL' : `= ${w.value.kind === 'hex' ? '0x' + w.value.text : `'${w.value.text.replace(/'/g, "''")}'`}`}`).join(' AND ')).join('\nOR ')
  } else {
    const esc = (s: string) => (/[\t\n"]/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s)
    text = [vis.map((ci) => esc(cols[ci]!.name)).join('\t'), ...idx.map((i) => vis.map((ci) => {
      const v = currentValue(rows.value[i]!, ci)
      return esc(isCellValue(v) ? v.text : v === null ? '\\N' : String(v))
    }).join('\t'))].join('\n')
  }
  await navigator.clipboard.writeText(text).then(() => ui.toast('success', 'Copiado.', 1500)).catch(() => ui.toast('error', 'Não foi possível copiar.'))
}

function onMenu(e: MouseEvent, row: number, col: number) {
  const c = gridCols.value[col]!
  const r = rows.value[row]!
  const sel = selectedRows()
  const v = currentValue(r, col)
  const shown = isCellValue(v) ? (v.kind === 'null' ? 'NULL' : v.text) : v === null ? 'NULL' : String(v)
  const short = shown.length > 30 ? shown.slice(0, 30) + '…' : shown
  const ro = readOnly.value || !c.editable || r.deleted
  const filterItems = (and: boolean): MenuItem[] => [
    { label: `= ${short}`, action: () => quick('=', col, sel, and) },
    { label: `≠ ${short}`, action: () => quick('!=', col, sel, and) },
    { label: `> ${short}`, action: () => quick('>', col, sel, and), disabled: v === null },
    { label: `< ${short}`, action: () => quick('<', col, sel, and), disabled: v === null },
    { label: `contém “${short}”`, action: () => quick('like', col, sel, and), disabled: v === null },
    { label: `começa com “${short}”`, action: () => quick('starts', col, sel, and), disabled: v === null },
    { label: `termina com “${short}”`, action: () => quick('ends', col, sel, and), disabled: v === null },
    ...(sel.length > 1 ? [
      { label: `IN (${sel.length} valores selecionados)`, action: () => quick('in', col, sel, and) },
      { label: `NOT IN (${sel.length} valores selecionados)`, action: () => quick('notin', col, sel, and) },
    ] : []),
    { separator: true, label: '' },
    { label: 'IS NULL', action: () => quick('null', col, sel, and) },
    { label: 'IS NOT NULL', action: () => quick('notnull', col, sel, and) },
    { separator: true, label: '' },
    { label: 'Igual a…', action: () => promptFilter('=', col, and) },
    { label: 'Contendo…', action: () => promptFilter('like', col, and) },
    { label: 'Valores mais frequentes…', icon: 'filter', action: () => moreValues(col, and) },
  ]
  const exprs = insertableExpressions.filter((x) => x.kinds.includes(c.editor))
  const items: MenuItem[] = [
    { label: ro ? 'Ver valor' : 'Editar valor', icon: 'edit', shortcut: 'Enter', action: () => (ro || ['longtext', 'json', 'binary', 'set', 'spatial'].includes(c.editor) || r.truncated.has(col) ? openEditor(row, col) : grid.value?.startEdit(row, col)) },
    { label: 'Editar em janela…', action: () => openEditor(row, col) },
    { label: 'Definir NULL', shortcut: 'Ctrl+Shift+N', disabled: ro || c.info?.nullable === false, action: () => setCell(row, col, { kind: 'null', text: '' }) },
    { label: 'Definir DEFAULT', disabled: ro, action: () => setCell(row, col, { kind: 'default', text: '' }) },
    { label: 'Inserir valor', disabled: ro || !exprs.length, children: exprs.map((x) => ({ label: x.label, hint: x.expr, action: () => setCell(row, col, { kind: 'expression', text: x.expr }) })) },
    { separator: true, label: '' },
    { label: 'Filtrar por este valor', icon: 'filter', children: filterItems(false) },
    { label: 'Adicionar ao filtro (AND)', children: filterItems(true), disabled: !appliedFilter.value },
    { label: 'Remover filtro', disabled: !appliedFilter.value, action: clearFilter },
    ...(c.fk ? [{ label: `Ir para ${c.fk.table}.${c.fk.column} = ${short}`, icon: 'link' as const, disabled: v === null, action: () => followFk(row, col) }] : []),
    { separator: true, label: '' },
    { label: 'Copiar valor', icon: 'copy', shortcut: 'Ctrl+C', action: () => navigator.clipboard.writeText(shown) },
    { label: 'Copiar', children: [
      { label: sel.length > 1 ? `${sel.length} linhas (TSV com cabeçalho)` : 'Linha (TSV com cabeçalho)', action: () => copyAs('tsv', sel) },
      { label: sel.length > 1 ? `${sel.length} linhas como INSERT` : 'Linha como INSERT', action: () => copyAs('insert', sel) },
      { label: 'Condição WHERE da(s) linha(s)', action: () => copyAs('where', sel) },
      { label: 'Nome da coluna', action: () => navigator.clipboard.writeText(c.name) },
      { label: 'Em outro formato…', icon: 'download', action: exportRows },
    ] },
    { separator: true, label: '' },
    { label: 'Ordenar crescente por esta coluna', icon: 'arrowUp', action: () => sortBy(c.name, false) },
    { label: 'Ordenar decrescente por esta coluna', icon: 'arrowDown', action: () => sortBy(c.name, true) },
    { label: 'Ocultar coluna', disabled: visible.value.length < 2, action: () => { toggleColumn(c.name); void applyColumns() } },
    ...(c.numeric && /int/i.test(c.info?.dataType ?? c.type) ? [{ label: timestampCols.value.has(c.name) ? 'Exibir como número' : 'Exibir como data (timestamp UNIX)', icon: 'clock' as const, action: () => toggleTimestamp(c.name) }] : []),
    { separator: true, label: '' },
    { label: 'Inserir linha', icon: 'plus', shortcut: 'Insert', disabled: readOnly.value, action: () => insertRow() },
    { label: 'Duplicar linha', icon: 'copy', shortcut: 'Ctrl+D', disabled: readOnly.value, action: () => insertRow(r) },
    { label: sel.length > 1 ? `Excluir ${sel.length} linhas` : 'Excluir linha', icon: 'trash', shortcut: 'Ctrl+Del', danger: true, disabled: readOnly.value, action: deleteRows },
    { label: 'Desfazer alterações da(s) linha(s)', icon: 'undo', shortcut: 'Ctrl+Z', disabled: !sel.some((i) => rows.value[i] && rowIsDirty(rows.value[i]!)), action: () => undoRows() },
  ]
  menu.value = { x: e.clientX, y: e.clientY, items }
}

const summary = computed(() => {
  const est = status.value?.rows
  const parts: string[] = []
  if (exactCount.value !== null) parts.push(`${formatCount(exactCount.value)} ${appliedFilter.value ? 'linhas no filtro' : 'linhas'}`)
  else if (est !== undefined && !appliedFilter.value) parts.push(`≈${formatCount(est)} linhas`)
  parts.push(`${formatCount(loadedCount.value)} carregadas${hasMore.value ? ' (há mais)' : ''}`)
  if (duration.value) parts.push(formatDuration(duration.value))
  return parts.join(' · ')
})

const historyMenu = ref<{ x: number; y: number } | null>(null)
function showHistory(e: MouseEvent) {
  const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
  historyMenu.value = { x: r.left, y: r.bottom + 2 }
}
const historyItems = computed<MenuItem[]>(() => history.value.length
  ? [...history.value.map((h) => ({ label: h.length > 90 ? h.slice(0, 90) + '…' : h, action: () => { filter.value = h; applyFilter() } })),
    { separator: true, label: '' }, { label: 'Limpar histórico', icon: 'trash' as const, action: () => { history.value = []; persist() } }]
  : [{ label: 'Nenhum filtro recente', disabled: true }])

function showColumns(e: MouseEvent) {
  const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
  columnsMenu.value = { x: Math.min(r.left, window.innerWidth - 300), y: r.bottom + 2 }
  columnFilter.value = ''
}

function exportRows() {
  const vis = visible.value
  const cols = gridCols.value
  const keys = new Set((key.value.columns ?? []).map((k) => k.toLowerCase()))
  const rowValues = (r: GridRow) => vis.map((ci) => {
    const v = currentValue(r, ci)
    return isCellValue(v) ? (v.kind === 'null' ? null : v.kind === 'hex' ? '0x' + v.text : v.text) : v
  })
  ui.exportDialog = {
    title: table.value,
    columns: vis.map((ci) => {
      const c = cols[ci]!
      return { name: c.name, type: c.type, numeric: c.numeric, binary: c.binary, key: keys.has(c.name.toLowerCase()), autoIncrement: /auto_increment/i.test(c.info?.extra ?? '') }
    }),
    rows: () => rows.value.filter((r) => !r.deleted).map(rowValues),
    selected: () => selectedRows().map((i) => rows.value[i]).filter((r): r is GridRow => !!r).map(rowValues),
    table: `${db.value}.${table.value}`,
    full: { kind: 'table', connectionId: conn.value, spec: { database: db.value, table: table.value, columns: hidden.value.size ? visibleNames.value : [], filter: appliedFilter.value, sort: sort.value, offset: 0, limit: 0, partialLength: 0, noLimit: true } },
  }
}

function editorSave(v: CellValue) {
  const ed = editor.value
  if (!ed) return
  setCell(ed.row, ed.col, v)
  editor.value = null
  grid.value?.focus()
}
</script>

<template>
  <div class="td">
    <div class="filterbar">
      <div class="where">
        <span class="kw mono">WHERE</span>
        <textarea
          v-model="filter" class="input mono flt" rows="1" spellcheck="false" placeholder="filtro SQL, ex.: status = 'pago' AND total > 100  (Enter aplica · Shift+Enter quebra linha)"
          aria-label="Filtro WHERE" @keydown="filterKey"
        />
        <button class="icon-btn" title="Filtros recentes" aria-label="Filtros recentes" @click="showHistory"><Icon name="history" :size="14" /></button>
        <button class="btn small primary" :disabled="loading" @click="applyFilter()">Aplicar</button>
        <button class="btn small" :disabled="!appliedFilter && !filter" @click="clearFilter">Limpar</button>
      </div>
      <div class="search">
        <Icon name="search" :size="12" />
        <input v-model="search" placeholder="Buscar texto em todas as colunas" aria-label="Buscar texto" @keydown.enter="applySearch" />
      </div>
    </div>
    <div class="toolbar">
      <template v-if="!readOnly">
        <button class="btn small" title="Inserir linha (Insert)" @click="insertRow()"><Icon name="plus" :size="12" /> Inserir</button>
        <button class="btn small" title="Duplicar linha (Ctrl+D)" @click="onAction('duplicate')"><Icon name="copy" :size="12" /> Duplicar</button>
        <button class="btn small" title="Excluir linhas selecionadas (Ctrl+Delete)" @click="deleteRows"><Icon name="trash" :size="12" /> Excluir</button>
        <button class="btn small" title="Definir NULL (Ctrl+Shift+N)" @click="setNull">NULL</button>
        <span class="sep" />
        <button class="btn small" :class="{ primary: pending > 0 }" :disabled="!pending || saving" title="Salvar alterações numa transação (Ctrl+S)" @click="save">
          <span v-if="saving" class="spinner" /><Icon v-else name="save" :size="12" /> Salvar<template v-if="pending"> ({{ pending }})</template>
        </button>
        <button class="btn small" :disabled="!pending" title="Descartar todas as alterações" @click="undoRows(true)"><Icon name="undo" :size="12" /> Descartar</button>
        <button class="btn small" :disabled="!pending" title="Ver o SQL das alterações" @click="showSql">SQL</button>
        <span class="sep" />
      </template>
      <span v-else class="badge ro"><Icon name="lock" :size="10" /> somente leitura</span>
      <button class="btn small" title="Exportar ou copiar como CSV, JSON, SQL, Markdown… (inclusive a tabela inteira)" @click="exportRows"><Icon name="download" :size="12" /> Exportar</button>
      <button class="btn small" title="Escolher colunas" @click="showColumns"><Icon name="columns" :size="12" /> Colunas<template v-if="hidden.size"> ({{ columns.length - hidden.size }}/{{ columns.length }})</template></button>
      <button v-if="sort.length" class="btn small" title="Remover ordenação" @click="resetSort"><Icon name="x" :size="11" /> Ordenação</button>
      <span class="spacer" />
      <span class="muted summary">{{ summary }}</span>
      <button class="btn small" :disabled="counting" title="SELECT COUNT(*) com o filtro" @click="countExact"><span v-if="counting" class="spinner" /> Contar</button>
      <button v-if="hasMore" class="btn small" :disabled="loadingMore" @click="loadMore()">+{{ formatCount(PAGE) }}</button>
      <button v-if="hasMore" class="btn small" :disabled="loadingMore" :title="`Carregar até ${formatCount(SHOW_ALL)} linhas`" @click="loadMore(true)">Tudo</button>
      <button v-if="loading" class="btn small" @click="cancelLoad"><Icon name="stop" :size="10" /> Cancelar</button>
      <button class="icon-btn" title="Recarregar (F5)" aria-label="Recarregar" @click="load({ keepCount: true })"><span v-if="loading" class="spinner" /><Icon v-else name="refresh" :size="14" /></button>
    </div>
    <div v-if="key.kind === 'none' && !readOnly && rows.length" class="banner warn">
      <Icon name="alert" :size="13" /> Sem chave primária ou índice único NOT NULL: as edições identificam a linha por todos os valores e usam LIMIT 1.
    </div>
    <div v-if="saveError" class="banner err">
      <Icon name="alert" :size="13" />
      <div class="selectable"><strong>Nada foi salvo (transação revertida):</strong> {{ saveError.message }}<code class="mono">{{ saveError.sql }}</code></div>
      <button class="icon-btn" aria-label="Fechar" @click="saveError = null"><Icon name="x" :size="12" /></button>
    </div>
    <div class="body">
      <div v-if="error" class="center">
        <div class="error-box selectable" role="alert">
          <Icon name="alert" :size="16" />
          <div>
            <div>{{ error }}</div>
            <code class="mono faint">{{ lastSql }}</code>
          </div>
        </div>
      </div>
      <div v-else-if="loading && !rows.length" class="center"><span class="spinner" /> Carregando…</div>
      <EditableGrid
        v-else ref="grid" :columns="gridCols" :visible="visible" :rows="rows" :sort="sort" :read-only="readOnly" :has-more="hasMore" :loading-more="loadingMore"
        :lookups="lookups" :timestamp-cols="timestampCols" @sort="onSort" @load-more="loadMore()" @commit="onCommit" @open-editor="openEditor" @need-lookup="needLookup" @menu="onMenu" @action="onAction"
      />
      <div v-if="!error && !loading && !rows.length" class="empty faint">{{ appliedFilter ? 'Nenhuma linha atende ao filtro.' : 'Tabela vazia.' }}<template v-if="!readOnly"> Pressione Insert para adicionar uma linha.</template></div>
    </div>
    <div class="sqlline mono faint ellipsis" :title="lastSql">{{ lastSql }}</div>

    <ContextMenu v-if="menu" :x="menu.x" :y="menu.y" :items="menu.items" @close="menu = null" />
    <ContextMenu v-if="historyMenu" :x="historyMenu.x" :y="historyMenu.y" :items="historyItems" @close="historyMenu = null" />
    <Teleport to="body">
      <div v-if="columnsMenu" class="colpicker-backdrop" @mousedown.self="applyColumns">
        <div class="colpicker" :style="{ left: columnsMenu.x + 'px', top: columnsMenu.y + 'px' }" role="dialog" aria-label="Colunas visíveis" @keydown.esc="applyColumns">
          <input v-model="columnFilter" class="input" placeholder="Filtrar colunas" autofocus />
          <div class="cp-actions">
            <button class="btn small" @click="hidden = new Set()">Todas</button>
            <button class="btn small" @click="hidden = new Set(columns.slice(1).map((c) => c.name))">Nenhuma</button>
          </div>
          <div class="cp-list">
            <label v-for="c in columns.filter((x) => !columnFilter || x.name.toLowerCase().includes(columnFilter.toLowerCase()))" :key="c.name" class="check">
              <input type="checkbox" :checked="!hidden.has(c.name)" @change="toggleColumn(c.name)" /> {{ c.name }} <span class="faint">{{ c.columnType }}</span>
            </label>
          </div>
          <button class="btn small primary" @click="applyColumns">Aplicar</button>
        </div>
      </div>
    </Teleport>
    <CellEditorDialog
      v-if="editor && gridCols[editor.col]" :column="gridCols[editor.col]!" :value="editor.value" :loading="editor.loading" :error="editor.error" :partial="editor.partial"
      :read-only="readOnly || !gridCols[editor.col]!.editable || rows[editor.row]?.deleted" @save="editorSave" @close="editor = null; grid?.focus()"
    />
  </div>
</template>

<style scoped>
.td {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}
.filterbar {
  display: flex;
  gap: 10px;
  padding: 6px 8px;
  border-bottom: 1px solid var(--border);
  align-items: flex-start;
  flex-wrap: wrap;
}
.where {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  flex: 1;
  min-width: 360px;
}
.kw {
  font-size: 11px;
  font-weight: 700;
  color: var(--accent);
  line-height: 28px;
}
.flt {
  flex: 1;
  min-height: 28px;
  height: 28px;
  max-height: 120px;
  padding: 5px 8px;
  resize: vertical;
  font-size: 12px;
}
.search {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 8px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius);
  background: var(--bg-input);
  color: var(--text-faint);
  width: 240px;
}
.search input {
  flex: 1;
  min-width: 0;
  border: none;
  outline: none;
  background: transparent;
  color: var(--text);
  user-select: text;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 8px;
  border-bottom: 1px solid var(--border);
  flex-wrap: wrap;
}
.sep {
  width: 1px;
  height: 18px;
  background: var(--border);
  margin: 0 2px;
}
.spacer {
  flex: 1;
}
.summary {
  font-size: 12px;
}
.ro {
  background: var(--warning-soft);
  color: var(--warning);
}
.banner {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 6px 10px;
  font-size: 12px;
  border-bottom: 1px solid var(--border);
}
.banner.warn {
  background: var(--warning-soft);
}
.banner.err {
  background: var(--danger-soft);
  color: var(--text);
}
.banner.err > .icon {
  color: var(--danger);
}
.banner code {
  display: block;
  margin-top: 2px;
  white-space: pre-wrap;
  font-size: 11px;
}
.banner > div {
  flex: 1;
}
.body {
  position: relative;
  flex: 1;
  min-height: 0;
}
.center {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  height: 100%;
  padding: 16px;
}
.error-box {
  display: flex;
  gap: 10px;
  max-width: 760px;
  padding: 12px 14px;
  border-radius: var(--radius);
  background: var(--danger-soft);
  border: 1px solid var(--danger);
  line-height: 1.5;
}
.error-box code {
  display: block;
  margin-top: 6px;
  font-size: 11px;
  white-space: pre-wrap;
}
.empty {
  position: absolute;
  top: 60px;
  left: 0;
  right: 0;
  text-align: center;
  pointer-events: none;
}
.sqlline {
  padding: 2px 8px;
  font-size: 11px;
  border-top: 1px solid var(--border);
  background: var(--bg-elev);
}
.colpicker-backdrop {
  position: fixed;
  inset: 0;
  z-index: 950;
}
.colpicker {
  position: fixed;
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 290px;
  max-height: 60vh;
  padding: 10px;
  background: var(--bg-elev);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius);
  box-shadow: var(--shadow);
}
.cp-actions {
  display: flex;
  gap: 6px;
}
.cp-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  overflow: auto;
  min-height: 0;
}
.cp-list .faint {
  font-size: 11px;
  margin-left: auto;
}
</style>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { monaco } from '../../editor/monaco'
import { keywordBefore } from '../../editor/uppercase'
import { registerCompletion } from '../../editor/completion'
import { formatSql } from '../../editor/format'
import { statementAt, statementRanges } from '../../editor/statement'
import { findParams, substituteParams } from '../../editor/params'
import { useTabsStore, type RunInput } from '../../stores/tabs'
import { useSettingsStore } from '../../stores/settings'
import { useConnectionsStore } from '../../stores/connections'
import { useUiStore } from '../../stores/ui'

const emit = defineEmits<{ cursor: [line: number, column: number]; help: [word: string] }>()

const tabs = useTabsStore()
const settings = useSettingsStore()
const conns = useConnectionsStore()
const ui = useUiStore()

const host = ref<HTMLElement | null>(null)
const editor = shallowRef<monaco.editor.IStandaloneCodeEditor | null>(null)
// One model and one view state per tab: switching tabs swaps models instead
// of re-creating the editor, preserving undo history, cursor and scroll.
const models = new Map<string, monaco.editor.ITextModel>()
const viewStates = new Map<string, monaco.editor.ICodeEditorViewState | null>()
const disposables: monaco.IDisposable[] = []
let currentId: string | null = null
let stmtDecorations: monaco.editor.IEditorDecorationsCollection | null = null

function uriFor(id: string) {
  return monaco.Uri.parse(`inmemory://tab/${id}.sql`)
}

function tabIdOf(model: monaco.editor.ITextModel): string | null {
  for (const [id, m] of models) if (m === model) return id
  return null
}

function modelFor(id: string): monaco.editor.ITextModel | null {
  const tab = tabs.find(id)
  if (!tab || tab.kind !== 'query') return null
  let m = models.get(id)
  if (!m) {
    m = monaco.editor.createModel(tab.sql, 'mysql', uriFor(id))
    m.updateOptions({ tabSize: settings.prefs.editorTabSize, insertSpaces: !settings.prefs.editorUseTabs })
    const model = m
    disposables.push(model.onDidChangeContent(() => tabs.setSql(id, model.getValue())))
    models.set(id, m)
  } else if (m.getValue() !== tab.sql) {
    // Content changed outside the editor (e.g. file reloaded).
    m.pushEditOperations([], [{ range: m.getFullModelRange(), text: tab.sql }], () => null)
  }
  return m
}

function showTab(id: string | null) {
  const ed = editor.value
  if (!ed) return
  if (currentId && models.has(currentId)) viewStates.set(currentId, ed.saveViewState())
  currentId = id
  const m = id ? modelFor(id) : null
  ed.setModel(m)
  if (id && m) {
    const vs = viewStates.get(id)
    if (vs) ed.restoreViewState(vs)
    applyMarker()
    updateStatementHighlight()
    ed.focus()
  }
}

function applyMarker() {
  const t = tabs.active
  const m = t ? models.get(t.id) : null
  if (!m) return
  const mk = t?.marker
  monaco.editor.setModelMarkers(
    m,
    'execution',
    mk && mk.line >= 1 && mk.line <= m.getLineCount()
      ? [{
          severity: monaco.MarkerSeverity.Error,
          message: mk.message,
          startLineNumber: mk.line,
          startColumn: m.getLineFirstNonWhitespaceColumn(mk.line) || 1,
          endLineNumber: mk.line,
          endColumn: m.getLineMaxColumn(mk.line),
        }]
      : [],
  )
  if (mk && mk.line >= 1 && mk.line <= m.getLineCount()) editor.value?.revealLineInCenterIfOutsideViewport(mk.line)
}

let highlightTimer: number | undefined
function updateStatementHighlight() {
  window.clearTimeout(highlightTimer)
  highlightTimer = window.setTimeout(() => {
    const ed = editor.value
    const m = ed?.getModel()
    if (!ed || !m || !stmtDecorations) return
    const text = m.getValue()
    if (statementRanges(text).length < 2 || !ed.getSelection()?.isEmpty()) {
      stmtDecorations.clear()
      return
    }
    const pos = ed.getPosition()
    const r = pos ? statementAt(text, m.getOffsetAt(pos)) : null
    if (!r) {
      stmtDecorations.clear()
      return
    }
    const s = m.getPositionAt(r.start)
    const e = m.getPositionAt(r.end)
    stmtDecorations.set([{ range: new monaco.Range(s.lineNumber, 1, e.lineNumber, 1), options: { isWholeLine: true, linesDecorationsClassName: 'current-statement' } }])
  }, 60)
}

/** Text and cursor to execute for the given mode. */
function runInput(mode: 'statement' | 'script'): RunInput | null {
  const ed = editor.value
  const m = ed?.getModel()
  if (!ed || !m) return null
  const sel = ed.getSelection()
  if (sel && !sel.isEmpty()) {
    // A selection runs exactly the selected text (every statement in it).
    return { sql: m.getValueInRange(sel), cursorOffset: -1, mode: 'script', baseLine: sel.startLineNumber }
  }
  const pos = ed.getPosition()
  return {
    sql: m.getValue(),
    cursorOffset: mode === 'statement' && pos ? m.getOffsetAt(pos) : -1,
    mode,
    baseLine: 1,
  }
}

/** Applies the tab's bind parameters (:name) to the text to run. */
function withParams(t: { params?: Record<string, string>; useParams?: boolean }, input: RunInput): RunInput {
  // On by default as soon as the SQL has :name parameters.
  if (t.useParams === false || !findParams(input.sql).length) return input
  const values = t.params ?? {}
  if (input.cursorOffset >= 0) {
    // Only the statement under the cursor runs: substitute inside it so the
    // cursor offset stays meaningful.
    const r = statementAt(input.sql, input.cursorOffset)
    if (!r) return input
    const text = input.sql.slice(r.start, r.end)
    return { sql: substituteParams(text, values), cursorOffset: -1, mode: 'statement', baseLine: input.baseLine + (input.sql.slice(0, r.start).match(/\n/g)?.length ?? 0) }
  }
  return { ...input, sql: substituteParams(input.sql, values) }
}

async function run(mode: 'statement' | 'script') {
  const t = tabs.active
  const input = runInput(mode)
  if (t && input) await tabs.run(t.id, withParams(t, input))
}

/** Runs EXPLAIN (or a variant) for the statement under the cursor / selection. */
async function explain(prefix = 'EXPLAIN') {
  const t = tabs.active
  const input = runInput('statement')
  if (!t || !input) return
  const p = withParams(t, input)
  let text = p.sql
  if (p.cursorOffset >= 0) {
    const r = statementAt(p.sql, p.cursorOffset)
    if (!r) return
    text = p.sql.slice(r.start, r.end)
  } else {
    const first = statementRanges(text)[0]
    if (first) text = text.slice(first.start, first.end)
  }
  text = text.trim().replace(/;$/, '')
  if (!text) return
  await tabs.run(t.id, { sql: `${prefix} ${text}`, cursorOffset: -1, mode: 'statement', baseLine: p.baseLine })
}

/** Word under the cursor (for SQL help). */
function wordAtCursor(): string {
  const ed = editor.value
  const m = ed?.getModel()
  const pos = ed?.getPosition()
  if (!m || !pos) return ''
  const sel = ed!.getSelection()
  if (sel && !sel.isEmpty()) return m.getValueInRange(sel).trim()
  return m.getWordAtPosition(pos)?.word ?? ''
}

function selectedText(): string {
  const ed = editor.value
  const m = ed?.getModel()
  const sel = ed?.getSelection()
  return m && sel && !sel.isEmpty() ? m.getValueInRange(sel) : ''
}

function format() {
  const ed = editor.value
  const m = ed?.getModel()
  const t = tabs.active
  if (!ed || !m || !t) return
  const dialect = t.connectionId && conns.byId.get(t.connectionId)?.kind === 'mariadb' ? 'mariadb' : 'mysql'
  const sel = ed.getSelection()
  const range = sel && !sel.isEmpty() ? sel : m.getFullModelRange()
  try {
    const out = formatSql(m.getValueInRange(range), dialect)
    ed.executeEdits('format', [{ range, text: out }])
    ed.pushUndoStop()
  } catch (e) {
    ui.toast('warning', `Não foi possível formatar: ${e instanceof Error ? e.message.split('\n')[0] : String(e)}`)
  }
}

function insertText(text: string) {
  const ed = editor.value
  if (!ed || !ed.getModel()) return
  const sel = ed.getSelection()
  if (!sel) return
  ed.executeEdits('insert', [{ range: sel, text, forceMoveMarkers: true }])
  ed.pushUndoStop()
  ed.focus()
}

function focus() {
  editor.value?.focus()
}

defineExpose({ run, format, insertText, focus, explain, wordAtCursor, selectedText })

onMounted(() => {
  if (!host.value) return
  const ed = monaco.editor.create(host.value, {
    model: null,
    theme: settings.effectiveTheme === 'dark' ? 'studio-dark' : 'studio-light',
    fontSize: settings.prefs.editorFontSize,
    fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', 'Source Code Pro', 'DejaVu Sans Mono', monospace",
    automaticLayout: true,
    minimap: { enabled: settings.prefs.editorMinimap },
    wordWrap: settings.prefs.editorWordWrap ? 'on' : 'off',
    scrollBeyondLastLine: false,
    fixedOverflowWidgets: true,
    wordBasedSuggestions: 'off',
    quickSuggestions: settings.prefs.autocompleteOff ? false : { other: true, comments: false, strings: false },
    suggestOnTriggerCharacters: true,
    suggest: { showWords: false, showIcons: true, preview: false, insertMode: 'replace' },
    tabSize: settings.prefs.editorTabSize,
    renderLineHighlight: 'all',
    lineNumbersMinChars: 3,
    glyphMargin: false,
    folding: true,
    bracketPairColorization: { enabled: true },
    padding: { top: 8 },
    smoothScrolling: true,
    contextmenu: true,
    'semanticHighlighting.enabled': false,
  })
  editor.value = ed
  stmtDecorations = ed.createDecorationsCollection()

  ed.addAction({ id: 'dbstudio.runStatement', label: 'Executar instrução sob o cursor', keybindings: [monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter], contextMenuGroupId: 'navigation', contextMenuOrder: 1, run: () => run('statement') })
  ed.addAction({ id: 'dbstudio.runScript', label: 'Executar script', keybindings: [monaco.KeyMod.CtrlCmd | monaco.KeyMod.Shift | monaco.KeyCode.Enter, monaco.KeyCode.F5], contextMenuGroupId: 'navigation', contextMenuOrder: 2, run: () => run('script') })
  ed.addAction({ id: 'dbstudio.cancel', label: 'Cancelar execução', keybindings: [monaco.KeyMod.CtrlCmd | monaco.KeyMod.Alt | monaco.KeyCode.KeyC], run: () => { if (tabs.active) void tabs.cancel(tabs.active.id) } })
  ed.addAction({ id: 'dbstudio.format', label: 'Formatar SQL', keybindings: [monaco.KeyMod.Shift | monaco.KeyMod.Alt | monaco.KeyCode.KeyF], contextMenuGroupId: '1_modification', run: format })
  ed.addAction({ id: 'dbstudio.explain', label: 'EXPLAIN da instrução', keybindings: [monaco.KeyMod.CtrlCmd | monaco.KeyMod.Shift | monaco.KeyCode.KeyE], contextMenuGroupId: 'navigation', contextMenuOrder: 3, run: () => explain() })
  ed.addAction({ id: 'dbstudio.help', label: 'Ajuda SQL da palavra sob o cursor', keybindings: [monaco.KeyCode.F1], contextMenuGroupId: 'navigation', contextMenuOrder: 4, run: () => emit('help', wordAtCursor()) })

  disposables.push(
    registerCompletion((model) => {
      const id = tabIdOf(model)
      const t = id ? tabs.find(id) : undefined
      return t ? { connectionId: t.connectionId, database: t.database } : null
    }),
    ed.onDidChangeCursorPosition((e) => {
      emit('cursor', e.position.lineNumber, e.position.column)
      updateStatementHighlight()
    }),
    ed.onDidChangeModelContent(updateStatementHighlight),
    ed.onDidChangeModelContent((e) => {
      if (!settings.prefs.uppercaseKeywords || e.isUndoing || e.isRedoing || e.isFlush || e.changes.length !== 1) return
      const ch = e.changes[0]!
      if (ch.rangeLength !== 0 || ch.text.length !== 1 && !/^\r?\n\s*$/.test(ch.text)) return
      const model = ed.getModel()
      if (!model || model.getValueLength() > 500_000) return
      const text = model.getValue()
      const hit = keywordBefore(text, ch.rangeOffset, ch.text[0] === '\r' ? '\n' : ch.text[0]!)
      if (!hit) return
      const a = model.getPositionAt(hit.start)
      const b = model.getPositionAt(hit.end)
      // Applied after the current edit so the typed character stays put.
      queueMicrotask(() => ed.executeEdits('dbstudio.uppercase', [{ range: new monaco.Range(a.lineNumber, a.column, b.lineNumber, b.column), text: hit.upper }]))
    }),
  )
  showTab(tabs.activeId)
})

watch(() => tabs.activeId, (id) => showTab(id))

// Dispose models of closed tabs.
watch(
  () => tabs.tabs.map((t) => t.id),
  (ids) => {
    const alive = new Set(ids)
    for (const [id, m] of models) {
      if (!alive.has(id)) {
        if (editor.value?.getModel() === m) editor.value.setModel(null)
        m.dispose()
        models.delete(id)
        viewStates.delete(id)
        if (currentId === id) currentId = null
      }
    }
  },
)

// External content changes for the active tab (open file, history, etc.).
watch(
  () => tabs.active?.sql,
  (sql) => {
    const t = tabs.active
    const m = t ? models.get(t.id) : null
    if (m && sql !== undefined && m.getValue() !== sql) {
      m.pushEditOperations([], [{ range: m.getFullModelRange(), text: sql }], () => null)
    }
  },
)

watch(() => tabs.active?.marker, applyMarker)
watch(() => settings.effectiveTheme, (t) => monaco.editor.setTheme(t === 'dark' ? 'studio-dark' : 'studio-light'))
watch(() => settings.prefs.editorFontSize, (s) => editor.value?.updateOptions({ fontSize: s }))
watch(() => [settings.prefs.editorTabSize, settings.prefs.editorUseTabs, settings.prefs.editorWordWrap, settings.prefs.editorMinimap, settings.prefs.autocompleteOff] as const, ([tab, useTabs, wrap, minimap, noAuto]) => {
  editor.value?.updateOptions({ tabSize: tab, wordWrap: wrap ? 'on' : 'off', minimap: { enabled: minimap }, quickSuggestions: noAuto ? false : { other: true, comments: false, strings: false } })
  for (const m of models.values()) m.updateOptions({ tabSize: tab, insertSpaces: !useTabs })
})

onBeforeUnmount(() => {
  window.clearTimeout(highlightTimer)
  for (const d of disposables) d.dispose()
  for (const m of models.values()) m.dispose()
  models.clear()
  editor.value?.dispose()
})
</script>

<template>
  <div ref="host" class="editor-host" />
</template>

<style scoped>
.editor-host {
  width: 100%;
  height: 100%;
  min-height: 0;
  user-select: text;
}
:deep(.current-statement) {
  border-left: 3px solid var(--accent);
  margin-left: 3px;
}
</style>

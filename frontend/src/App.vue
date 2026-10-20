<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import Sidebar from './components/sidebar/Sidebar.vue'
import TabBar from './components/editor/TabBar.vue'
import EditorToolbar from './components/editor/EditorToolbar.vue'
import SqlEditor from './components/editor/SqlEditor.vue'
import ResultsPanel from './components/results/ResultsPanel.vue'
import StatusBar from './components/layout/StatusBar.vue'
import ConnectionDialog from './components/dialogs/ConnectionDialog.vue'
import ConfirmDialog from './components/dialogs/ConfirmDialog.vue'
import PromptDialog from './components/dialogs/PromptDialog.vue'
import PreferencesDialog from './components/dialogs/PreferencesDialog.vue'
import Toasts from './components/dialogs/Toasts.vue'
import CreateDatabaseDialog from './components/dialogs/CreateDatabaseDialog.vue'
import Icon from './components/common/Icon.vue'
import ObjectView from './components/objects/ObjectView.vue'
import SqlLogPanel from './components/layout/SqlLogPanel.vue'
import QueryHelpers from './components/editor/QueryHelpers.vue'
import TasksPanel from './components/layout/TasksPanel.vue'
import ExportDialog from './components/dialogs/ExportDialog.vue'
import DumpDialog from './components/dialogs/DumpDialog.vue'
import CsvImportDialog from './components/dialogs/CsvImportDialog.vue'
import TableToolsDialog from './components/dialogs/TableToolsDialog.vue'
import CopyTableDialog from './components/dialogs/CopyTableDialog.vue'
import GenerateDataDialog from './components/dialogs/GenerateDataDialog.vue'
import CompareDialog from './components/dialogs/CompareDialog.vue'
import DatabaseFilterDialog from './components/dialogs/DatabaseFilterDialog.vue'
import { useSettingsStore } from './stores/settings'
import { useConnectionsStore } from './stores/connections'
import { useTabsStore } from './stores/tabs'
import { useUiStore } from './stores/ui'
import { useMetadataStore } from './stores/metadata'
import { backend } from './api/backend'
import { toAppError } from './api/errors'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { actionFor, comboOf, effectiveShortcuts } from './utils/shortcuts'
import { useTasksStore } from './stores/tasks'

const settings = useSettingsStore()
const conns = useConnectionsStore()
const tabs = useTabsStore()
const ui = useUiStore()
const meta = useMetadataStore()
const tasks = useTasksStore()

const editor = ref<InstanceType<typeof SqlEditor> | null>(null)
const sidebar = ref<InstanceType<typeof Sidebar> | null>(null)
const cursor = ref({ line: 1, column: 1 })
const ready = ref(false)
const sidebarHidden = ref(false)

const active = computed(() => tabs.active)
const isQuery = computed(() => active.value?.kind === 'query')
const helpTopic = ref<{ word: string; seq: number } | null>(null)

function openHelp(word: string) {
  tabs.layout.helpersVisible = true
  helpTopic.value = { word, seq: (helpTopic.value?.seq ?? 0) + 1 }
}

function dragHelpers(e: MouseEvent) {
  e.preventDefault()
  const startX = e.clientX
  const start = tabs.layout.helpersWidth
  const move = (ev: MouseEvent) => (tabs.layout.helpersWidth = Math.max(220, Math.min(window.innerWidth * 0.5, start - (ev.clientX - startX))))
  const up = () => { window.removeEventListener('mousemove', move); window.removeEventListener('mouseup', up) }
  window.addEventListener('mousemove', move)
  window.addEventListener('mouseup', up)
}

// ---- layout splitters ----
function dragSidebar(e: MouseEvent) {
  e.preventDefault()
  const startX = e.clientX
  const start = tabs.layout.sidebarWidth
  const move = (ev: MouseEvent) => (tabs.layout.sidebarWidth = Math.max(180, Math.min(window.innerWidth * 0.6, start + ev.clientX - startX)))
  const up = () => { window.removeEventListener('mousemove', move); window.removeEventListener('mouseup', up) }
  window.addEventListener('mousemove', move)
  window.addEventListener('mouseup', up)
}

function dragResults(e: MouseEvent) {
  e.preventDefault()
  const startY = e.clientY
  const start = tabs.layout.resultsHeight
  const move = (ev: MouseEvent) => (tabs.layout.resultsHeight = Math.max(90, Math.min(window.innerHeight - 200, start - (ev.clientY - startY))))
  const up = () => { window.removeEventListener('mousemove', move); window.removeEventListener('mouseup', up) }
  window.addEventListener('mousemove', move)
  window.addEventListener('mouseup', up)
}

function dragLog(e: MouseEvent) {
  e.preventDefault()
  const startY = e.clientY
  const start = tabs.layout.logHeight
  const move = (ev: MouseEvent) => (tabs.layout.logHeight = Math.max(80, Math.min(window.innerHeight - 260, start - (ev.clientY - startY))))
  const up = () => { window.removeEventListener('mousemove', move); window.removeEventListener('mouseup', up) }
  window.addEventListener('mousemove', move)
  window.addEventListener('mouseup', up)
}

function splitterKey(e: KeyboardEvent, which: 'sidebar' | 'results') {
  const step = e.shiftKey ? 50 : 10
  if (which === 'sidebar' && (e.key === 'ArrowLeft' || e.key === 'ArrowRight')) {
    tabs.layout.sidebarWidth = Math.max(180, tabs.layout.sidebarWidth + (e.key === 'ArrowRight' ? step : -step))
  } else if (which === 'results' && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
    tabs.layout.resultsHeight = Math.max(90, tabs.layout.resultsHeight + (e.key === 'ArrowUp' ? step : -step))
  }
}

// ---- files and saved queries ----
async function openFile() {
  try {
    const f = await backend.openSqlFile()
    if (!f) return
    const existing = tabs.tabs.find((t) => t.filePath === f.path)
    if (existing) {
      tabs.activate(existing.id)
      return
    }
    const cur = active.value
    if (cur && !cur.sql.trim() && !cur.filePath && cur.status === 'idle') {
      tabs.setSql(cur.id, f.content)
      cur.title = f.name
      tabs.markSaved(cur.id, { filePath: f.path })
    } else {
      const t = tabs.open({ title: f.name, sql: f.content, filePath: f.path, savedContent: f.content })
      tabs.activate(t.id)
    }
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}

async function saveFile(as = false) {
  const t = active.value
  if (!t) return
  try {
    if (t.filePath && !as) {
      await backend.saveSqlFile(t.filePath, t.sql)
      tabs.markSaved(t.id, {})
      ui.toast('success', 'Arquivo salvo.', 2000)
      return
    }
    if (t.savedQueryId && !as) {
      await saveQuery()
      return
    }
    const f = await backend.saveSqlFileAs(t.title, t.sql)
    if (f) {
      tabs.markSaved(t.id, { filePath: f.path, title: f.name })
      ui.toast('success', `Salvo em ${f.path}`)
    }
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}

async function saveQuery() {
  const t = active.value
  if (!t) return
  let name = t.title
  if (!t.savedQueryId) {
    const r = await ui.prompt({ title: 'Salvar consulta', label: 'Nome', value: t.title, confirmLabel: 'Salvar' })
    if (!r || !r.value.trim()) return
    name = r.value.trim()
  }
  try {
    const q = await backend.saveQuery({ id: t.savedQueryId ?? '', name, sql: t.sql, connectionId: t.connectionId ?? '' })
    tabs.markSaved(t.id, { savedQueryId: q.id, title: q.name })
    ui.toast('success', `Consulta "${q.name}" salva.`, 2500)
    await sidebar.value?.reloadSaved()
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}

// ---- global shortcuts ----
const shortcutMap = computed(() => effectiveShortcuts(settings.prefs.shortcuts))

function cycleTab(back: boolean) {
  if (!tabs.tabs.length) return
  const i = tabs.tabs.findIndex((t) => t.id === tabs.activeId)
  const next = tabs.tabs[(i + (back ? -1 : 1) + tabs.tabs.length) % tabs.tabs.length]
  if (next) tabs.activate(next.id)
}

function focusTree() {
  sidebarHidden.value = false
  void nextTick(() => document.querySelector<HTMLInputElement>('.side .search input')?.focus())
}

function toggleEditorResults() {
  const inResults = !!(document.activeElement as HTMLElement | null)?.closest?.('.results')
  if (inResults || !isQuery.value) editor.value?.focus()
  else document.querySelector<HTMLElement>('.results [role=grid], .results .grid')?.focus()
}

function onKey(e: KeyboardEvent) {
  if (ui.confirmState || ui.promptState || ui.connectionDialog.open || ui.ddlDialog || ui.exportDialog || ui.dumpDialog || ui.csvDialog || ui.toolsDialog || ui.copyTableDialog || ui.generateDialog || ui.compareDialog || ui.databaseFilterDialog || ui.preferencesOpen) return
  const mod = e.ctrlKey || e.metaKey
  const handled = () => { e.preventDefault(); e.stopPropagation() }
  // Ctrl+Enter from the helpers panel (e.g. after typing a parameter) runs the tab.
  if (mod && e.key === 'Enter' && isQuery.value && (e.target as HTMLElement | null)?.closest?.('.qh')) {
    handled()
    void editor.value?.run(e.shiftKey ? 'script' : 'statement')
    return
  }
  const combo = comboOf(e)
  // Plain keys are left to the focused widget, except function keys.
  if (!mod && !e.altKey && !/^F\d+$/.test(combo)) return
  const action = actionFor(combo, shortcutMap.value)
  if (!action) return
  if ((action === 'saveFile' || action === 'saveFileAs') && !isQuery.value) return // object tabs save themselves
  handled()
  switch (action) {
    case 'newTab': tabs.open(); break
    case 'closeTab': if (active.value) void tabs.close(active.value.id); break
    case 'nextTab': case 'nextTabAlt': cycleTab(false); break
    case 'prevTab': case 'prevTabAlt': cycleTab(true); break
    case 'openFile': void openFile(); break
    case 'saveFile': void saveFile(false); break
    case 'saveFileAs': void saveFile(true); break
    case 'preferences': ui.preferencesOpen = true; break
    case 'toggleSidebar': sidebarHidden.value = !sidebarHidden.value; break
    case 'toggleLog': tabs.layout.logVisible = !tabs.layout.logVisible; break
    case 'toggleHelpers': tabs.layout.helpersVisible = !tabs.layout.helpersVisible; break
    case 'toggleTasks': tasks.panelOpen = !tasks.panelOpen; break
    case 'focusTree': focusTree(); break
    case 'focusEditor': toggleEditorResults(); break
    case 'serverView': if (active.value?.connectionId) tabs.openObject({ kind: 'host', connectionId: active.value.connectionId }); break
    default: {
      const n = /^tab(\d)$/.exec(action)
      const t = n ? tabs.tabs[Number(n[1]) - 1] : undefined
      if (t) tabs.activate(t.id)
    }
  }
}

let offInvalidated: (() => void) | undefined

onMounted(async () => {
  window.addEventListener('keydown', onKey, true)
  try {
    await settings.load()
  } catch (e) {
    ui.toast('error', `Falha ao carregar preferências: ${toAppError(e).message}`)
  }
  try {
    await conns.load()
  } catch (e) {
    ui.toast('error', `Falha ao carregar conexões: ${toAppError(e).message}`)
  }
  await tabs.restore()
  if (settings.prefs.reconnectOnStartup) {
    for (const id of tabs.lastOpenConnections) await conns.connect(id)
  }
  for (const w of settings.info?.warnings ?? []) ui.toast('warning', w, 12000)
  try {
    offInvalidated = EventsOn('metadata:invalidated', (conn: string, db: string) => void meta.invalidate(conn, db, false))
  } catch {
    // runtime unavailable (e.g. tests)
  }
  ready.value = true
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey, true)
  offInvalidated?.()
})

window.addEventListener('beforeunload', () => void tabs.flush())
</script>

<template>
  <div class="app" :style="{ gridTemplateColumns: sidebarHidden ? '0 0 1fr' : `${tabs.layout.sidebarWidth}px 4px 1fr` }">
    <Sidebar v-show="!sidebarHidden" ref="sidebar" class="side" @insert="editor?.insertText($event)" />
    <div v-show="!sidebarHidden" class="vsplit" role="separator" aria-orientation="vertical" aria-label="Redimensionar barra lateral" tabindex="0" @mousedown="dragSidebar" @keydown="splitterKey($event, 'sidebar')" />
    <main class="main">
      <TabBar />
      <EditorToolbar
        v-if="active && isQuery" @run="editor?.run($event)" @format="editor?.format()" @save="saveFile()" @save-as="saveFile(true)" @save-query="saveQuery" @open="openFile"
        @explain="editor?.explain($event)" @helpers="tabs.layout.helpersVisible = !tabs.layout.helpersVisible"
      />
      <div v-show="active && isQuery" class="work">
        <div class="editor-row">
          <div class="editor-area" :class="{ prod: active?.connectionId && conns.byId.get(active.connectionId)?.environment === 'production' }">
            <SqlEditor ref="editor" @cursor="(l, c) => (cursor = { line: l, column: c })" @help="openHelp" />
          </div>
          <template v-if="tabs.layout.helpersVisible && active && isQuery">
            <div class="vsplit" role="separator" aria-orientation="vertical" aria-label="Redimensionar painel de ajuda" @mousedown="dragHelpers" />
            <QueryHelpers
              :tab="active" :help-topic="helpTopic" :selection="() => editor?.selectedText() ?? ''" :style="{ width: tabs.layout.helpersWidth + 'px' }"
              @insert="editor?.insertText($event)" @close="tabs.layout.helpersVisible = false"
            />
          </template>
        </div>
        <div class="hsplit" role="separator" aria-orientation="horizontal" aria-label="Redimensionar painel de resultados" tabindex="0" @mousedown="dragResults" @keydown="splitterKey($event, 'results')" />
        <div class="results-area" :style="{ height: tabs.layout.resultsHeight + 'px' }">
          <ResultsPanel v-if="active && isQuery" :key="active.id" :tab="active" />
        </div>
      </div>
      <div v-if="active && !isQuery" class="object-area">
        <ObjectView :tab="active" />
      </div>
      <div v-if="!active && ready" class="welcome">
        <Icon name="database" :size="40" />
        <h1>Anansi</h1>
        <template v-if="!conns.profiles.length">
          <p>Comece cadastrando uma conexão MySQL ou MariaDB.</p>
          <button class="btn primary" @click="ui.openConnectionDialog()"><Icon name="plus" :size="14" /> Adicionar conexão</button>
        </template>
        <template v-else>
          <p>Abra uma aba SQL para consultar seus bancos, ou dê dois cliques num objeto da árvore.</p>
          <div class="actions">
            <button class="btn primary" @click="tabs.open()"><Icon name="plus" :size="14" /> Nova consulta <kbd>Ctrl+T</kbd></button>
            <button class="btn" @click="openFile"><Icon name="folder" :size="14" /> Abrir arquivo <kbd>Ctrl+O</kbd></button>
          </div>
        </template>
      </div>
    </main>
    <div v-if="tabs.layout.logVisible" class="log-area" :style="{ height: tabs.layout.logHeight + 'px' }">
      <div class="hsplit" role="separator" aria-orientation="horizontal" aria-label="Redimensionar log" @mousedown="dragLog" />
      <SqlLogPanel class="log-panel" @close="tabs.layout.logVisible = false" />
    </div>
    <StatusBar class="statusbar" :line="cursor.line" :column="cursor.column" @toggle-log="tabs.layout.logVisible = !tabs.layout.logVisible" />
  </div>
  <ConnectionDialog />
  <PreferencesDialog />
  <CreateDatabaseDialog />
  <ExportDialog />
  <DumpDialog />
  <CsvImportDialog />
  <TableToolsDialog />
  <CopyTableDialog />
  <GenerateDataDialog />
  <CompareDialog />
  <DatabaseFilterDialog />
  <TasksPanel />
  <ConfirmDialog />
  <PromptDialog />
  <Toasts />
</template>

<style scoped>
.app {
  display: grid;
  grid-template-rows: 1fr auto auto;
  height: 100%;
}
.object-area {
  flex: 1;
  min-height: 0;
}
.log-area {
  grid-column: 1 / -1;
  grid-row: 2;
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.log-panel {
  flex: 1;
  min-height: 0;
}
.side {
  grid-row: 1;
  border-right: 1px solid var(--border);
  overflow: hidden;
}
.vsplit {
  grid-row: 1;
  cursor: col-resize;
  background: transparent;
}
.vsplit:hover,
.vsplit:focus-visible,
.hsplit:hover,
.hsplit:focus-visible {
  background: var(--accent-soft);
  outline: none;
}
.main {
  grid-row: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
}
.work {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}
.work.hidden {
  display: none;
}
.editor-row {
  display: flex;
  flex: 1;
  min-height: 80px;
}
.editor-row > .vsplit {
  width: 4px;
  flex: none;
}
.editor-row > aside {
  flex: none;
}
.editor-area {
  flex: 1;
  min-width: 0;
  min-height: 80px;
  position: relative;
}
.editor-area.prod {
  box-shadow: inset 3px 0 0 var(--env-production);
}
.hsplit {
  height: 5px;
  cursor: row-resize;
  border-top: 1px solid var(--border);
  flex: none;
}
.results-area {
  flex: none;
  min-height: 90px;
  max-height: calc(100% - 120px);
}
.statusbar {
  grid-column: 1 / -1;
  grid-row: 3;
}
.welcome {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  color: var(--text-muted);
  padding: 24px;
  text-align: center;
}
.welcome h1 {
  margin: 0;
  font-size: 20px;
  color: var(--text);
}
.welcome p {
  margin: 0;
}
.welcome .actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: center;
}
kbd {
  font-family: var(--font-mono);
  font-size: 10px;
  opacity: 0.75;
}
</style>

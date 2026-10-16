<script setup lang="ts">
// Query helpers panel (HeidiSQL's helpers at the right of the query
// editor): table columns with statement generators, the server's SQL help,
// functions and keywords, snippets and bind parameters.
import { computed, ref, watch } from 'vue'
import type { Tab } from '../../stores/tabs'
import { useTabsStore } from '../../stores/tabs'
import { useMetadataStore } from '../../stores/metadata'
import { useConnectionsStore } from '../../stores/connections'
import { useUiStore } from '../../stores/ui'
import { backend } from '../../api/backend'
import { toAppError } from '../../api/errors'
import type { ColumnInfo, HelpResult, Snippet, TableInfo, Vocabulary } from '../../api/types'
import Icon from '../common/Icon.vue'
import { tableStatement, type StatementKind } from '../../composables/templates'
import { findParams } from '../../editor/params'
import { forceQuoteIdent, quoteIdent } from '../../utils/text'
import { textMatcher } from '../../utils/format'

const props = defineProps<{ tab: Tab; helpTopic?: { word: string; seq: number } | null; selection: () => string }>()
const emit = defineEmits<{ insert: [text: string]; close: [] }>()
const tabs = useTabsStore()
const meta = useMetadataStore()
const conns = useConnectionsStore()
const ui = useUiStore()

type Pane = 'columns' | 'help' | 'vocab' | 'snippets' | 'params'
const pane = ref<Pane>('columns')
const conn = computed(() => props.tab.connectionId ?? '')
const connected = computed(() => !!conn.value && conns.state(conn.value).status === 'connected')
const filter = ref('')

// ---- columns ----
const table = ref('')
const tables = computed(() => (meta.entry<TableInfo[]>(conn.value, 'tables', props.tab.database)?.data ?? []).map((t) => t.name))
const columns = computed(() => meta.entry<ColumnInfo[]>(conn.value, 'cols', props.tab.database, table.value)?.data ?? [])
const picked = ref(new Set<string>())
watch(() => [conn.value, props.tab.database, connected.value] as const, ([c, db, ok]) => {
  if (c && db && ok) void meta.tables(c, db).catch(() => undefined)
}, { immediate: true })
watch(table, (t) => {
  picked.value = new Set()
  if (t && conn.value) void meta.columns(conn.value, props.tab.database, t).catch(() => undefined)
})
watch(tables, (list) => {
  if (!list.includes(table.value)) table.value = list[0] ?? ''
})
function generate(kind: StatementKind) {
  const cols = picked.value.size ? columns.value.filter((c) => picked.value.has(c.name)) : columns.value
  emit('insert', tableStatement(kind, props.tab.database, table.value, cols))
}
function togglePick(name: string) {
  const s = new Set(picked.value)
  if (s.has(name)) s.delete(name)
  else s.add(name)
  picked.value = s
}

// ---- SQL help ----
const help = ref<HelpResult | null>(null)
const helpTrail = ref<string[]>([])
const helpLoading = ref(false)
const helpError = ref('')
const helpSearch = ref('')
async function openHelp(topic: string, push = true) {
  if (!connected.value) {
    helpError.value = 'Conecte a aba a um servidor para consultar a ajuda.'
    return
  }
  helpLoading.value = true
  helpError.value = ''
  try {
    const h = await backend.sqlHelp(conn.value, topic)
    if (h.kind === 'none' && !topic.includes('%')) {
      // Fall back to a prefix search, like the mysql client.
      const s = await backend.sqlHelp(conn.value, topic + '%')
      help.value = s.kind === 'none' ? h : s
    } else help.value = h
    if (push) helpTrail.value = [...helpTrail.value.filter((x) => x !== topic), topic].slice(-8)
  } catch (e) {
    helpError.value = toAppError(e).message
  } finally {
    helpLoading.value = false
  }
}
watch(() => props.helpTopic?.seq, () => {
  if (!props.helpTopic) return
  pane.value = 'help'
  helpSearch.value = props.helpTopic.word
  void openHelp(props.helpTopic.word || 'contents')
})
watch(pane, (p) => {
  if (p === 'help' && !help.value) void openHelp('contents')
  if (p === 'vocab' && !vocab.value) void loadVocab()
  if (p === 'snippets') void loadSnippets()
})

// ---- vocabulary ----
const vocab = ref<Vocabulary | null>(null)
async function loadVocab() {
  try {
    vocab.value = await backend.sqlVocabulary()
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}
const match = computed(() => textMatcher(filter.value))
const functions = computed(() => (vocab.value?.functions ?? []).filter((f) => match.value(f.name)))
const keywords = computed(() => (vocab.value?.keywords ?? []).filter((k) => match.value(k)))

// ---- snippets ----
const snippets = ref<Snippet[]>([])
async function loadSnippets() {
  try {
    snippets.value = await backend.listSnippets()
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}
async function insertSnippet(name: string) {
  try {
    emit('insert', await backend.readSnippet(name))
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}
async function newSnippet() {
  const text = props.selection() || props.tab.sql
  if (!text.trim()) {
    ui.toast('info', 'Selecione o SQL (ou escreva na aba) para criar o snippet.')
    return
  }
  const r = await ui.prompt({ title: 'Novo snippet', label: 'Nome', message: props.selection() ? 'Com o texto selecionado.' : 'Com o conteúdo da aba.', confirmLabel: 'Salvar' })
  if (!r?.value.trim()) return
  try {
    await backend.saveSnippet(r.value.trim(), text)
    await loadSnippets()
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}
async function renameSnippet(s: Snippet) {
  const r = await ui.prompt({ title: 'Renomear snippet', label: 'Nome', value: s.name, confirmLabel: 'Renomear' })
  if (!r?.value.trim() || r.value.trim() === s.name) return
  try {
    await backend.renameSnippet(s.name, r.value.trim())
    await loadSnippets()
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}
async function deleteSnippet(s: Snippet) {
  if (!(await ui.confirm({ title: 'Excluir snippet', message: `Excluir o snippet "${s.name}"?`, confirmLabel: 'Excluir', danger: true }))) return
  await backend.deleteSnippet(s.name).catch((e) => ui.toast('error', toAppError(e).message))
  await loadSnippets()
}
const shownSnippets = computed(() => snippets.value.filter((s) => match.value(s.name)))

// ---- parameters ----
const params = computed(() => findParams(props.tab.sql))
function setParam(name: string, v: string) {
  const t = tabs.find(props.tab.id)
  if (t) t.params = { ...(t.params ?? {}), [name]: v }
}
function setUseParams(v: boolean) {
  const t = tabs.find(props.tab.id)
  if (t) t.useParams = v
}

const panes: { id: Pane; icon: 'column' | 'info' | 'fn' | 'bookmark' | 'edit'; label: string }[] = [
  { id: 'columns', icon: 'column', label: 'Colunas' },
  { id: 'help', icon: 'info', label: 'Ajuda SQL' },
  { id: 'vocab', icon: 'fn', label: 'Funções e palavras-chave' },
  { id: 'snippets', icon: 'bookmark', label: 'Snippets' },
  { id: 'params', icon: 'edit', label: 'Parâmetros' },
]
</script>

<template>
  <aside class="qh" aria-label="Ajudantes de consulta">
    <header>
      <nav role="tablist">
        <button v-for="p in panes" :key="p.id" role="tab" :aria-selected="pane === p.id" :class="{ active: pane === p.id }" :title="p.label" @click="pane = p.id">
          <Icon :name="p.icon" :size="14" />
          <span v-if="p.id === 'params' && params.length" class="n">{{ params.length }}</span>
        </button>
      </nav>
      <span class="title">{{ panes.find((p) => p.id === pane)?.label }}</span>
      <button class="icon-btn" title="Fechar painel" aria-label="Fechar painel" @click="emit('close')"><Icon name="x" :size="12" /></button>
    </header>

    <div v-if="pane === 'columns'" class="body">
      <p v-if="!tab.database" class="faint pad">Escolha um database na barra da aba.</p>
      <template v-else>
        <select v-model="table" class="select compact" aria-label="Tabela">
          <option v-for="t in tables" :key="t" :value="t">{{ t }}</option>
        </select>
        <div class="gen">
          <button v-for="k in (['select', 'insert', 'update', 'delete'] as StatementKind[])" :key="k" class="btn small" :disabled="!table" :title="`Gerar ${k.toUpperCase()} com as colunas ${picked.size ? 'marcadas' : 'da tabela'}`" @click="generate(k)">{{ k.toUpperCase() }}</button>
        </div>
        <ul class="list">
          <li v-for="c in columns" :key="c.name" :title="c.comment || c.columnType" @dblclick="emit('insert', quoteIdent(c.name))">
            <input type="checkbox" :checked="picked.has(c.name)" :aria-label="`Marcar ${c.name}`" @change="togglePick(c.name)" />
            <Icon :name="c.key === 'PRI' ? 'key' : 'column'" :size="12" class="ico" />
            <span class="ellipsis nm">{{ c.name }}</span>
            <span class="faint ellipsis ty">{{ c.columnType }}</span>
          </li>
        </ul>
        <p class="faint hint">Duplo clique insere o nome · marque colunas para os geradores.</p>
        <button v-if="table" class="btn small" @click="emit('insert', `${forceQuoteIdent(tab.database)}.${forceQuoteIdent(table)}`)">Inserir nome da tabela</button>
      </template>
    </div>

    <div v-else-if="pane === 'help'" class="body">
      <form class="search" @submit.prevent="openHelp(helpSearch || 'contents')">
        <Icon name="search" :size="12" />
        <input v-model="helpSearch" placeholder="Tópico, função ou padrão com %" aria-label="Buscar na ajuda" />
      </form>
      <div class="trail">
        <button class="link" @click="openHelp('contents')">Conteúdo</button>
        <template v-for="t in helpTrail.filter((x) => x !== 'contents')" :key="t"> › <button class="link" @click="openHelp(t, false)">{{ t }}</button></template>
      </div>
      <div v-if="helpLoading" class="faint pad"><span class="spinner" /></div>
      <p v-else-if="helpError" class="err pad">{{ helpError }}</p>
      <template v-else-if="help">
        <ul v-if="help.kind === 'list'" class="list">
          <li v-for="it in help.items ?? []" :key="it.name" class="clickable" @click="openHelp(it.name)">
            <Icon :name="it.isCategory ? 'folder' : 'file'" :size="12" class="ico" /> <span class="ellipsis">{{ it.name }}</span>
          </li>
        </ul>
        <div v-else-if="help.kind === 'topic'" class="topic selectable">
          <h4>{{ help.name }}</h4>
          <pre>{{ help.description }}</pre>
          <template v-if="help.example">
            <div class="ex-head"><span class="muted">Exemplo</span><button class="btn small" @click="emit('insert', help.example)">Inserir</button></div>
            <pre class="mono ex">{{ help.example }}</pre>
          </template>
        </div>
        <p v-else class="faint pad">Nada encontrado para “{{ help.name }}”.</p>
      </template>
    </div>

    <div v-else-if="pane === 'vocab'" class="body">
      <div class="search"><Icon name="search" :size="12" /><input v-model="filter" placeholder="Filtrar" aria-label="Filtrar" /></div>
      <h5>Funções</h5>
      <ul class="list">
        <li v-for="f in functions" :key="f.name" :title="f.signature" class="clickable" @dblclick="emit('insert', f.name + '(')">
          <Icon name="fn" :size="12" class="ico" /> <span class="mono ellipsis">{{ f.signature }}</span>
          <button class="icon-btn mini" title="Ajuda" aria-label="Ajuda" @click.stop="pane = 'help'; openHelp(f.name)"><Icon name="info" :size="11" /></button>
        </li>
      </ul>
      <h5>Palavras reservadas</h5>
      <div class="kw">
        <button v-for="k in keywords" :key="k" class="chip mono" @dblclick="emit('insert', k + ' ')">{{ k }}</button>
      </div>
      <p class="faint hint">Duplo clique insere no editor.</p>
    </div>

    <div v-else-if="pane === 'snippets'" class="body">
      <div class="row">
        <button class="btn small" @click="newSnippet"><Icon name="plus" :size="11" /> Novo</button>
        <button class="btn small" title="Abrir a pasta de snippets" @click="backend.openSnippetsFolder().catch(() => undefined)"><Icon name="folder" :size="12" /></button>
        <button class="icon-btn" title="Atualizar" aria-label="Atualizar" @click="loadSnippets"><Icon name="refresh" :size="12" /></button>
      </div>
      <div class="search"><Icon name="search" :size="12" /><input v-model="filter" placeholder="Filtrar" aria-label="Filtrar snippets" /></div>
      <ul class="list">
        <li v-for="s in shownSnippets" :key="s.name" class="clickable" :title="`${s.size} bytes`" @dblclick="insertSnippet(s.name)">
          <Icon name="file" :size="12" class="ico" /> <span class="ellipsis nm">{{ s.name }}</span>
          <button class="icon-btn mini" title="Inserir" aria-label="Inserir" @click.stop="insertSnippet(s.name)"><Icon name="plus" :size="11" /></button>
          <button class="icon-btn mini" title="Renomear" aria-label="Renomear" @click.stop="renameSnippet(s)"><Icon name="edit" :size="11" /></button>
          <button class="icon-btn mini" title="Excluir" aria-label="Excluir" @click.stop="deleteSnippet(s)"><Icon name="trash" :size="11" /></button>
        </li>
      </ul>
      <p v-if="!snippets.length" class="faint pad">Nenhum snippet. Selecione SQL no editor e clique em Novo. São arquivos .sql na pasta de configuração.</p>
    </div>

    <div v-else class="body">
      <label class="check"><input type="checkbox" :checked="tab.useParams !== false" @change="setUseParams(($event.target as HTMLInputElement).checked)" /> Substituir parâmetros ao executar</label>
      <p class="faint hint">Use <code class="mono">:nome</code> no SQL. Números, NULL e textos já entre aspas são usados como estão; o resto vira texto entre aspas.</p>
      <label v-for="p in params" :key="p" class="field">
        <span class="mono">:{{ p }}</span>
        <input class="input mono" :value="tab.params?.[p] ?? ''" spellcheck="false" @input="setParam(p, ($event.target as HTMLInputElement).value)" />
      </label>
      <p v-if="!params.length" class="faint pad">Nenhum parâmetro no SQL desta aba.</p>
    </div>
  </aside>
</template>

<style scoped>
.qh {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-width: 0;
  background: var(--bg-elev);
  border-left: 1px solid var(--border);
}
header {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 6px;
  border-bottom: 1px solid var(--border);
}
nav {
  display: flex;
  gap: 1px;
}
nav button {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 26px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
}
nav button.active {
  background: var(--accent-soft);
  color: var(--accent);
}
.n {
  position: absolute;
  top: 0;
  right: 0;
  font-size: 9px;
  padding: 0 3px;
  border-radius: 6px;
  background: var(--accent);
  color: var(--accent-text);
}
.title {
  flex: 1;
  font-size: 12px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.body {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 12px;
}
.pad {
  margin: 0;
  padding: 4px 0;
}
.hint {
  margin: 0;
  font-size: 11px;
}
.gen,
.row {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}
.list {
  list-style: none;
  margin: 0;
  padding: 0;
}
.list li {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 24px;
  padding: 0 4px;
  border-radius: var(--radius-sm);
}
.list li:hover {
  background: var(--bg-hover);
}
.clickable {
  cursor: pointer;
}
.ico {
  color: var(--text-faint);
  flex: none;
}
.nm {
  flex: 1;
  min-width: 0;
}
.ty {
  max-width: 45%;
  font-size: 11px;
}
.mini {
  width: 20px;
  height: 20px;
  flex: none;
}
.search {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 26px;
  padding: 0 8px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius);
  background: var(--bg-input);
  color: var(--text-faint);
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
.trail {
  font-size: 11px;
  color: var(--text-faint);
}
.link {
  border: none;
  background: none;
  padding: 0;
  color: var(--accent);
  cursor: pointer;
  font-size: 11px;
}
.topic h4 {
  margin: 4px 0;
}
.topic pre {
  white-space: pre-wrap;
  margin: 0;
  font-family: var(--font-ui);
  line-height: 1.45;
}
.topic pre.ex {
  font-family: var(--font-mono);
  padding: 6px;
  background: var(--bg-sunken);
  border-radius: var(--radius-sm);
}
.ex-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 8px;
}
h5 {
  margin: 6px 0 0;
  font-size: 11px;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.kw {
  display: flex;
  flex-wrap: wrap;
  gap: 3px;
}
.chip {
  padding: 1px 5px;
  border: 1px solid var(--border);
  border-radius: 3px;
  background: var(--bg);
  font-size: 10.5px;
  cursor: pointer;
}
.err {
  color: var(--danger);
}
</style>

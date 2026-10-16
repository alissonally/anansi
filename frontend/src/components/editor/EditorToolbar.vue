<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import ContextMenu, { type MenuItem } from '../common/ContextMenu.vue'
import { useTabsStore } from '../../stores/tabs'
import { environmentLabels, useConnectionsStore } from '../../stores/connections'
import { useMetadataStore } from '../../stores/metadata'
import { useSettingsStore } from '../../stores/settings'
import { useUiStore } from '../../stores/ui'
import Icon from '../common/Icon.vue'
import { backend } from '../../api/backend'

const emit = defineEmits<{ run: [mode: 'statement' | 'script']; format: []; save: []; saveAs: []; saveQuery: []; open: []; explain: [prefix: string]; helpers: [] }>()
const tabs = useTabsStore()
const conns = useConnectionsStore()
const meta = useMetadataStore()
const settings = useSettingsStore()
const ui = useUiStore()

const tab = computed(() => tabs.active)
const profile = computed(() => (tab.value?.connectionId ? conns.byId.get(tab.value.connectionId) : undefined))
const connState = computed(() => (profile.value ? conns.state(profile.value.id) : null))
const running = computed(() => tab.value?.status !== 'idle')

const databases = computed(() => {
  const id = profile.value?.id
  if (!id) return []
  const list = meta.entry<{ name: string; system: boolean }[]>(id, 'dbs')?.data ?? []
  const filtered = settings.prefs.showSystemSchemas ? list : list.filter((d) => !d.system)
  const cur = tab.value?.database
  if (cur && !filtered.some((d) => d.name === cur)) return [{ name: cur, system: false }, ...filtered]
  return filtered
})

// Load the database list and warm the autocomplete cache when needed.
watch(
  () => [profile.value?.id, connState.value?.status, tab.value?.database] as const,
  ([id, status, db]) => {
    if (!id || status !== 'connected') return
    void meta.databases(id).catch(() => undefined)
    if (db) void backend.warmMetadata(id, db).catch(() => undefined)
  },
  { immediate: true },
)

async function onConnection(e: Event) {
  const t = tab.value
  if (!t) return
  const sel = e.target as HTMLSelectElement
  const value = sel.value || null
  if (value === '__new__') {
    sel.value = t.connectionId ?? ''
    ui.openConnectionDialog()
    return
  }
  const ok = await tabs.setConnection(t.id, value)
  if (!ok) sel.value = t.connectionId ?? ''
  else if (value && conns.state(value).status !== 'connected') await conns.connect(value)
}

function onDatabase(e: Event) {
  if (tab.value) tabs.setDatabase(tab.value.id, (e.target as HTMLSelectElement).value)
}

const explainMenu = ref<{ x: number; y: number } | null>(null)
const isMaria = computed(() => (connState.value?.server?.flavor ?? '').toLowerCase() === 'mariadb')
const explainItems = computed<MenuItem[]>(() => [
  { label: 'EXPLAIN', hint: 'plano estimado', action: () => emit('explain', 'EXPLAIN') },
  { label: 'EXPLAIN FORMAT=JSON', action: () => emit('explain', 'EXPLAIN FORMAT=JSON') },
  ...(isMaria.value
    ? [{ label: 'ANALYZE', hint: 'executa e mede (MariaDB)', action: () => emit('explain', 'ANALYZE') }]
    : [
        { label: 'EXPLAIN FORMAT=TREE', hint: 'MySQL 8', action: () => emit('explain', 'EXPLAIN FORMAT=TREE') },
        { label: 'EXPLAIN ANALYZE', hint: 'executa e mede (MySQL 8.0.18+)', action: () => emit('explain', 'EXPLAIN ANALYZE') },
      ]),
])
function showExplain(e: MouseEvent) {
  const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
  explainMenu.value = { x: r.left, y: r.bottom + 2 }
}

function toggleProfile() {
  const t = tab.value
  if (t) t.profile = !t.profile
}

async function tx(cmd: 'COMMIT' | 'ROLLBACK') {
  const t = tab.value
  if (t) await tabs.run(t.id, { sql: cmd, cursorOffset: -1, mode: 'script', baseLine: 1 })
}
</script>

<template>
  <div v-if="tab" class="toolbar" :class="{ prod: profile?.environment === 'production' }">
    <div class="group conn">
      <span class="status-dot" :class="connState?.status ?? 'none'" :title="connState?.status ?? 'sem conexão'" />
      <select class="select compact conn-select" :value="tab.connectionId ?? ''" aria-label="Conexão da aba" :disabled="running" @change="onConnection">
        <option value="">— Selecione a conexão —</option>
        <option v-for="p in conns.profiles" :key="p.id" :value="p.id">{{ p.name }} ({{ environmentLabels[p.environment] }})</option>
        <option value="__new__">+ Nova conexão…</option>
      </select>
      <span v-if="profile" class="badge env-badge" :class="`env-${profile.environment}`">{{ environmentLabels[profile.environment] }}</span>
      <span v-if="profile?.readOnly" class="badge ro" title="Somente leitura"><Icon name="lock" :size="10" /> leitura</span>
      <select
        v-if="profile" class="select compact db-select" :value="tab.database" aria-label="Database padrão da aba" :disabled="running || connState?.status !== 'connected'"
        @change="onDatabase"
      >
        <option value="">(nenhum database)</option>
        <option v-for="d in databases" :key="d.name" :value="d.name">{{ d.name }}</option>
      </select>
    </div>

    <div class="group">
      <button class="btn small primary" :disabled="running || !tab.connectionId" title="Executar instrução sob o cursor ou seleção (Ctrl+Enter)" @click="emit('run', 'statement')">
        <Icon name="play" :size="12" /> Executar
      </button>
      <button class="btn small" :disabled="running || !tab.connectionId" title="Executar script inteiro (Ctrl+Shift+Enter / F5)" @click="emit('run', 'script')">
        <Icon name="playAll" :size="12" /> Script
      </button>
      <button class="btn small" :disabled="tab.status !== 'running'" title="Cancelar execução (Ctrl+Alt+C)" @click="tabs.cancel(tab.id)">
        <Icon name="stop" :size="11" /> Cancelar
      </button>
      <span class="split">
        <button class="btn small" :disabled="running || !tab.connectionId" title="EXPLAIN da instrução sob o cursor (Ctrl+Shift+E)" @click="emit('explain', 'EXPLAIN')">EXPLAIN</button>
        <button class="btn small caret" :disabled="running || !tab.connectionId" title="Outras formas de EXPLAIN" aria-label="Outras formas de EXPLAIN" @click="showExplain"><Icon name="chevronDown" :size="11" /></button>
      </span>
      <button class="btn small" :class="{ on: tab.profile }" :aria-pressed="!!tab.profile" title="Coletar o perfil de execução (SHOW PROFILE) — resultados grandes não são paginados" @click="toggleProfile">
        <Icon name="clock" :size="12" /> Perfil
      </button>
    </div>

    <div v-if="tab.inTransaction" class="group txn" role="status">
      <span class="badge tx"><Icon name="lock" :size="11" /> Transação aberta</span>
      <button class="btn small" :disabled="running" @click="tx('COMMIT')"><Icon name="commit" :size="12" /> COMMIT</button>
      <button class="btn small" :disabled="running" @click="tx('ROLLBACK')"><Icon name="rollback" :size="12" /> ROLLBACK</button>
    </div>

    <span class="spacer" />

    <div class="group">
      <button class="icon-btn" title="Formatar SQL (Shift+Alt+F)" aria-label="Formatar SQL" @click="emit('format')"><Icon name="format" /></button>
      <button class="icon-btn" title="Abrir arquivo SQL (Ctrl+O)" aria-label="Abrir arquivo" @click="emit('open')"><Icon name="folder" /></button>
      <button class="icon-btn" title="Salvar arquivo (Ctrl+S)" aria-label="Salvar arquivo" @click="emit('save')"><Icon name="save" /></button>
      <button class="icon-btn" title="Salvar como consulta salva" aria-label="Salvar como consulta salva" @click="emit('saveQuery')"><Icon name="bookmark" /></button>
      <button class="icon-btn" :class="{ on: tabs.layout.helpersVisible }" title="Painel de ajuda: colunas, ajuda SQL, funções, snippets e parâmetros (Ctrl+Shift+H)" aria-label="Painel de ajuda" @click="emit('helpers')"><Icon name="info" /></button>
    </div>
    <ContextMenu v-if="explainMenu" :x="explainMenu.x" :y="explainMenu.y" :items="explainItems" @close="explainMenu = null" />
  </div>
</template>

<style scoped>
.toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: var(--toolbar-h);
  padding: 6px 8px;
  border-bottom: 1px solid var(--border);
  background: var(--bg);
  flex-wrap: wrap;
  flex: none;
}
.toolbar.prod {
  box-shadow: inset 0 2px 0 var(--env-production);
  background: linear-gradient(var(--danger-soft), transparent 60%), var(--bg);
}
.group {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
.conn {
  flex: 0 1 auto;
}
.conn-select {
  max-width: 260px;
}
.db-select {
  max-width: 180px;
}
.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--text-faint);
  flex: none;
}
.status-dot.connected {
  background: var(--success);
}
.status-dot.connecting {
  background: var(--warning);
}
.status-dot.error {
  background: var(--danger);
}
.spacer {
  flex: 1;
}
.split {
  display: inline-flex;
}
.split .btn:first-child {
  border-top-right-radius: 0;
  border-bottom-right-radius: 0;
}
.split .caret {
  padding: 0 4px;
  border-left: none;
  border-top-left-radius: 0;
  border-bottom-left-radius: 0;
}
.btn.on,
.icon-btn.on {
  background: var(--accent-soft);
  border-color: var(--accent);
  color: var(--accent);
}
.ro {
  background: var(--warning-soft);
  color: var(--warning);
}
.tx {
  background: var(--warning-soft);
  color: var(--warning);
}
</style>

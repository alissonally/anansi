<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useTabsStore, type Tab } from '../../stores/tabs'
import { useUiStore } from '../../stores/ui'
import { backend } from '../../api/backend'
import Icon from '../common/Icon.vue'
import ResultGrid from './ResultGrid.vue'
import QueryResultEditor from './QueryResultEditor.vue'
import type { EditableInfo } from '../../api/types'
import { formatDuration, formatNumber, plural } from '../../utils/text'

const props = defineProps<{ tab: Tab }>()
const tabs = useTabsStore()
const ui = useUiStore()

const view = ref<'result' | 'messages'>('result')
const now = ref(performance.now())
let timer: number | undefined

watch(
  () => props.tab.status,
  (s) => {
    window.clearInterval(timer)
    if (s !== 'idle') {
      timer = window.setInterval(() => (now.value = performance.now()), 100)
    }
  },
  { immediate: true },
)
onBeforeUnmount(() => window.clearInterval(timer))

watch(() => props.tab.results, () => (view.value = 'result'))

const elapsed = computed(() => (props.tab.startedAt === null ? 0 : now.value - props.tab.startedAt))
const current = computed(() => props.tab.results[props.tab.activeResult] ?? null)

// ---- editable results ----
const editable = ref(new Map<object, EditableInfo | null>())
const editing = ref(false)
const showWarnings = ref(false)
watch(current, async (r) => {
  editing.value = false
  showWarnings.value = false
  if (!r || !r.hasRows || r.profile || r.error || !props.tab.connectionId || editable.value.has(r)) return
  editable.value.set(r, null)
  try {
    const info = await backend.editableResult(props.tab.connectionId, r.sql, props.tab.database, r.columns.map((c) => c.name))
    editable.value = new Map(editable.value).set(r, info)
  } catch {
    // not editable
  }
}, { immediate: true })
const editInfo = computed(() => (current.value ? (editable.value.get(current.value) ?? null) : null))

function selectResult(i: number) {
  const t = tabs.find(props.tab.id)
  if (t) t.activeResult = i
  view.value = 'result'
}

function summary(i: number): string {
  const r = props.tab.results[i]
  if (!r) return ''
  if (r.error) return 'erro'
  if (r.profile) return 'perfil'
  if (r.hasRows) return `${formatNumber(r.rows.length)}${r.hasMore ? '+' : ''}`
  if (r.rowsAffected !== undefined) return plural(r.rowsAffected, 'linha', 'linhas')
  return 'ok'
}

function shortSql(sql: string): string {
  const s = sql.replace(/\s+/g, ' ').trim()
  return s.length > 40 ? s.slice(0, 40) + '…' : s
}

const tableHint = computed(() => {
  const m = current.value?.sql.match(/\bfrom\s+((?:`[^`]+`|[\w$]+)(?:\.(?:`[^`]+`|[\w$]+))?)/i)
  return m?.[1] ?? ''
})

function exportResult() {
  const r = current.value
  if (!r) return
  const info = editInfo.value
  const keys = new Set((info?.key.columns ?? []).map((k) => k.toLowerCase()))
  ui.exportDialog = {
    title: props.tab.title,
    columns: r.columns.map((c, i) => {
      const src = info?.columns?.[i] ?? ''
      const ci = src ? info?.info?.find((x) => x.name === src) : undefined
      return { name: c.name, type: c.type, numeric: c.numeric, binary: c.binary, key: !!src && keys.has(src.toLowerCase()), autoIncrement: /auto_increment/i.test(ci?.extra ?? '') }
    }),
    rows: () => r.rows,
    table: info ? `${info.database}.${info.table}` : tableHint.value.replace(/`/g, ''),
    query: r.sql,
    full: props.tab.connectionId && r.kind === 'read' ? { kind: 'query', connectionId: props.tab.connectionId, database: props.tab.database, sql: r.sql } : undefined,
  }
}

async function closeResult() {
  await backend.closeResult(props.tab.id).catch(() => undefined)
  const t = tabs.find(props.tab.id)
  if (t) {
    t.results = []
    t.notice = null
    t.error = null
  }
}

const statusText = computed(() => {
  const t = props.tab
  if (t.status === 'running') return `Executando… ${formatDuration(elapsed.value)}`
  if (t.status === 'cancelling') return 'Cancelando…'
  if (t.cancelled) return 'Execução cancelada'
  return ''
})
</script>

<template>
  <section class="results" aria-label="Resultados">
    <header class="bar">
      <div class="chips" role="tablist">
        <template v-if="tab.results.length > 1 || (tab.results.length && view === 'messages')">
          <button
            v-for="(r, i) in tab.results" :key="i" role="tab" class="chip" :aria-selected="view === 'result' && tab.activeResult === i"
            :class="{ active: view === 'result' && tab.activeResult === i, error: !!r.error }" :title="r.sql"
            @click="selectResult(i)"
          >
            <span class="idx">{{ r.profile ? '⏱' : i + 1 }}</span>
            <span class="ellipsis">{{ shortSql(r.sql) }}</span>
            <span class="count">{{ summary(i) }}</span>
            <span v-if="r.warnings?.length" class="warnmark" :title="`${r.warnings.length} aviso(s)`">⚠{{ r.warnings.length }}</span>
          </button>
        </template>
        <button v-if="tab.results.length" role="tab" class="chip" :class="{ active: view === 'messages' }" @click="view = 'messages'">
          <Icon name="terminal" :size="12" /> Mensagens
        </button>
      </div>
      <div class="stats">
        <template v-if="tab.status === 'idle' && current && view === 'result'">
          <span v-if="current.hasRows">
            {{ plural(current.rows.length, 'linha', 'linhas') }}<template v-if="current.hasMore"> (há mais)</template>
          </span>
          <span v-else-if="current.rowsAffected !== undefined">{{ plural(current.rowsAffected, 'linha afetada', 'linhas afetadas') }}</span>
          <span>{{ formatDuration(current.durationMs) }}</span>
        </template>
        <span v-if="statusText" class="status-text">{{ statusText }}</span>
      </div>
      <div class="actions">
        <button v-if="editInfo && !editing && tab.status === 'idle'" class="btn small" :title="`Editar as linhas de ${editInfo.table} pela chave`" @click="editing = true">
          <Icon name="edit" :size="11" /> Editar
        </button>
        <button v-if="current?.hasMore" class="btn small" :disabled="current.loadingMore || tab.status !== 'idle'" @click="tabs.fetchMore(tab.id, tab.activeResult)">
          <span v-if="current.loadingMore" class="spinner" /> Carregar mais
        </button>
        <button class="icon-btn" title="Exportar / copiar como… (CSV, JSON, SQL, Markdown…)" aria-label="Exportar" :disabled="!current?.hasRows || tab.status !== 'idle'" @click="exportResult">
          <Icon name="download" :size="15" />
        </button>
        <button class="icon-btn" title="Limpar resultado" aria-label="Limpar resultado" :disabled="!tab.results.length || tab.status !== 'idle'" @click="closeResult">
          <Icon name="x" :size="15" />
        </button>
      </div>
    </header>

    <div v-if="tab.notice || tab.cancelled || current?.limitReached || current?.warnings?.length" class="banners">
      <div v-if="tab.notice" class="banner warn"><Icon name="info" :size="14" /> {{ tab.notice }}</div>
      <div v-if="tab.cancelled && tab.status === 'idle'" class="banner warn"><Icon name="stop" :size="12" /> A execução foi cancelada. Instruções restantes não foram executadas.</div>
      <div v-if="current?.limitReached" class="banner warn"><Icon name="alert" :size="14" /> Limite de 200.000 linhas carregadas atingido. Refine a consulta com WHERE/LIMIT.</div>
      <div v-if="current?.warnings?.length && view === 'result'" class="banner warn wlist">
        <Icon name="alert" :size="14" />
        <div class="selectable">
          <div v-for="(w, i) in (showWarnings ? current.warnings : current.warnings.slice(0, 2))" :key="i">{{ w.level }} {{ w.code }}: {{ w.message }}</div>
          <button v-if="current.warnings.length > 2" class="linkbtn" @click="showWarnings = !showWarnings">{{ showWarnings ? 'menos' : `mais ${current.warnings.length - 2}` }}</button>
        </div>
      </div>
    </div>

    <div class="body">
      <div v-if="tab.status !== 'idle'" class="center">
        <div class="running">
          <span class="spinner big" />
          <div>
            <strong>{{ tab.status === 'cancelling' ? 'Cancelando a consulta…' : 'Executando consulta…' }}</strong>
            <div class="muted mono">{{ formatDuration(elapsed) }}</div>
          </div>
          <button v-if="tab.status === 'running'" class="btn" @click="tabs.cancel(tab.id)"><Icon name="stop" :size="12" /> Cancelar</button>
        </div>
      </div>

      <div v-else-if="tab.error" class="center">
        <div class="error-box" role="alert">
          <Icon name="alert" :size="18" />
          <div class="selectable">{{ tab.error }}</div>
        </div>
      </div>

      <div v-else-if="!tab.results.length" class="center empty">
        <Icon name="terminal" :size="28" />
        <p>Os resultados aparecem aqui.</p>
        <p class="faint">
          <kbd>Ctrl</kbd>+<kbd>Enter</kbd> executa a instrução sob o cursor · <kbd>Ctrl</kbd>+<kbd>Shift</kbd>+<kbd>Enter</kbd> executa o script
        </p>
      </div>

      <div v-else-if="view === 'messages'" class="messages selectable">
        <div v-for="(r, i) in tab.results" :key="i" class="msg" :class="{ error: !!r.error }">
          <span class="when mono">{{ i + 1 }}.</span>
          <div class="text">
            <code class="mono">{{ r.sql }}</code>
            <div v-if="r.error" class="err">Erro {{ r.error.code ?? '' }}{{ r.error.sqlState ? ` (${r.error.sqlState})` : '' }}: {{ r.error.message }}</div>
            <div v-else class="ok">
              <template v-if="r.hasRows">{{ plural(r.rows.length, 'linha retornada', 'linhas retornadas') }}{{ r.hasMore ? ' (parcial)' : '' }}</template>
              <template v-else-if="r.rowsAffected !== undefined">{{ plural(r.rowsAffected, 'linha afetada', 'linhas afetadas') }}</template>
              <template v-else>Executada</template>
              <template v-if="r.lastInsertId"> · último ID {{ r.lastInsertId }}</template>
              · {{ formatDuration(r.durationMs) }}
            </div>
            <div v-for="(w, wi) in r.warnings ?? []" :key="wi" class="warnline">{{ w.level }} {{ w.code }}: {{ w.message }}</div>
          </div>
        </div>
        <div v-if="tab.cancelled" class="msg error"><span class="when" /> <div class="text">Execução cancelada pelo usuário.</div></div>
      </div>

      <template v-else-if="current">
        <div v-if="current.error" class="center">
          <div class="error-box" role="alert">
            <Icon name="alert" :size="18" />
            <div class="selectable">
              <div class="err-title">
                {{ current.error.category === 'cancelled' ? 'Consulta interrompida' : 'Erro' }}
                <span v-if="current.error.code" class="mono">{{ current.error.code }}</span>
                <span v-if="current.error.sqlState" class="mono faint">SQLSTATE {{ current.error.sqlState }}</span>
              </div>
              <div>{{ current.error.message }}</div>
              <code class="mono faint stmt">{{ current.sql }}</code>
            </div>
          </div>
        </div>
        <QueryResultEditor v-else-if="editing && editInfo" :key="'e' + tab.id + ':' + tab.activeResult" :tab="tab" :result="current" :info="editInfo" @close="editing = false" />
        <ResultGrid v-else-if="current.hasRows && current.rows.length" :key="tab.id + ':' + tab.activeResult" :result="current" :table-hint="tableHint" @load-more="tabs.fetchMore(tab.id, tab.activeResult)" />
        <div v-else-if="current.hasRows" class="center empty">
          <p>Nenhuma linha retornada.</p>
          <p class="faint mono">{{ current.columns.map((c) => c.name).join(', ') }}</p>
        </div>
        <div v-else class="center">
          <div class="ok-box">
            <Icon name="check" :size="18" />
            <div>
              <strong>Instrução executada</strong>
              <div v-if="current.rowsAffected !== undefined">{{ plural(current.rowsAffected, 'linha afetada', 'linhas afetadas') }}</div>
              <div v-if="current.lastInsertId" class="muted">Último ID inserido: {{ current.lastInsertId }}</div>
              <div class="muted">{{ formatDuration(current.durationMs) }}</div>
            </div>
          </div>
        </div>
      </template>
    </div>
  </section>
</template>

<style scoped>
.results {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  background: var(--bg);
}
.bar {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 34px;
  padding: 0 6px 0 8px;
  border-bottom: 1px solid var(--border);
  background: var(--bg-elev);
  flex: none;
}
.chips {
  display: flex;
  gap: 4px;
  min-width: 0;
  overflow-x: auto;
  flex: 1;
}
.chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 260px;
  height: 24px;
  padding: 0 8px;
  border: 1px solid var(--border);
  border-radius: 12px;
  background: transparent;
  color: var(--text-muted);
  font-size: 12px;
  cursor: pointer;
  flex: none;
}
.chip.active {
  background: var(--accent-soft);
  border-color: var(--accent);
  color: var(--text);
}
.chip.error {
  border-color: var(--danger);
}
.chip .idx {
  font-weight: 700;
}
.chip .count {
  color: var(--text-faint);
}
.stats {
  display: flex;
  gap: 12px;
  color: var(--text-muted);
  font-size: 12px;
  white-space: nowrap;
}
.status-text {
  color: var(--accent);
}
.actions {
  display: flex;
  align-items: center;
  gap: 2px;
}
.banners {
  flex: none;
}
.banner {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  font-size: 12px;
  border-bottom: 1px solid var(--border);
}
.banner.warn {
  background: var(--warning-soft);
  color: var(--text);
}
.warnmark {
  color: var(--warning);
  font-size: 11px;
}
.wlist {
  align-items: flex-start;
}
.linkbtn {
  border: none;
  background: none;
  padding: 0;
  color: var(--accent);
  cursor: pointer;
  font-size: 12px;
}
.warnline {
  color: var(--warning);
}
.banner.warn > .icon {
  color: var(--warning);
}
.body {
  position: relative;
  flex: 1;
  min-height: 0;
  overflow: hidden;
}
.center {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  padding: 16px;
  text-align: center;
  overflow: auto;
}
.empty {
  color: var(--text-muted);
  gap: 4px;
}
.empty p {
  margin: 0;
}
kbd {
  padding: 0 4px;
  border: 1px solid var(--border-strong);
  border-radius: 3px;
  font-size: 11px;
  font-family: var(--font-mono);
}
.running {
  display: flex;
  align-items: center;
  gap: 14px;
  text-align: left;
}
.spinner.big {
  width: 22px;
  height: 22px;
  border-width: 3px;
}
.error-box,
.ok-box {
  display: flex;
  gap: 10px;
  max-width: 760px;
  padding: 12px 14px;
  border-radius: var(--radius);
  text-align: left;
  line-height: 1.5;
}
.error-box {
  background: var(--danger-soft);
  border: 1px solid var(--danger);
}
.error-box > .icon {
  color: var(--danger);
  margin-top: 2px;
}
.err-title {
  display: flex;
  gap: 8px;
  font-weight: 600;
}
.stmt {
  display: block;
  margin-top: 6px;
  font-size: 11px;
  white-space: pre-wrap;
  max-height: 120px;
  overflow: auto;
}
.ok-box {
  background: var(--success-soft);
  border: 1px solid var(--success);
}
.ok-box > .icon {
  color: var(--success);
  margin-top: 2px;
}
.messages {
  height: 100%;
  overflow: auto;
  padding: 8px 12px;
  font-size: 12px;
}
.msg {
  display: flex;
  gap: 8px;
  padding: 6px 0;
  border-bottom: 1px solid var(--border);
}
.msg .when {
  color: var(--text-faint);
  min-width: 24px;
}
.msg .text {
  min-width: 0;
}
.msg code {
  display: block;
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--text);
}
.msg .ok {
  color: var(--success);
}
.msg .err,
.msg.error .text {
  color: var(--danger);
}
</style>

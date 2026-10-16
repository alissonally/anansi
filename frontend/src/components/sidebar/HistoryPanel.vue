<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { backend } from '../../api/backend'
import { toAppError } from '../../api/errors'
import type { HistoryEntry } from '../../api/types'
import { useConnectionsStore } from '../../stores/connections'
import { useSettingsStore } from '../../stores/settings'
import { useTabsStore } from '../../stores/tabs'
import { useUiStore } from '../../stores/ui'
import Icon from '../common/Icon.vue'
import { formatDuration } from '../../utils/text'

const emit = defineEmits<{ insert: [text: string] }>()
const conns = useConnectionsStore()
const settings = useSettingsStore()
const tabs = useTabsStore()
const ui = useUiStore()

const entries = ref<HistoryEntry[]>([])
const search = ref('')
const connectionId = ref('')
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    entries.value = await backend.listHistory(connectionId.value, search.value, 300)
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  } finally {
    loading.value = false
  }
}

let timer: number | undefined
watch([search, connectionId], () => {
  window.clearTimeout(timer)
  timer = window.setTimeout(load, 200)
})
// Refresh after executions finish.
watch(() => tabs.tabs.map((t) => t.status).join(), (now, before) => {
  if (before && before.includes('running') && !now.includes('running')) window.setTimeout(load, 700)
})
onMounted(load)

function openInTab(e: HistoryEntry) {
  const valid = conns.byId.has(e.connectionId)
  tabs.open({ title: 'Histórico', connectionId: valid ? e.connectionId : null, database: e.database, sql: e.sql })
}

async function clear() {
  const target = connectionId.value ? conns.byId.get(connectionId.value)?.name : null
  const ok = await ui.confirm({ title: 'Limpar histórico', message: target ? `Remover o histórico da conexão "${target}"?` : 'Remover todo o histórico?', confirmLabel: 'Limpar', danger: true })
  if (!ok) return
  await backend.clearHistory(connectionId.value).catch((er) => ui.toast('error', toAppError(er).message))
  await load()
}

function when(iso: string) {
  const d = new Date(iso)
  return d.toLocaleString('pt-BR', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })
}
</script>

<template>
  <div class="panel">
    <div class="filters">
      <input v-model="search" class="input" placeholder="Buscar no histórico…" aria-label="Buscar no histórico" />
      <div class="row">
        <select v-model="connectionId" class="select compact" aria-label="Filtrar por conexão">
          <option value="">Todas as conexões</option>
          <option v-for="p in conns.profiles" :key="p.id" :value="p.id">{{ p.name }}</option>
        </select>
        <button class="icon-btn" title="Atualizar" aria-label="Atualizar" @click="load"><Icon name="refresh" :size="14" /></button>
        <button class="icon-btn" title="Limpar histórico" aria-label="Limpar histórico" @click="clear"><Icon name="trash" :size="14" /></button>
      </div>
    </div>
    <p v-if="!settings.prefs.historyEnabled" class="note">O histórico está desativado nas preferências.</p>
    <div class="list">
      <div v-if="!entries.length && !loading" class="empty">Nenhuma consulta no histórico.</div>
      <div v-for="e in entries" :key="e.id" class="entry" :class="{ failed: !e.success }" tabindex="0" :title="e.error ?? e.sql" @dblclick="openInTab(e)" @keydown.enter="openInTab(e)">
        <code class="sql mono">{{ e.sql }}</code>
        <div class="meta">
          <Icon :name="e.success ? 'check' : 'alert'" :size="11" />
          <span class="ellipsis">{{ e.connectionName || 'conexão removida' }}{{ e.database ? ' · ' + e.database : '' }}</span>
          <span class="time">{{ when(e.executedAt) }} · {{ formatDuration(e.durationMs) }}</span>
        </div>
        <div class="actions">
          <button class="icon-btn" title="Inserir no editor" aria-label="Inserir no editor" @click="emit('insert', e.sql)"><Icon name="edit" :size="12" /></button>
          <button class="icon-btn" title="Abrir em nova aba" aria-label="Abrir em nova aba" @click="openInTab(e)"><Icon name="plus" :size="12" /></button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}
.filters {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px;
}
.row {
  display: flex;
  gap: 4px;
  align-items: center;
}
.row .select {
  flex: 1;
  min-width: 0;
}
.note {
  margin: 0 8px 8px;
  font-size: 12px;
  color: var(--warning);
}
.list {
  flex: 1;
  overflow: auto;
}
.empty {
  padding: 24px 12px;
  color: var(--text-muted);
  text-align: center;
}
.entry {
  position: relative;
  padding: 6px 10px;
  border-bottom: 1px solid var(--border);
  cursor: default;
}
.entry:hover,
.entry:focus-visible {
  background: var(--bg-hover);
  outline: none;
}
.sql {
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
  font-size: 11.5px;
  white-space: pre-wrap;
  word-break: break-word;
}
.meta {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: 3px;
  font-size: 11px;
  color: var(--text-faint);
}
.meta .icon {
  color: var(--success);
}
.failed .meta .icon {
  color: var(--danger);
}
.time {
  margin-left: auto;
  white-space: nowrap;
}
.actions {
  position: absolute;
  top: 4px;
  right: 4px;
  display: none;
  background: var(--bg-elev);
  border-radius: var(--radius-sm);
}
.entry:hover .actions {
  display: flex;
}
</style>

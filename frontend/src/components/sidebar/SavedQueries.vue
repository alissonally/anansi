<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { backend } from '../../api/backend'
import { toAppError } from '../../api/errors'
import type { SavedQuery } from '../../api/types'
import { useConnectionsStore } from '../../stores/connections'
import { useTabsStore } from '../../stores/tabs'
import { useUiStore } from '../../stores/ui'
import Icon from '../common/Icon.vue'

const conns = useConnectionsStore()
const tabs = useTabsStore()
const ui = useUiStore()
const queries = ref<SavedQuery[]>([])

async function load() {
  try {
    queries.value = await backend.listSavedQueries()
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}
onMounted(load)
defineExpose({ load })

function open(q: SavedQuery) {
  const existing = tabs.tabs.find((t) => t.savedQueryId === q.id)
  if (existing) {
    tabs.activate(existing.id)
    return
  }
  const conn = q.connectionId && conns.byId.has(q.connectionId) ? q.connectionId : undefined
  tabs.open({ title: q.name, sql: q.sql, savedQueryId: q.id, savedContent: q.sql, ...(conn ? { connectionId: conn } : {}) })
}

async function remove(q: SavedQuery) {
  if (!(await ui.confirm({ title: 'Excluir consulta salva', message: `Excluir "${q.name}"?`, confirmLabel: 'Excluir', danger: true }))) return
  try {
    await backend.deleteSavedQuery(q.id)
    for (const t of tabs.tabs) if (t.savedQueryId === q.id) { t.savedQueryId = undefined; t.savedContent = null }
    await load()
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}
</script>

<template>
  <div class="panel">
    <div v-if="!queries.length" class="empty">
      <Icon name="bookmark" :size="24" />
      <p>Nenhuma consulta salva.</p>
      <p class="faint">Use o botão <Icon name="bookmark" :size="11" style="display: inline" /> da barra do editor para salvar a aba atual.</p>
    </div>
    <div v-for="q in queries" :key="q.id" class="item" tabindex="0" @dblclick="open(q)" @keydown.enter="open(q)">
      <Icon name="bookmark" :size="13" />
      <div class="text">
        <div class="name ellipsis">{{ q.name }}</div>
        <div class="sub ellipsis mono">{{ q.sql.replace(/\s+/g, ' ').slice(0, 80) }}</div>
      </div>
      <button class="icon-btn" title="Abrir" aria-label="Abrir" @click="open(q)"><Icon name="plus" :size="13" /></button>
      <button class="icon-btn" title="Excluir" aria-label="Excluir" @click="remove(q)"><Icon name="trash" :size="13" /></button>
    </div>
  </div>
</template>

<style scoped>
.panel {
  height: 100%;
  overflow: auto;
  padding: 4px 0;
}
.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 28px 16px;
  color: var(--text-muted);
  text-align: center;
}
.empty p {
  margin: 0;
}
.item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px 6px 12px;
  cursor: default;
}
.item:hover,
.item:focus-visible {
  background: var(--bg-hover);
  outline: none;
}
.text {
  flex: 1;
  min-width: 0;
}
.sub {
  font-size: 11px;
  color: var(--text-faint);
}
.item .icon-btn {
  visibility: hidden;
}
.item:hover .icon-btn {
  visibility: visible;
}
</style>

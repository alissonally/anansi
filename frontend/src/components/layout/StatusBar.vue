<script setup lang="ts">
import { computed } from 'vue'
import { environmentLabels, useConnectionsStore } from '../../stores/connections'
import { useSettingsStore } from '../../stores/settings'
import { useTabsStore } from '../../stores/tabs'
import { useTasksStore } from '../../stores/tasks'
import { useUiStore } from '../../stores/ui'
import Icon from '../common/Icon.vue'

defineProps<{ line: number; column: number }>()
const emit = defineEmits<{ toggleLog: [] }>()
const conns = useConnectionsStore()
const settings = useSettingsStore()
const tabs = useTabsStore()
const tasks = useTasksStore()
const ui = useUiStore()

const tab = computed(() => tabs.active)
const profile = computed(() => (tab.value?.connectionId ? conns.byId.get(tab.value.connectionId) : undefined))
const state = computed(() => (profile.value ? conns.state(profile.value.id) : null))
const openCount = computed(() => conns.profiles.filter((p) => conns.state(p.id).status === 'connected').length)

function toggleTheme() {
  const next = settings.effectiveTheme === 'dark' ? 'light' : 'dark'
  void settings.save({ ...settings.prefs, theme: next })
}
</script>

<template>
  <footer class="status" :class="{ prod: profile?.environment === 'production' }" role="contentinfo">
    <template v-if="profile">
      <span class="item strong"><span class="dot" :class="state?.status" /> {{ profile.name }}</span>
      <span class="badge env-badge" :class="`env-${profile.environment}`">{{ environmentLabels[profile.environment] }}</span>
      <span v-if="state?.status === 'connected' && state.server" class="item">{{ state.server.flavor }} {{ state.server.version }}</span>
      <span v-if="state?.status === 'connected' && state.server" class="item" :class="state.server.tlsActive ? 'ok' : 'warn'" :title="state.server.tlsCipher || 'Conexão sem criptografia'">
        <Icon name="lock" :size="11" /> {{ state.server.tlsActive ? 'TLS' : 'sem TLS' }}
      </span>
      <span v-if="state?.status === 'connecting'" class="item">conectando…</span>
      <span v-if="state?.status === 'disconnected' || state?.status === 'error'" class="item warn">desconectado</span>
      <span v-if="tab?.database" class="item"><Icon name="database" :size="11" /> {{ tab.database }}</span>
    </template>
    <span v-else class="item faint">Sem conexão nesta aba</span>
    <span v-if="tab?.inTransaction" class="item warn"><Icon name="lock" :size="11" /> transação aberta</span>
    <span class="spacer" />
    <span class="item faint">{{ openCount }} {{ openCount === 1 ? 'conexão aberta' : 'conexões abertas' }}</span>
    <span v-if="tab?.kind === 'query'" class="item faint">Ln {{ line }}, Col {{ column }}</span>
    <button class="sb-btn" :class="{ on: tasks.panelOpen }" title="Tarefas em segundo plano" aria-label="Tarefas" @click="tasks.panelOpen = !tasks.panelOpen; tasks.start()">
      <span v-if="tasks.running" class="spinner tiny" /><Icon v-else name="clock" :size="12" /> Tarefas<template v-if="tasks.running"> ({{ tasks.running }})</template>
    </button>
    <button class="sb-btn" :class="{ on: tabs.layout.logVisible }" title="Log de SQL (Ctrl+Alt+L)" aria-label="Log de SQL" @click="emit('toggleLog')"><Icon name="terminal" :size="12" /> Log</button>
    <button class="sb-btn" :title="settings.effectiveTheme === 'dark' ? 'Tema claro' : 'Tema escuro'" aria-label="Alternar tema" @click="toggleTheme">
      <Icon :name="settings.effectiveTheme === 'dark' ? 'sun' : 'moon'" :size="12" />
    </button>
    <button class="sb-btn" title="Preferências (Ctrl+,)" aria-label="Preferências" @click="ui.preferencesOpen = true"><Icon name="settings" :size="12" /></button>
    <span class="item faint">v{{ settings.info?.version }}</span>
  </footer>
</template>

<style scoped>
.status {
  display: flex;
  align-items: center;
  gap: 12px;
  height: var(--status-h);
  padding: 0 8px;
  border-top: 1px solid var(--border);
  background: var(--bg-elev);
  font-size: 11.5px;
  color: var(--text-muted);
  white-space: nowrap;
  overflow: hidden;
  flex: none;
}
.status.prod {
  border-top: 2px solid var(--env-production);
}
.item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.strong {
  color: var(--text);
  font-weight: 600;
}
.ok {
  color: var(--success);
}
.warn {
  color: var(--warning);
}
.dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--text-faint);
}
.dot.connected {
  background: var(--success);
}
.dot.connecting {
  background: var(--warning);
}
.dot.error {
  background: var(--danger);
}
.spacer {
  flex: 1;
}
.sb-btn {
  display: inline-flex;
  align-items: center;
  height: 18px;
  padding: 0 4px;
  border: none;
  border-radius: 3px;
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
}
.sb-btn {
  gap: 4px;
  font-size: 11px;
}
.sb-btn:hover {
  background: var(--bg-hover);
  color: var(--text);
}
.spinner.tiny {
  width: 10px;
  height: 10px;
  border-width: 2px;
}
.sb-btn.on {
  color: var(--accent);
}
@media (max-width: 900px) {
  .status .faint:not(:last-child) {
    display: none;
  }
}
</style>

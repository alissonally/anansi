<script setup lang="ts">
import { computed } from 'vue'
import type { Tab } from '../../stores/tabs'
import { useConnectionsStore } from '../../stores/connections'
import HostView from './HostView.vue'
import DatabaseView from './DatabaseView.vue'
import TableView from './TableView.vue'
import RoutineEditor from './RoutineEditor.vue'
import TriggerEditor from './TriggerEditor.vue'
import EventEditor from './EventEditor.vue'
import UserManager from './UserManager.vue'
import Icon from '../common/Icon.vue'

const props = defineProps<{ tab: Tab }>()
const conns = useConnectionsStore()
const connected = computed(() => !!props.tab.connectionId && conns.state(props.tab.connectionId).status === 'connected')
</script>

<template>
  <div v-if="!connected" class="offline">
    <Icon name="unplug" :size="28" />
    <p>A conexão desta aba não está aberta.</p>
    <button class="btn primary" :disabled="!tab.connectionId" @click="tab.connectionId && conns.connect(tab.connectionId)"><Icon name="plug" :size="14" /> Conectar</button>
  </div>
  <HostView v-else-if="tab.kind === 'host'" :key="tab.id" :tab="tab" />
  <DatabaseView v-else-if="tab.kind === 'database'" :key="tab.id" :tab="tab" />
  <TableView v-else-if="tab.kind === 'table' || tab.kind === 'view'" :key="tab.id" :tab="tab" />
  <RoutineEditor v-else-if="tab.kind === 'routine'" :key="tab.id + (tab.objectName ?? '')" :tab="tab" />
  <TriggerEditor v-else-if="tab.kind === 'trigger'" :key="tab.id + (tab.objectName ?? '')" :tab="tab" />
  <EventEditor v-else-if="tab.kind === 'event'" :key="tab.id + (tab.objectName ?? '')" :tab="tab" />
  <UserManager v-else-if="tab.kind === 'users'" :key="tab.id" :tab="tab" />
</template>

<style scoped>
.offline {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  height: 100%;
  color: var(--text-muted);
}
.offline p {
  margin: 0;
}
</style>

<script setup lang="ts">
import { ref } from 'vue'
import ConnectionTree from './ConnectionTree.vue'
import HistoryPanel from './HistoryPanel.vue'
import SavedQueries from './SavedQueries.vue'
import Icon from '../common/Icon.vue'
import { useUiStore } from '../../stores/ui'

const emit = defineEmits<{ insert: [text: string] }>()
const ui = useUiStore()
const view = ref<'connections' | 'history' | 'saved'>('connections')
const saved = ref<InstanceType<typeof SavedQueries> | null>(null)
defineExpose({ reloadSaved: () => saved.value?.load(), show: (v: 'connections' | 'history' | 'saved') => (view.value = v) })
</script>

<template>
  <aside class="sidebar" aria-label="Barra lateral">
    <header class="head">
      <nav class="views" role="tablist">
        <button role="tab" :aria-selected="view === 'connections'" :class="{ active: view === 'connections' }" title="Conexões" @click="view = 'connections'"><Icon name="database" :size="15" /> Conexões</button>
        <button role="tab" :aria-selected="view === 'history'" :class="{ active: view === 'history' }" title="Histórico" @click="view = 'history'"><Icon name="history" :size="15" /></button>
        <button role="tab" :aria-selected="view === 'saved'" :class="{ active: view === 'saved' }" title="Consultas salvas" @click="view = 'saved'"><Icon name="bookmark" :size="15" /></button>
      </nav>
      <button class="icon-btn" title="Nova conexão" aria-label="Nova conexão" @click="ui.openConnectionDialog()"><Icon name="plus" /></button>
    </header>
    <div class="content">
      <ConnectionTree v-if="view === 'connections'" @insert="emit('insert', $event)" />
      <HistoryPanel v-else-if="view === 'history'" @insert="emit('insert', $event)" />
      <SavedQueries v-else ref="saved" />
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-width: 0;
  background: var(--bg-elev);
}
.head {
  display: flex;
  align-items: center;
  gap: 4px;
  height: var(--tabbar-h);
  padding: 0 4px 0 6px;
  border-bottom: 1px solid var(--border);
  flex: none;
}
.views {
  display: flex;
  gap: 2px;
  flex: 1;
}
.views button {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 26px;
  padding: 0 8px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  font-weight: 600;
}
.views button.active {
  background: var(--bg-active);
  color: var(--text);
}
.content {
  flex: 1;
  min-height: 0;
}
</style>

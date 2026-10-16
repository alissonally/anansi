<script setup lang="ts">
import { onMounted } from 'vue'
import { useTasksStore } from '../../stores/tasks'
import { backend } from '../../api/backend'
import Icon from '../common/Icon.vue'
import { formatBytes, formatCount } from '../../utils/format'
import { formatDuration } from '../../utils/text'
import type { Task } from '../../api/types'

const store = useTasksStore()
onMounted(() => void store.start())

function amount(t: Task, n: number) {
  return t.unit === 'bytes' ? formatBytes(n) : formatCount(n)
}
function pct(t: Task) {
  return t.total > 0 ? Math.min(100, (t.done / t.total) * 100) : null
}
function elapsed(t: Task) {
  const end = t.finishedAt && !t.finishedAt.startsWith('0001') ? new Date(t.finishedAt).getTime() : Date.now()
  return formatDuration(end - new Date(t.startedAt).getTime())
}
const units: Record<string, string> = { rows: 'linhas', bytes: '', statements: 'instruções', objects: 'objetos', tables: 'tabelas' }
</script>

<template>
  <section v-if="store.panelOpen" class="tasks" aria-label="Tarefas em segundo plano">
    <header>
      <strong>Tarefas</strong>
      <span class="spacer" />
      <button class="btn small ghost" :disabled="!store.tasks.some((t) => t.status !== 'running')" @click="store.clearFinished()">Limpar concluídas</button>
      <button class="icon-btn" aria-label="Fechar" @click="store.panelOpen = false"><Icon name="x" :size="12" /></button>
    </header>
    <ul>
      <li v-for="t in store.tasks" :key="t.id" :class="t.status">
        <div class="row">
          <span class="st">
            <span v-if="t.status === 'running'" class="spinner" />
            <Icon v-else :name="t.status === 'done' ? 'check' : t.status === 'error' ? 'alert' : 'stop'" :size="12" />
          </span>
          <span class="title ellipsis" :title="t.title">{{ t.title }}</span>
          <span class="faint small">{{ elapsed(t) }}</span>
          <button v-if="t.status === 'running'" class="icon-btn" title="Cancelar" aria-label="Cancelar" @click="store.cancel(t.id)"><Icon name="stop" :size="10" /></button>
          <button v-else-if="t.path" class="icon-btn" title="Abrir pasta" aria-label="Abrir pasta" @click="backend.openTaskFolder(t.id)"><Icon name="folder" :size="12" /></button>
          <button v-if="t.status !== 'running'" class="icon-btn" title="Remover" aria-label="Remover" @click="store.dismiss(t.id)"><Icon name="x" :size="11" /></button>
        </div>
        <div v-if="t.status === 'running'" class="bar" :class="{ indet: pct(t) === null }"><span :style="{ width: (pct(t) ?? 30) + '%' }" /></div>
        <div class="detail faint small ellipsis" :title="t.error || t.message || t.result">
          <template v-if="t.status === 'running'">{{ amount(t, t.done) }}<template v-if="t.total"> de {{ amount(t, t.total) }}</template> {{ units[t.unit] ?? '' }}<template v-if="t.message"> · {{ t.message }}</template></template>
          <template v-else-if="t.status === 'error'"><span class="err">{{ t.error }}</span></template>
          <template v-else>{{ t.result }}</template>
          <template v-if="t.warnings"> · <span class="warn">{{ t.warnings }} avisos</span></template>
        </div>
      </li>
      <li v-if="!store.tasks.length" class="faint small empty">Nenhuma tarefa.</li>
    </ul>
  </section>
</template>

<style scoped>
.tasks {
  position: fixed;
  left: 12px;
  bottom: 34px;
  z-index: 800;
  width: 380px;
  max-height: 50vh;
  display: flex;
  flex-direction: column;
  background: var(--bg-elev);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius);
  box-shadow: var(--shadow);
}
header {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 8px;
  border-bottom: 1px solid var(--border);
}
.spacer {
  flex: 1;
}
ul {
  list-style: none;
  margin: 0;
  padding: 4px 0;
  overflow: auto;
}
li {
  padding: 6px 10px;
  border-bottom: 1px solid var(--border);
}
li:last-child {
  border-bottom: none;
}
.row {
  display: flex;
  align-items: center;
  gap: 6px;
}
.st {
  width: 14px;
  display: inline-flex;
}
li.done .st {
  color: var(--success);
}
li.error .st {
  color: var(--danger);
}
.title {
  flex: 1;
  min-width: 0;
  font-weight: 500;
}
.small {
  font-size: 11px;
}
.bar {
  height: 4px;
  margin: 5px 0 3px;
  border-radius: 2px;
  background: var(--bg-active);
  overflow: hidden;
}
.bar span {
  display: block;
  height: 100%;
  background: var(--accent);
  transition: width 0.2s;
}
.bar.indet span {
  animation: slide 1.2s ease-in-out infinite;
}
@keyframes slide {
  from { margin-left: -30%; }
  to { margin-left: 100%; }
}
.err {
  color: var(--danger);
}
.warn {
  color: var(--warning);
}
.empty {
  border: none;
}
</style>

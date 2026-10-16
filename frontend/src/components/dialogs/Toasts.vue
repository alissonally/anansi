<script setup lang="ts">
import { useUiStore } from '../../stores/ui'
import Icon from '../common/Icon.vue'

const ui = useUiStore()
const icons = { info: 'info', success: 'check', warning: 'alert', error: 'alert' } as const
</script>

<template>
  <div class="toasts" role="status" aria-live="polite">
    <div v-for="t in ui.toasts" :key="t.id" class="toast" :class="t.kind">
      <Icon :name="icons[t.kind]" :size="16" />
      <span class="msg selectable">{{ t.message }}</span>
      <button class="icon-btn" aria-label="Fechar notificação" @click="ui.dismiss(t.id)"><Icon name="x" :size="14" /></button>
    </div>
  </div>
</template>

<style scoped>
.toasts {
  position: fixed;
  right: 16px;
  bottom: calc(var(--status-h) + 12px);
  z-index: 950;
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-width: min(440px, calc(100vw - 32px));
}
.toast {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 10px 8px 10px 12px;
  background: var(--bg-elev);
  border: 1px solid var(--border-strong);
  border-left-width: 3px;
  border-radius: var(--radius);
  box-shadow: var(--shadow);
}
.toast > .icon {
  margin-top: 1px;
}
.msg {
  flex: 1;
  line-height: 1.45;
  word-break: break-word;
}
.info { border-left-color: var(--accent); }
.success { border-left-color: var(--success); }
.success > :first-child { color: var(--success); }
.warning { border-left-color: var(--warning); }
.warning > :first-child { color: var(--warning); }
.error { border-left-color: var(--danger); }
.error > :first-child { color: var(--danger); }
</style>

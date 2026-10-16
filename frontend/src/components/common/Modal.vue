<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import Icon from './Icon.vue'

const props = withDefaults(defineProps<{ title: string; width?: number; closeOnBackdrop?: boolean }>(), { width: 480, closeOnBackdrop: true })
const emit = defineEmits<{ close: [] }>()
const panel = ref<HTMLElement | null>(null)
let previouslyFocused: Element | null = null

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.stopPropagation()
    emit('close')
    return
  }
  if (e.key === 'Tab' && panel.value) {
    // Keep focus inside the dialog.
    const focusables = Array.from(panel.value.querySelectorAll<HTMLElement>('button, input, select, textarea, [tabindex]:not([tabindex="-1"])')).filter((x) => !x.hasAttribute('disabled'))
    if (!focusables.length) return
    const first = focusables[0]!
    const last = focusables[focusables.length - 1]!
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault()
      first.focus()
    }
  }
}

onMounted(async () => {
  previouslyFocused = document.activeElement
  await nextTick()
  const auto = panel.value?.querySelector<HTMLElement>('[autofocus], input:not([type=checkbox]), select, textarea, .btn.primary, .btn.danger')
  auto?.focus()
})
onBeforeUnmount(() => {
  if (previouslyFocused instanceof HTMLElement) previouslyFocused.focus()
})

function backdrop() {
  if (props.closeOnBackdrop) emit('close')
}
</script>

<template>
  <Teleport to="body">
    <div class="backdrop" @mousedown.self="backdrop" @keydown="onKey">
      <div ref="panel" class="panel" role="dialog" aria-modal="true" :aria-label="title" :style="{ width: `min(${width}px, calc(100vw - 32px))` }">
        <header>
          <h2>{{ title }}</h2>
          <button class="icon-btn" aria-label="Fechar" @click="emit('close')"><Icon name="x" /></button>
        </header>
        <div class="body"><slot /></div>
        <footer v-if="$slots.footer"><slot name="footer" /></footer>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.backdrop {
  position: fixed;
  inset: 0;
  z-index: 900;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.45);
}
.panel {
  display: flex;
  flex-direction: column;
  max-height: calc(100vh - 32px);
  background: var(--bg-elev);
  border: 1px solid var(--border-strong);
  border-radius: 10px;
  box-shadow: var(--shadow);
}
header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 12px 8px 16px;
}
h2 {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
}
.body {
  padding: 4px 16px 16px;
  overflow: auto;
}
footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 16px;
  border-top: 1px solid var(--border);
}
</style>

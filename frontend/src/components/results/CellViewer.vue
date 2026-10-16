<script setup lang="ts">
import { computed } from 'vue'
import Modal from '../common/Modal.vue'
import Icon from '../common/Icon.vue'
import type { Cell, ResultColumn } from '../../api/types'
import { useUiStore } from '../../stores/ui'

const props = defineProps<{ column: ResultColumn; value: Cell; truncated: boolean }>()
const emit = defineEmits<{ close: [] }>()
const ui = useUiStore()

const text = computed(() => {
  if (props.value === null) return ''
  const s = String(props.value)
  if (typeof props.value === 'string' && /^\s*[[{]/.test(s)) {
    try {
      return JSON.stringify(JSON.parse(s), null, 2)
    } catch {
      return s
    }
  }
  return s
})

async function copy() {
  await navigator.clipboard.writeText(props.value === null ? 'NULL' : String(props.value))
  ui.toast('success', 'Valor copiado.')
}
</script>

<template>
  <Modal :title="`${column.name} · ${column.type}`" :width="680" @close="emit('close')">
    <p v-if="value === null" class="null">NULL</p>
    <pre v-else class="value selectable mono">{{ text }}</pre>
    <p v-if="truncated" class="warn"><Icon name="alert" :size="13" /> O valor foi truncado em 256 KB para exibição.</p>
    <template #footer>
      <button class="btn" @click="copy"><Icon name="copy" :size="14" /> Copiar</button>
      <button class="btn primary" @click="emit('close')">Fechar</button>
    </template>
  </Modal>
</template>

<style scoped>
.value {
  margin: 0;
  padding: 10px;
  max-height: 60vh;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
  background: var(--bg-sunken);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  font-size: 12px;
}
.null {
  color: var(--null);
  font-style: italic;
}
.warn {
  display: flex;
  gap: 6px;
  align-items: center;
  color: var(--warning);
  font-size: 12px;
}
</style>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Modal from '../common/Modal.vue'
import Icon from '../common/Icon.vue'
import { useUiStore } from '../../stores/ui'

const ui = useUiStore()
const typed = ref('')
const opts = computed(() => ui.confirmState?.options ?? null)
const canConfirm = computed(() => !opts.value?.typeToConfirm || typed.value.trim() === opts.value.typeToConfirm)
watch(opts, () => (typed.value = ''))
</script>

<template>
  <Modal v-if="opts" :title="opts.title" :width="560" :close-on-backdrop="false" @close="ui.resolveConfirm(false)">
    <div class="content">
      <div class="message" :class="{ danger: opts.danger }">
        <Icon v-if="opts.danger" name="alert" :size="18" />
        <p>{{ opts.message }}</p>
      </div>
      <ul v-if="opts.items?.length" class="items">
        <li v-for="(it, i) in opts.items" :key="i">
          <code class="mono selectable">{{ it.text }}</code>
          <span v-if="it.note" class="note">{{ it.note }}</span>
        </li>
      </ul>
      <label v-if="opts.typeToConfirm" class="field">
        <span>Para confirmar, digite <strong>{{ opts.typeToConfirm }}</strong></span>
        <input v-model="typed" class="input" autofocus @keydown.enter="canConfirm && ui.resolveConfirm(true)" />
      </label>
    </div>
    <template #footer>
      <button class="btn" @click="ui.resolveConfirm(false)">{{ opts.cancelLabel ?? 'Cancelar' }}</button>
      <button class="btn" :class="opts.danger ? 'danger' : 'primary'" :disabled="!canConfirm" @click="ui.resolveConfirm(true)">
        {{ opts.confirmLabel ?? 'Confirmar' }}
      </button>
    </template>
  </Modal>
</template>

<style scoped>
.content {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.message {
  display: flex;
  gap: 10px;
  align-items: flex-start;
}
.message.danger {
  color: var(--danger);
}
.message p {
  margin: 0;
  color: var(--text);
  line-height: 1.5;
}
.items {
  margin: 0;
  padding: 0;
  list-style: none;
  max-height: 240px;
  overflow: auto;
  border: 1px solid var(--border);
  border-radius: var(--radius);
}
.items li {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px 10px;
  border-bottom: 1px solid var(--border);
}
.items li:last-child {
  border-bottom: none;
}
.items code {
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 12px;
}
.note {
  font-size: 11px;
  color: var(--warning);
}
</style>

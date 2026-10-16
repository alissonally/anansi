<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Modal from '../common/Modal.vue'
import { useUiStore } from '../../stores/ui'

const ui = useUiStore()
const opts = computed(() => ui.promptState?.options ?? null)
const value = ref('')
const checked = ref(false)
watch(opts, (o) => {
  value.value = o?.value ?? ''
  checked.value = o?.checkbox?.value ?? false
}, { immediate: true })

function submit() {
  ui.resolvePrompt({ value: value.value, checked: checked.value })
}
</script>

<template>
  <Modal v-if="opts" :title="opts.title" :width="420" @close="ui.resolvePrompt(null)">
    <form class="content" @submit.prevent="submit">
      <p v-if="opts.message" class="msg">{{ opts.message }}</p>
      <label v-if="!opts.hideInput" class="field">
        <span>{{ opts.label }}</span>
        <select v-if="opts.choices" v-model="value" class="select">
          <option v-for="c in opts.choices" :key="c.value" :value="c.value">{{ c.label }}</option>
        </select>
        <textarea v-else-if="opts.multiline" v-model="value" class="input area mono" rows="6" :placeholder="opts.placeholder" />
        <input v-else v-model="value" class="input" :type="opts.password ? 'password' : 'text'" :placeholder="opts.placeholder" autocomplete="off" autofocus />
      </label>
      <label v-if="opts.checkbox" class="check">
        <input v-model="checked" type="checkbox" />
        {{ opts.checkbox.label }}
      </label>
    </form>
    <template #footer>
      <button class="btn" @click="ui.resolvePrompt(null)">Cancelar</button>
      <button class="btn primary" @click="submit">{{ opts.confirmLabel ?? 'OK' }}</button>
    </template>
  </Modal>
</template>

<style scoped>
.content {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.area {
  height: auto;
  padding: 6px 8px;
  resize: vertical;
}
.msg {
  white-space: pre-wrap;
  margin: 0;
  color: var(--text-muted);
  line-height: 1.5;
}
</style>

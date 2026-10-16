<script setup lang="ts">
import { computed, ref } from 'vue'
import Icon, { type IconName } from '../common/Icon.vue'
import CodeEditor from '../common/CodeEditor.vue'

const props = defineProps<{
  icon: IconName
  title: string
  subtitle: string
  isNew: boolean
  dirty: boolean
  saving: boolean
  loading: boolean
  loadError: string
  failure: { message: string; sql: string } | null
  preview: string[]
  previewError: string
  /** Extra notice shown under the header (e.g. how saving works). */
  note?: string
}>()
const emit = defineEmits<{ save: []; discard: []; dismiss: [] }>()
const tab = ref<'def' | 'sql'>('def')
const sql = computed(() => (props.previewError ? `-- ${props.previewError}` : props.preview.map((s) => s.trimEnd() + (s.trimEnd().endsWith(';') ? '' : ';')).join('\n\n')))

function onKey(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
    e.preventDefault()
    emit('save')
  }
}
</script>

<template>
  <div class="oe" @keydown="onKey">
    <header class="oe-head">
      <Icon :name="icon" :size="16" />
      <strong>{{ title }}</strong>
      <span class="muted">{{ subtitle }}</span>
      <span class="spacer" />
      <slot name="actions" />
      <span v-if="previewError" class="oe-perr ellipsis" :title="previewError"><Icon name="alert" :size="12" /> {{ previewError }}</span>
      <button class="btn small" :disabled="!dirty" @click="emit('discard')">Descartar</button>
      <button class="btn small primary" :disabled="(!dirty && !isNew) || saving || !!previewError || loading" title="Ctrl+S" @click="emit('save')">
        <span v-if="saving" class="spinner" /><Icon v-else name="save" :size="12" /> {{ isNew ? 'Criar' : 'Salvar' }}
      </button>
    </header>
    <p v-if="note" class="note">{{ note }}</p>
    <div v-if="failure" class="oe-banner">
      <Icon name="alert" :size="13" />
      <div class="selectable"><strong>Erro:</strong> {{ failure.message }}<code class="mono">{{ failure.sql }}</code></div>
      <button class="icon-btn" aria-label="Fechar" @click="emit('dismiss')"><Icon name="x" :size="12" /></button>
    </div>
    <div v-if="loading" class="oe-center"><span class="spinner" /> Carregando…</div>
    <div v-else-if="loadError" class="oe-center danger"><Icon name="alert" :size="16" /> {{ loadError }}</div>
    <template v-else>
      <slot name="form" />
      <nav class="oe-tabs" role="tablist">
        <button role="tab" :class="{ active: tab === 'def' }" :aria-selected="tab === 'def'" @click="tab = 'def'"><slot name="bodyLabel">Corpo</slot></button>
        <button role="tab" :class="{ active: tab === 'sql' }" :aria-selected="tab === 'sql'" @click="tab = 'sql'">SQL gerado</button>
      </nav>
      <div class="oe-split">
        <div v-show="tab === 'def'" class="oe-code"><slot name="body" /></div>
        <div v-if="tab === 'sql'" class="oe-code"><CodeEditor :model-value="sql" readonly /></div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.note {
  margin: 0;
  padding: 6px 10px;
  font-size: 12px;
  color: var(--text-muted);
  border-bottom: 1px solid var(--border);
}
.danger {
  color: var(--danger);
}
</style>

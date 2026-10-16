<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Modal from '../common/Modal.vue'
import Icon from '../common/Icon.vue'
import { useUiStore } from '../../stores/ui'
import { useConnectionsStore } from '../../stores/connections'
import { useMetadataStore } from '../../stores/metadata'
import { useTabsStore } from '../../stores/tabs'
import { backend } from '../../api/backend'
import { toAppError } from '../../api/errors'
import type { CreateDatabaseSpec } from '../../api/types'
import { runWithConfirmation, useDdlOptions, useSqlPreview } from './ddl'

const ui = useUiStore()
const conns = useConnectionsStore()
const meta = useMetadataStore()
const tabs = useTabsStore()

const state = computed(() => (ui.ddlDialog?.kind === 'database' ? ui.ddlDialog : null))
const connId = computed(() => state.value?.connectionId ?? '')
const profile = computed(() => conns.byId.get(connId.value))
const { options, error: optionsError, charsets, collationsOf, defaultCollationOf } = useDdlOptions(() => connId.value)

const spec = ref<CreateDatabaseSpec>({ name: '', charset: '', collation: '', ifNotExists: false })
const busy = ref(false)
const submitted = ref(false)

watch(state, (s) => {
  if (!s) return
  spec.value = { name: '', charset: '', collation: '', ifNotExists: false }
  submitted.value = false
})
// Start from the server defaults once known.
watch(options, (o) => {
  if (o && !spec.value.charset) {
    spec.value.charset = o.defaultCharset
    spec.value.collation = o.defaultCollation
  }
}, { immediate: true })

function onCharset() {
  spec.value.collation = defaultCollationOf(spec.value.charset)
}

const { sql, error } = useSqlPreview(spec, (s) => backend.previewCreateDatabase(connId.value, s))
const nameMissing = computed(() => !spec.value.name.trim())

async function create() {
  submitted.value = true
  if (nameMissing.value || error.value) return
  busy.value = true
  const s = state.value!
  try {
    const ok = await runWithConfirmation((confirmed) => backend.createDatabase(s.connectionId, spec.value, confirmed))
    if (!ok) return
    await meta.invalidate(s.connectionId, '', false)
    await meta.databases(s.connectionId).catch(() => undefined)
    ui.toast('success', `Database "${spec.value.name}" criado.`)
    s.onCreated?.(spec.value.name)
    ui.ddlDialog = null
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  } finally {
    busy.value = false
  }
}

function openInEditor() {
  const s = state.value!
  tabs.open({ title: `Novo database`, connectionId: s.connectionId, sql: sql.value + ';\n' })
  ui.ddlDialog = null
}
</script>

<template>
  <Modal v-if="state" :title="`Novo database — ${profile?.name ?? ''}`" :width="520" @close="ui.ddlDialog = null">
    <form class="form" @submit.prevent="create">
      <p v-if="profile?.environment === 'production'" class="callout"><Icon name="shield" :size="14" /> Conexão de produção: será pedida confirmação.</p>
      <label class="field">
        <span>Nome</span>
        <input v-model="spec.name" class="input" :class="{ invalid: submitted && nameMissing }" maxlength="64" spellcheck="false" autofocus />
      </label>
      <div class="row">
        <label class="field grow">
          <span>Charset</span>
          <select v-model="spec.charset" class="select" :disabled="!options" @change="onCharset">
            <option value="">(padrão do servidor)</option>
            <option v-for="c in charsets" :key="c.name" :value="c.name">{{ c.name }} — {{ c.description }}</option>
          </select>
        </label>
        <label class="field grow">
          <span>Collation</span>
          <select v-model="spec.collation" class="select" :disabled="!spec.charset">
            <option value="">(padrão do charset)</option>
            <option v-for="c in collationsOf(spec.charset)" :key="c" :value="c">{{ c }}</option>
          </select>
        </label>
      </div>
      <label class="check"><input v-model="spec.ifNotExists" type="checkbox" /> IF NOT EXISTS</label>
      <p v-if="optionsError" class="err">{{ optionsError }}</p>
      <div class="field">
        <span>SQL</span>
        <pre class="preview mono selectable" :class="{ invalid: error && !nameMissing }">{{ error && !nameMissing ? error : sql || '—' }}</pre>
      </div>
    </form>
    <template #footer>
      <button class="btn" @click="ui.ddlDialog = null">Cancelar</button>
      <button class="btn" :disabled="!!error || nameMissing" title="Abre o SQL numa nova aba sem executar" @click="openInEditor">Abrir no editor</button>
      <button class="btn primary" :disabled="busy" @click="create"><span v-if="busy" class="spinner" /> Criar database</button>
    </template>
  </Modal>
</template>

<style scoped>
.form {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.row {
  display: flex;
  gap: 12px;
}
.grow {
  flex: 1;
  min-width: 0;
}
.preview {
  margin: 0;
  padding: 8px 10px;
  min-height: 40px;
  white-space: pre-wrap;
  word-break: break-word;
  background: var(--bg-sunken);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  font-size: 12px;
}
.preview.invalid {
  color: var(--danger);
  border-color: var(--danger);
}
.err {
  margin: 0;
  color: var(--danger);
  font-size: 12px;
}
.callout {
  display: flex;
  gap: 8px;
  align-items: center;
  margin: 0;
  padding: 8px 10px;
  border-radius: var(--radius);
  background: var(--danger-soft);
  color: var(--danger);
  font-size: 12px;
}
@media (max-width: 560px) {
  .row {
    flex-direction: column;
  }
}
</style>

<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Tab } from '../../stores/tabs'
import { useTabsStore } from '../../stores/tabs'
import { useMetadataStore } from '../../stores/metadata'
import { backend } from '../../api/backend'
import type { TableInfo, TriggerDef } from '../../api/types'
import EditorShell from './EditorShell.vue'
import CodeEditor from '../common/CodeEditor.vue'
import Icon from '../common/Icon.vue'
import { useObjectEditor } from '../../composables/objectEditor'
import { useObjectActions } from '../../composables/objectActions'

const props = defineProps<{ tab: Tab }>()
const tabs = useTabsStore()
const meta = useMetadataStore()
const actions = useObjectActions()
const conn = computed(() => props.tab.connectionId ?? '')
const db = computed(() => props.tab.database)
const definers = ref<string[]>([])
void backend.definers(conn.value).then((d) => (definers.value = d)).catch(() => undefined)
void meta.tables(conn.value, db.value).catch(() => undefined)
const tables = computed(() => (meta.entry<TableInfo[]>(conn.value, 'tables', db.value)?.data ?? []).filter((t) => t.type === 'table').map((t) => t.name))

const ed = useObjectEditor<TriggerDef>(() => props.tab, {
  load: () => backend.loadTriggerDef(conn.value, db.value, props.tab.objectName ?? ''),
  blank: () => ({ database: db.value, name: 'novo_trigger', origName: '', table: tables.value[0] ?? '', timing: 'BEFORE', event: 'INSERT', body: 'BEGIN\n  -- SET NEW.coluna = ...;\nEND', definer: '' }),
  preview: async (d) => [await backend.previewTriggerDef(conn.value, d)],
  save: (d, c) => backend.saveTriggerDef(conn.value, d, c),
  nameOf: (d) => d.name,
})
const { def } = ed
void ed.load()
</script>

<template>
  <EditorShell
    icon="zap" :title="def?.name || tab.objectName || 'Novo trigger'" :subtitle="`trigger · ${db}`" :is-new="ed.isNew.value"
    :dirty="ed.dirty.value" :saving="ed.saving.value" :loading="ed.loading.value" :load-error="ed.loadError.value" :failure="ed.failure.value"
    :preview="ed.preview.value" :preview-error="ed.previewError.value"
    note="Ao salvar um trigger existente, a nova versão é validada com um nome temporário antes de substituir a atual; se a substituição falhar, a versão anterior é restaurada."
    @save="ed.save" @discard="ed.discard" @dismiss="ed.failure.value = null"
  >
    <template #actions>
      <button v-if="def?.table" class="btn small" @click="actions.open(conn, { kind: 'table', database: db, name: def.table }, 'data')"><Icon name="table" :size="12" /> Tabela</button>
      <button v-if="!ed.isNew.value" class="btn small danger" @click="actions.drop(conn, [{ kind: 'trigger', database: db, name: tab.objectName ?? '' }]).then((ok) => ok && tabs.close(tab.id, true))">
        <Icon name="trash" :size="12" /> Excluir…
      </button>
    </template>
    <template v-if="def" #form>
      <div class="oe-form">
        <label class="field"><span>Nome</span><input v-model="def.name" class="input mono" spellcheck="false" /></label>
        <label class="field"><span>Tabela</span>
          <select v-model="def.table" class="select"><option v-if="def.table && !tables.includes(def.table)" :value="def.table">{{ def.table }}</option><option v-for="t in tables" :key="t" :value="t">{{ t }}</option></select>
        </label>
        <label class="field"><span>Momento</span><select v-model="def.timing" class="select"><option>BEFORE</option><option>AFTER</option></select></label>
        <label class="field"><span>Evento</span><select v-model="def.event" class="select"><option>INSERT</option><option>UPDATE</option><option>DELETE</option></select></label>
        <label class="field"><span>Definer</span>
          <input v-model="def.definer" class="input mono" list="trigger-definers" placeholder="(usuário atual)" spellcheck="false" />
          <datalist id="trigger-definers"><option v-for="d in definers" :key="d" :value="d" /></datalist>
        </label>
      </div>
    </template>
    <template #bodyLabel>Corpo (FOR EACH ROW)</template>
    <template #body><CodeEditor v-if="def" v-model="def.body" @save="ed.save" /></template>
  </EditorShell>
</template>

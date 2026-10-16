<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Tab } from '../../stores/tabs'
import { backend } from '../../api/backend'
import type { ViewDef } from '../../api/types'
import EditorShell from './EditorShell.vue'
import CodeEditor from '../common/CodeEditor.vue'
import { useObjectEditor } from '../../composables/objectEditor'
import { formatSql } from '../../editor/format'

const props = defineProps<{ tab: Tab }>()
const emit = defineEmits<{ saved: [] }>()
const conn = computed(() => props.tab.connectionId ?? '')
const db = computed(() => props.tab.database)
const definers = ref<string[]>([])
void backend.definers(conn.value).then((d) => (definers.value = d)).catch(() => undefined)

const ed = useObjectEditor<ViewDef>(() => props.tab, {
  load: async () => {
    const v = await backend.loadViewDef(conn.value, db.value, props.tab.objectName ?? '')
    // The server stores the SELECT on a single line; format it for editing.
    try {
      v.select = formatSql(v.select, 'mysql')
    } catch {
      // keep as is
    }
    return v
  },
  blank: () => ({ database: db.value, name: 'nova_view', origName: '', definer: '', security: 'DEFINER', algorithm: 'UNDEFINED', checkOption: '', select: 'SELECT\n  1 AS coluna' }),
  preview: (d) => backend.previewViewDef(conn.value, d),
  save: (d, c) => backend.saveViewDef(conn.value, d, c),
  nameOf: (d) => d.name,
})
const { def } = ed
void ed.load()

async function save() {
  if (await ed.save()) emit('saved')
}
</script>

<template>
  <EditorShell
    icon="view" :title="def?.name || tab.objectName || 'Nova view'" :subtitle="`view · ${db}`" :is-new="ed.isNew.value"
    :dirty="ed.dirty.value" :saving="ed.saving.value" :loading="ed.loading.value" :load-error="ed.loadError.value" :failure="ed.failure.value"
    :preview="ed.preview.value" :preview-error="ed.previewError.value"
    @save="save" @discard="ed.discard" @dismiss="ed.failure.value = null"
  >
    <template v-if="def" #form>
      <div class="oe-form">
        <label class="field"><span>Nome</span><input v-model="def.name" class="input mono" spellcheck="false" /></label>
        <label class="field"><span>Algoritmo</span><select v-model="def.algorithm" class="select"><option>UNDEFINED</option><option>MERGE</option><option>TEMPTABLE</option></select></label>
        <label class="field"><span>SQL SECURITY</span><select v-model="def.security" class="select"><option>DEFINER</option><option>INVOKER</option></select></label>
        <label class="field"><span>CHECK OPTION</span><select v-model="def.checkOption" class="select"><option value="">nenhum</option><option>CASCADED</option><option>LOCAL</option></select></label>
        <label class="field"><span>Definer</span>
          <input v-model="def.definer" class="input mono" list="view-definers" placeholder="(usuário atual)" spellcheck="false" />
          <datalist id="view-definers"><option v-for="d in definers" :key="d" :value="d" /></datalist>
        </label>
      </div>
    </template>
    <template #bodyLabel>SELECT</template>
    <template #body><CodeEditor v-if="def" v-model="def.select" @save="save" /></template>
  </EditorShell>
</template>

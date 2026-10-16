<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Tab } from '../../stores/tabs'
import { useTabsStore } from '../../stores/tabs'
import { backend } from '../../api/backend'
import type { EventDef } from '../../api/types'
import EditorShell from './EditorShell.vue'
import CodeEditor from '../common/CodeEditor.vue'
import Icon from '../common/Icon.vue'
import { useObjectEditor } from '../../composables/objectEditor'
import { useObjectActions } from '../../composables/objectActions'

const props = defineProps<{ tab: Tab }>()
const tabs = useTabsStore()
const actions = useObjectActions()
const conn = computed(() => props.tab.connectionId ?? '')
const db = computed(() => props.tab.database)
const definers = ref<string[]>([])
void backend.definers(conn.value).then((d) => (definers.value = d)).catch(() => undefined)
const fields = ['SECOND', 'MINUTE', 'HOUR', 'DAY', 'WEEK', 'MONTH', 'QUARTER', 'YEAR', 'MINUTE_SECOND', 'HOUR_SECOND', 'HOUR_MINUTE', 'DAY_SECOND', 'DAY_MINUTE', 'DAY_HOUR', 'YEAR_MONTH']

function now() {
  const d = new Date(Date.now() + 3600_000)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:00:00`
}

const ed = useObjectEditor<EventDef>(() => props.tab, {
  load: () => backend.loadEventDef(conn.value, db.value, props.tab.objectName ?? ''),
  blank: () => ({ database: db.value, name: 'novo_evento', origName: '', definer: '', recurring: true, executeAt: now(), intervalValue: '1', intervalField: 'DAY',
    starts: '', ends: '', preserve: true, status: 'DISABLE', comment: '', body: 'BEGIN\n  -- instruções\nEND' }),
  preview: async (d) => [await backend.previewEventDef(conn.value, d)],
  save: (d, c) => backend.saveEventDef(conn.value, d, c),
  nameOf: (d) => d.name,
})
const { def } = ed
void ed.load()
</script>

<template>
  <EditorShell
    icon="clock" :title="def?.name || tab.objectName || 'Novo evento'" :subtitle="`evento · ${db}`" :is-new="ed.isNew.value"
    :dirty="ed.dirty.value" :saving="ed.saving.value" :loading="ed.loading.value" :load-error="ed.loadError.value" :failure="ed.failure.value"
    :preview="ed.preview.value" :preview-error="ed.previewError.value"
    note="Eventos só rodam com o agendador ligado (event_scheduler = ON, ver Variáveis na aba do servidor)."
    @save="ed.save" @discard="ed.discard" @dismiss="ed.failure.value = null"
  >
    <template #actions>
      <button v-if="!ed.isNew.value" class="btn small danger" @click="actions.drop(conn, [{ kind: 'event', database: db, name: tab.objectName ?? '' }]).then((ok) => ok && tabs.close(tab.id, true))">
        <Icon name="trash" :size="12" /> Excluir…
      </button>
    </template>
    <template v-if="def" #form>
      <div class="oe-form">
        <label class="field"><span>Nome</span><input v-model="def.name" class="input mono" spellcheck="false" /></label>
        <label class="field"><span>Estado</span>
          <select v-model="def.status" class="select"><option value="ENABLE">ativado</option><option value="DISABLE">desativado</option><option value="DISABLE ON SLAVE">desativado na réplica</option></select>
        </label>
        <label class="field"><span>Agenda</span>
          <select v-model="def.recurring" class="select"><option :value="true">repetir a cada…</option><option :value="false">uma vez, em…</option></select>
        </label>
        <template v-if="def.recurring">
          <label class="field"><span>Intervalo</span><input v-model="def.intervalValue" class="input mono" placeholder="1 ou '1:30'" /></label>
          <label class="field"><span>Unidade</span><select v-model="def.intervalField" class="select"><option v-for="f in fields" :key="f" :value="f">{{ f }}</option></select></label>
          <label class="field"><span>Início (opcional)</span><input v-model="def.starts" class="input mono" placeholder="AAAA-MM-DD HH:MM:SS" /></label>
          <label class="field"><span>Fim (opcional)</span><input v-model="def.ends" class="input mono" placeholder="AAAA-MM-DD HH:MM:SS" /></label>
        </template>
        <label v-else class="field"><span>Executar em</span><input v-model="def.executeAt" class="input mono" placeholder="AAAA-MM-DD HH:MM:SS ou expressão" /></label>
        <label class="check"><input v-model="def.preserve" type="checkbox" /> manter após expirar (ON COMPLETION PRESERVE)</label>
        <label class="field"><span>Definer</span>
          <input v-model="def.definer" class="input mono" list="event-definers" placeholder="(usuário atual)" spellcheck="false" />
          <datalist id="event-definers"><option v-for="d in definers" :key="d" :value="d" /></datalist>
        </label>
        <label class="field wide"><span>Comentário</span><input v-model="def.comment" class="input" /></label>
      </div>
    </template>
    <template #bodyLabel>Corpo (DO)</template>
    <template #body><CodeEditor v-if="def" v-model="def.body" @save="ed.save" /></template>
  </EditorShell>
</template>

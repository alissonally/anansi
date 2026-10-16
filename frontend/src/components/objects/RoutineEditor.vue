<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Tab } from '../../stores/tabs'
import { useTabsStore } from '../../stores/tabs'
import { backend } from '../../api/backend'
import type { RoutineDef } from '../../api/types'
import EditorShell from './EditorShell.vue'
import CodeEditor from '../common/CodeEditor.vue'
import Icon from '../common/Icon.vue'
import { useObjectEditor } from '../../composables/objectEditor'
import { useObjectActions } from '../../composables/objectActions'
import { routineCall } from '../../composables/templates'

const props = defineProps<{ tab: Tab }>()
const tabs = useTabsStore()
const actions = useObjectActions()
const conn = computed(() => props.tab.connectionId ?? '')
const db = computed(() => props.tab.database)
const definers = ref<string[]>([])
void backend.definers(conn.value).then((d) => (definers.value = d)).catch(() => undefined)

const ed = useObjectEditor<RoutineDef>(() => props.tab, {
  load: () => backend.loadRoutineDef(conn.value, db.value, props.tab.objectType ?? 'PROCEDURE', props.tab.objectName ?? ''),
  blank: () => ({
    database: db.value, name: props.tab.objectType === 'FUNCTION' ? 'nova_function' : 'nova_procedure', origName: '', type: (props.tab.objectType as 'PROCEDURE' | 'FUNCTION') ?? 'PROCEDURE',
    origType: '', params: [], returns: props.tab.objectType === 'FUNCTION' ? 'INT' : '', body: props.tab.objectType === 'FUNCTION' ? 'BEGIN\n  RETURN 0;\nEND' : 'BEGIN\n  \nEND',
    deterministic: false, dataAccess: 'CONTAINS SQL', security: 'DEFINER', definer: '', comment: '',
  }),
  preview: async (d) => [await backend.previewRoutineDef(conn.value, d)],
  save: (d, c) => backend.saveRoutineDef(conn.value, d, c),
  nameOf: (d) => d.name,
})
const { def } = ed
void ed.load()

const params = computed(() => def.value?.params ?? [])
function addParam() {
  const d = def.value
  if (!d) return
  ;(d.params ?? (d.params = [])).push({ mode: d.type === 'PROCEDURE' ? 'IN' : '', name: `p${params.value.length + 1}`, type: 'INT' })
}
function move(i: number, delta: number) {
  const l = def.value?.params
  const j = i + delta
  if (!l || j < 0 || j >= l.length) return
  ;[l[i], l[j]] = [l[j]!, l[i]!]
}

function onTypeChange() {
  const d = def.value
  if (!d) return
  for (const p of d.params ?? []) p.mode = d.type === 'FUNCTION' ? '' : (p.mode || 'IN')
  if (d.type === 'FUNCTION' && !d.returns) d.returns = 'INT'
}

async function save() {
  if (await ed.save()) {
    const t = tabs.find(props.tab.id)
    if (t && def.value) t.objectType = def.value.type
  }
}

function run() {
  const d = def.value
  if (!d || ed.isNew.value) return
  tabs.open({ title: `Executar ${d.name}`, connectionId: conn.value, database: db.value,
    sql: routineCall(db.value, { name: d.origName || d.name, type: d.type, params: d.params, body: '', deterministic: false, dataAccess: '', security: '', definer: '' }) })
}
</script>

<template>
  <EditorShell
    icon="fn" :title="def?.name || tab.objectName || 'Nova rotina'" :subtitle="`${(def?.type ?? tab.objectType ?? '').toLowerCase()} · ${db}`" :is-new="ed.isNew.value"
    :dirty="ed.dirty.value" :saving="ed.saving.value" :loading="ed.loading.value" :load-error="ed.loadError.value" :failure="ed.failure.value"
    :preview="ed.preview.value" :preview-error="ed.previewError.value"
    note="O servidor não altera o corpo de rotinas: ao salvar, a nova versão é criada primeiro com um nome temporário (validação), depois a antiga é substituída. Se algo falhar, a versão anterior é restaurada. Privilégios EXECUTE concedidos à rotina precisam ser refeitos."
    @save="save" @discard="ed.discard" @dismiss="ed.failure.value = null"
  >
    <template #actions>
      <button v-if="!ed.isNew.value" class="btn small" @click="run"><Icon name="play" :size="11" /> Executar…</button>
      <button v-if="!ed.isNew.value" class="btn small danger" @click="actions.drop(conn, [{ kind: def?.type === 'FUNCTION' ? 'function' : 'procedure', database: db, name: tab.objectName ?? '' }]).then((ok) => ok && tabs.close(tab.id, true))">
        <Icon name="trash" :size="12" /> Excluir…
      </button>
    </template>
    <template v-if="def" #form>
      <div class="oe-form">
        <label class="field"><span>Nome</span><input v-model="def.name" class="input mono" spellcheck="false" /></label>
        <label class="field"><span>Tipo</span>
          <select v-model="def.type" class="select" @change="onTypeChange">
            <option value="PROCEDURE">PROCEDURE</option><option value="FUNCTION">FUNCTION</option>
          </select>
        </label>
        <label v-if="def.type === 'FUNCTION'" class="field"><span>Retorna</span><input v-model="def.returns" class="input mono" placeholder="INT, VARCHAR(100)…" spellcheck="false" /></label>
        <label class="field"><span>Acesso a dados</span>
          <select v-model="def.dataAccess" class="select"><option v-for="a in ['CONTAINS SQL', 'NO SQL', 'READS SQL DATA', 'MODIFIES SQL DATA']" :key="a" :value="a">{{ a }}</option></select>
        </label>
        <label class="field"><span>SQL SECURITY</span>
          <select v-model="def.security" class="select"><option value="DEFINER">DEFINER</option><option value="INVOKER">INVOKER</option></select>
        </label>
        <label class="field"><span>Definer</span>
          <input v-model="def.definer" class="input mono" list="routine-definers" placeholder="(usuário atual)" spellcheck="false" />
          <datalist id="routine-definers"><option v-for="d in definers" :key="d" :value="d" /></datalist>
        </label>
        <label class="check"><input v-model="def.deterministic" type="checkbox" /> DETERMINISTIC</label>
        <label class="field wide"><span>Comentário</span><input v-model="def.comment" class="input" /></label>
        <div class="wide params">
          <div class="phead"><span class="muted">Parâmetros</span><button class="btn small" @click="addParam"><Icon name="plus" :size="11" /> Adicionar</button></div>
          <div v-for="(p, i) in params" :key="i" class="prow">
            <select v-if="def.type === 'PROCEDURE'" v-model="p.mode" class="select compact"><option v-for="m in ['IN', 'OUT', 'INOUT']" :key="m" :value="m">{{ m }}</option></select>
            <input v-model="p.name" class="input compact mono" placeholder="nome" spellcheck="false" />
            <input v-model="p.type" class="input compact mono" placeholder="tipo" spellcheck="false" />
            <button class="icon-btn" aria-label="Subir" @click="move(i, -1)"><Icon name="arrowUp" :size="12" /></button>
            <button class="icon-btn" aria-label="Descer" @click="move(i, 1)"><Icon name="arrowDown" :size="12" /></button>
            <button class="icon-btn" aria-label="Remover" @click="def.params!.splice(i, 1)"><Icon name="x" :size="12" /></button>
          </div>
          <p v-if="!params.length" class="faint small">Sem parâmetros.</p>
        </div>
      </div>
    </template>
    <template #body>
      <CodeEditor v-if="def" v-model="def.body" @save="save" />
    </template>
  </EditorShell>
</template>

<style scoped>
.params {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.phead {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 12px;
}
.prow {
  display: flex;
  gap: 4px;
  align-items: center;
}
.prow .input {
  flex: 1;
  min-width: 0;
}
.small {
  font-size: 12px;
  margin: 0;
}
</style>

<script setup lang="ts">
// Structure comparison between two databases (same or different
// connections) with the script that brings the target in line.
import { computed, ref, watch } from 'vue'
import Modal from '../common/Modal.vue'
import Icon from '../common/Icon.vue'
import { useUiStore } from '../../stores/ui'
import { useConnectionsStore } from '../../stores/connections'
import { useMetadataStore } from '../../stores/metadata'
import { useTabsStore } from '../../stores/tabs'
import { backend } from '../../api/backend'
import { toAppError } from '../../api/errors'
import type { DatabaseInfo, DiffStatus, SchemaDiff, SchemaDiffItem } from '../../api/types'
import { reportResults, withConfirmation } from '../../composables/objectActions'

const ui = useUiStore()
const conns = useConnectionsStore()
const meta = useMetadataStore()
const tabs = useTabsStore()
const st = computed(() => ui.compareDialog)

const srcConn = ref('')
const srcDb = ref('')
const dstConn = ref('')
const dstDb = ref('')
const srcDbs = ref<string[]>([])
const dstDbs = ref<string[]>([])
const diff = ref<SchemaDiff | null>(null)
const picked = ref(new Set<string>())
const current = ref<SchemaDiffItem | null>(null)
const showSame = ref(false)
const continueOnError = ref(false)
const busy = ref(false)

const connected = computed(() => conns.profiles.filter((p) => conns.state(p.id).status === 'connected'))
async function dbsOf(id: string): Promise<string[]> {
  if (!id) return []
  return (await meta.databases(id).catch(() => [] as DatabaseInfo[])).filter((d) => !d.system).map((d) => d.name)
}

watch(st, async (s) => {
  if (!s) return
  srcConn.value = s.connectionId
  srcDb.value = s.database
  dstConn.value = s.connectionId
  dstDb.value = ''
  diff.value = null
  current.value = null
}, { immediate: true })
watch(srcConn, async (id) => (srcDbs.value = await dbsOf(id)), { immediate: true })
watch(dstConn, async (id) => (dstDbs.value = await dbsOf(id)), { immediate: true })

const key = (i: SchemaDiffItem) => i.kind + ' ' + i.name
const items = computed(() => (diff.value?.items ?? []).filter((i) => showSame.value || i.status !== 'same'))
const counts = computed(() => {
  const c: Record<DiffStatus, number> = { create: 0, drop: 0, alter: 0, same: 0 }
  for (const i of diff.value?.items ?? []) c[i.status]++
  return c
})
const statusLabel: Record<DiffStatus, string> = { create: 'criar', drop: 'excluir', alter: 'alterar', same: 'igual' }
const kindLabel: Record<string, string> = { table: 'tabela', view: 'view', procedure: 'procedure', function: 'function', trigger: 'trigger', event: 'evento' }

async function compare() {
  if (!srcDb.value || !dstDb.value) return ui.toast('warning', 'Escolha os dois databases.')
  busy.value = true
  try {
    diff.value = await backend.compareSchemas(srcConn.value, srcDb.value, dstConn.value, dstDb.value)
    // Drops are opt-in: objects that exist only in the target are kept by default.
    picked.value = new Set(diff.value.items.filter((i) => i.status === 'create' || i.status === 'alter').map(key))
    current.value = items.value[0] ?? null
    for (const w of diff.value.warnings ?? []) ui.toast('warning', w, 10000)
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  } finally {
    busy.value = false
  }
}

function toggle(i: SchemaDiffItem) {
  const s = new Set(picked.value)
  if (s.has(key(i))) s.delete(key(i))
  else s.add(key(i))
  picked.value = s
}
const script = computed(() => (diff.value?.items ?? []).filter((i) => picked.value.has(key(i))).flatMap((i) => i.statements))
// Routines, triggers and events contain semicolons: the script uses DELIMITER.
const scriptText = computed(() => (script.value.length ? 'DELIMITER $$\n' + script.value.map((s) => s + '$$').join('\n') + '\nDELIMITER ;\n' : ''))

function openScript() {
  const t = tabs.open({ title: `sync ${dstDb.value}`, sql: scriptText.value, connectionId: dstConn.value, database: dstDb.value })
  tabs.activate(t.id)
  ui.compareDialog = null
}

async function apply() {
  busy.value = true
  try {
    const res = await withConfirmation((c) => backend.applySchemaDiff(dstConn.value, dstDb.value, script.value, continueOnError.value, c))
    if (!res) return
    reportResults(res, `${res.length} instrução(ões) aplicada(s) em ${dstDb.value}.`)
    await meta.invalidate(dstConn.value, dstDb.value)
    await compare()
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Modal v-if="st" title="Comparar estruturas" :width="1040" :close-on-backdrop="false" @close="ui.compareDialog = null">
    <div class="sides">
      <fieldset>
        <legend>Origem (modelo)</legend>
        <select v-model="srcConn" class="select" aria-label="Conexão de origem"><option v-for="p in connected" :key="p.id" :value="p.id">{{ p.name }}</option></select>
        <select v-model="srcDb" class="select" aria-label="Database de origem"><option value="" disabled>database…</option><option v-for="d in srcDbs" :key="d" :value="d">{{ d }}</option></select>
      </fieldset>
      <Icon name="chevronRight" :size="18" />
      <fieldset>
        <legend>Destino (será alterado)</legend>
        <select v-model="dstConn" class="select" aria-label="Conexão de destino"><option v-for="p in connected" :key="p.id" :value="p.id">{{ p.name }}</option></select>
        <select v-model="dstDb" class="select" aria-label="Database de destino"><option value="" disabled>database…</option><option v-for="d in dstDbs" :key="d" :value="d">{{ d }}</option></select>
      </fieldset>
      <button class="btn primary" :disabled="busy || !srcDb || !dstDb" @click="compare"><span v-if="busy" class="spinner" /> Comparar</button>
    </div>
    <div v-if="diff" class="body">
      <div class="list">
        <div class="summary small">
          <span class="s-create">{{ counts.create }} criar</span> · <span class="s-alter">{{ counts.alter }} alterar</span> · <span class="s-drop">{{ counts.drop }} excluir</span> · {{ counts.same }} iguais
          <label class="check"><input v-model="showSame" type="checkbox" /> mostrar iguais</label>
        </div>
        <div class="items">
          <div v-for="i in items" :key="key(i)" class="item" :class="{ on: current && key(current) === key(i) }" @click="current = i">
            <input type="checkbox" :checked="picked.has(key(i))" :disabled="i.status === 'same'" :aria-label="`Incluir ${i.name}`" @click.stop @change="toggle(i)" />
            <span class="badge" :class="'s-' + i.status">{{ statusLabel[i.status] }}</span>
            <span class="faint small">{{ kindLabel[i.kind] }}</span>
            <span class="mono">{{ i.name }}</span>
          </div>
          <p v-if="!items.length" class="faint">As estruturas são iguais.</p>
        </div>
      </div>
      <div v-if="current" class="detail">
        <div class="lbl">Instruções para o destino</div>
        <pre class="sql mono selectable stmts">{{ current.statements.join(';\n') || '(nenhuma)' }}</pre>
        <div class="pair">
          <div><div class="lbl">Origem</div><pre class="sql mono selectable">{{ current.sourceSQL || '(não existe)' }}</pre></div>
          <div><div class="lbl">Destino</div><pre class="sql mono selectable">{{ current.targetSQL || '(não existe)' }}</pre></div>
        </div>
      </div>
    </div>
    <template #footer>
      <span class="hint">Compara apenas a estrutura. Objetos só do destino não são excluídos se não forem marcados.</span>
      <label v-if="diff" class="check small"><input v-model="continueOnError" type="checkbox" /> continuar após erros</label>
      <button class="btn" @click="ui.compareDialog = null">Fechar</button>
      <button class="btn" :disabled="!script.length" @click="openScript">Abrir script no editor</button>
      <button class="btn primary" :disabled="busy || !script.length" @click="apply">Aplicar {{ script.length }} instrução(ões)</button>
    </template>
  </Modal>
</template>

<style scoped>
.sides {
  display: flex;
  align-items: center;
  gap: 10px;
}
fieldset {
  display: flex;
  gap: 6px;
  margin: 0;
  padding: 6px 10px 8px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
}
legend {
  font-size: 12px;
  color: var(--text-muted);
  padding: 0 4px;
}
.body {
  display: grid;
  grid-template-columns: 320px 1fr;
  gap: 12px;
  margin-top: 10px;
  height: 440px;
}
.list,
.detail {
  display: flex;
  flex-direction: column;
  min-height: 0;
  gap: 4px;
}
.summary {
  display: flex;
  gap: 6px;
  align-items: center;
  flex-wrap: wrap;
}
.summary .check {
  margin-left: auto;
}
.items {
  flex: 1;
  overflow: auto;
  border: 1px solid var(--border);
  border-radius: var(--radius);
}
.item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 3px 6px;
  cursor: pointer;
}
.item:hover {
  background: var(--bg-hover);
}
.item.on {
  background: var(--bg-active);
}
.badge {
  font-size: 10px;
  padding: 1px 5px;
  border-radius: var(--radius-sm);
  background: var(--bg-sunken);
  min-width: 46px;
  text-align: center;
}
.s-create {
  color: var(--success);
}
.s-alter {
  color: var(--warning);
}
.s-drop {
  color: var(--danger);
}
.small {
  font-size: 12px;
}
.faint {
  color: var(--text-faint);
  margin: 6px;
}
.lbl {
  font-size: 12px;
  color: var(--text-muted);
}
.pair {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  flex: 1;
  min-height: 0;
}
.pair > div {
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.sql {
  margin: 0;
  padding: 8px;
  overflow: auto;
  background: var(--bg-sunken);
  border-radius: var(--radius-sm);
  font-size: 11.5px;
  white-space: pre-wrap;
  flex: 1;
}
.stmts {
  flex: 0 0 auto;
  max-height: 140px;
}
.hint {
  margin-right: auto;
  color: var(--text-faint);
  font-size: 11px;
  align-self: center;
}
</style>

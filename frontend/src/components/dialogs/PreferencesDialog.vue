<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Modal from '../common/Modal.vue'
import Icon from '../common/Icon.vue'
import { useUiStore } from '../../stores/ui'
import { defaultPreferences, useSettingsStore } from '../../stores/settings'
import { useTabsStore } from '../../stores/tabs'
import { backend } from '../../api/backend'
import { toAppError } from '../../api/errors'
import type { Preferences } from '../../api/types'
import { comboOf, conflicts, effectiveShortcuts, fixedShortcuts, shortcutActions } from '../../utils/shortcuts'

const ui = useUiStore()
const settings = useSettingsStore()
const tabs = useTabsStore()
const section = ref<'general' | 'editor' | 'grid' | 'log' | 'shortcuts'>('general')
const form = ref<Preferences>(copy())
function copy(): Preferences {
  return { ...defaultPreferences, ...settings.prefs, shortcuts: { ...(settings.prefs.shortcuts ?? {}) }, timestampColumns: [...(settings.prefs.timestampColumns ?? [])] }
}
watch(() => ui.preferencesOpen, (o) => {
  if (o) {
    form.value = copy()
    section.value = 'general'
  }
})

const keys = computed(() => effectiveShortcuts(form.value.shortcuts))
const clashes = computed(() => conflicts(keys.value))
const groups = computed(() => [...new Set(shortcutActions.map((a) => a.group))])
const recording = ref('')

function record(e: KeyboardEvent, id: string) {
  if (e.key === 'Tab') return
  e.preventDefault()
  e.stopPropagation()
  if (e.key === 'Escape') {
    recording.value = ''
    ;(e.target as HTMLElement).blur()
    return
  }
  if (e.key === 'Backspace' || e.key === 'Delete') {
    setKey(id, '')
    return
  }
  const c = comboOf(e)
  if (!c) return
  if (!/^(Ctrl|Alt)\+|^F\d+$/.test(c)) {
    ui.toast('warning', 'Use Ctrl, Alt ou uma tecla de função (F1–F12).', 3000)
    return
  }
  setKey(id, c)
  recording.value = ''
  ;(e.target as HTMLElement).blur()
}
function setKey(id: string, combo: string) {
  const def = shortcutActions.find((a) => a.id === id)?.key ?? ''
  const s = { ...(form.value.shortcuts ?? {}) }
  if (combo === def) delete s[id]
  else s[id] = combo
  form.value.shortcuts = s
}

async function save() {
  if (clashes.value.size) {
    section.value = 'shortcuts'
    ui.toast('warning', 'Há atalhos repetidos; ajuste-os antes de salvar.')
    return
  }
  try {
    await settings.save({ ...form.value })
    ui.preferencesOpen = false
    ui.toast('success', 'Preferências salvas.')
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}

function restoreDefaults() {
  form.value = { ...defaultPreferences, shortcuts: {}, timestampColumns: form.value.timestampColumns }
}

async function clearHistory() {
  if (!(await ui.confirm({ title: 'Limpar histórico', message: 'Remover todo o histórico de consultas de todas as conexões?', confirmLabel: 'Limpar', danger: true }))) return
  try {
    await backend.clearHistory('')
    ui.toast('success', 'Histórico removido.')
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}

const sections = [['general', 'Geral'], ['editor', 'Editor SQL'], ['grid', 'Grade de dados'], ['log', 'Log e histórico'], ['shortcuts', 'Atalhos']] as const
</script>

<template>
  <Modal v-if="ui.preferencesOpen" title="Preferências" :width="640" @close="ui.preferencesOpen = false">
    <nav class="sections" role="tablist">
      <button v-for="s in sections" :key="s[0]" role="tab" :aria-selected="section === s[0]" :class="{ active: section === s[0] }" @click="section = s[0]">
        {{ s[1] }} <span v-if="s[0] === 'shortcuts' && clashes.size" class="warn">●</span>
      </button>
    </nav>
    <div class="form">
      <template v-if="section === 'general'">
        <div class="row">
          <label class="field grow">
            <span>Tema</span>
            <select v-model="form.theme" class="select">
              <option value="system">Seguir o sistema</option>
              <option value="dark">Escuro</option>
              <option value="light">Claro</option>
            </select>
          </label>
        </div>
        <label class="check"><input v-model="form.reconnectOnStartup" type="checkbox" /> Reconectar ao iniciar as conexões que estavam abertas</label>
        <label class="check"><input v-model="form.confirmDestructive" type="checkbox" /> Confirmar operações destrutivas (DROP, TRUNCATE, DELETE/UPDATE sem WHERE)</label>
        <p class="hint">Em conexões de produção, qualquer alteração sempre pede confirmação.</p>
        <label class="check"><input v-model="form.showSystemSchemas" type="checkbox" /> Mostrar schemas do sistema (mysql, sys, information_schema…)</label>
        <div class="row">
          <label class="field grow">
            <span>Tempo limite de consulta (s, 0 = sem limite)</span>
            <input v-model.number="form.queryTimeoutSeconds" class="input" type="number" min="0" />
          </label>
          <div class="field end"><button class="btn" @click="tabs.resetLayout(); ui.toast('success', 'Tamanhos dos painéis restaurados.', 2500)">Restaurar tamanhos dos painéis</button></div>
        </div>
      </template>

      <template v-else-if="section === 'editor'">
        <div class="row">
          <label class="field">
            <span>Fonte (px)</span>
            <input v-model.number="form.editorFontSize" class="input" type="number" min="8" max="40" />
          </label>
          <label class="field">
            <span>Largura da tabulação</span>
            <input v-model.number="form.editorTabSize" class="input" type="number" min="1" max="8" />
          </label>
        </div>
        <label class="check"><input v-model="form.editorUseTabs" type="checkbox" /> Inserir tabulações (em vez de espaços)</label>
        <label class="check"><input v-model="form.editorWordWrap" type="checkbox" /> Quebrar linhas longas</label>
        <label class="check"><input v-model="form.editorMinimap" type="checkbox" /> Mostrar minimapa</label>
        <label class="check"><input :checked="!form.autocompleteOff" type="checkbox" @change="form.autocompleteOff = !($event.target as HTMLInputElement).checked" /> Sugerir enquanto digita (Ctrl+Espaço sempre funciona)</label>
        <label class="check"><input v-model="form.uppercaseKeywords" type="checkbox" /> Converter palavras-chave para maiúsculas ao digitar</label>
        <label class="check"><input :checked="!form.hideWarnings" type="checkbox" @change="form.hideWarnings = !($event.target as HTMLInputElement).checked" /> Mostrar avisos do servidor após cada instrução (SHOW WARNINGS)</label>
      </template>

      <template v-else-if="section === 'grid'">
        <div class="row">
          <label class="field grow">
            <span>Linhas por página de resultado</span>
            <input v-model.number="form.maxRows" class="input" type="number" min="10" max="50000" />
          </label>
          <label class="field grow">
            <span>Largura máxima automática das colunas (px)</span>
            <input v-model.number="form.gridMaxColumnWidth" class="input" type="number" min="80" max="2000" />
          </label>
        </div>
        <label class="check"><input v-model="form.gridThousands" type="checkbox" /> Separar milhares nos números (apenas exibição)</label>
        <p class="hint">
          Colunas inteiras podem ser exibidas como data (timestamp UNIX) pelo menu da coluna na grade de dados.
          {{ form.timestampColumns?.length ? `${form.timestampColumns.length} coluna(s) configurada(s).` : '' }}
          <button v-if="form.timestampColumns?.length" class="linkbtn" @click="form.timestampColumns = []">Limpar</button>
        </p>
      </template>

      <template v-else-if="section === 'log'">
        <div class="row">
          <label class="field">
            <span>Entradas mantidas no log de SQL</span>
            <input v-model.number="form.sqlLogLimit" class="input" type="number" min="100" max="100000" />
          </label>
        </div>
        <label class="check"><input v-model="form.historyEnabled" type="checkbox" /> Registrar histórico de consultas</label>
        <div class="row">
          <label class="field">
            <span>Máximo de entradas do histórico</span>
            <input v-model.number="form.historyLimit" class="input" type="number" min="10" max="100000" />
          </label>
          <div class="field end"><button class="btn" @click="clearHistory">Limpar histórico</button></div>
        </div>
        <p class="hint">
          Literais em instruções com senha (IDENTIFIED BY, PASSWORD…) são mascarados no histórico e no log.
          Arquivos em <code>{{ settings.info?.configDir }}</code>.
        </p>
      </template>

      <template v-else>
        <p class="hint">Clique no atalho e pressione a nova combinação. Delete remove o atalho; Esc cancela.</p>
        <div class="keys">
          <template v-for="g in groups" :key="g">
            <div class="kgroup">{{ g }}</div>
            <div v-for="a in shortcutActions.filter((x) => x.group === g)" :key="a.id" class="krow">
              <span class="klabel">{{ a.label }}</span>
              <input
                class="input mono kinput" readonly :value="recording === a.id ? 'pressione…' : keys.get(a.id) || '—'"
                :class="{ clash: clashes.has(keys.get(a.id) ?? ''), changed: form.shortcuts?.[a.id] !== undefined }" :aria-label="`Atalho: ${a.label}`"
                @focus="recording = a.id" @blur="recording = ''" @keydown="record($event, a.id)"
              />
              <button class="icon-btn" title="Restaurar padrão" aria-label="Restaurar padrão" :disabled="form.shortcuts?.[a.id] === undefined" @click="setKey(a.id, a.key)"><Icon name="undo" :size="12" /></button>
            </div>
          </template>
          <div class="kgroup">Editor e grades (fixos)</div>
          <div v-for="f in fixedShortcuts" :key="f.key + f.label" class="krow">
            <span class="klabel">{{ f.label }}</span>
            <code class="kfixed">{{ f.key }}</code>
          </div>
        </div>
      </template>
    </div>
    <template #footer>
      <button class="btn" @click="restoreDefaults">Restaurar padrões</button>
      <span class="spacer" />
      <button class="btn" @click="ui.preferencesOpen = false">Cancelar</button>
      <button class="btn primary" @click="save">Salvar</button>
    </template>
  </Modal>
</template>

<style scoped>
.sections {
  display: flex;
  gap: 4px;
  margin-bottom: 12px;
  border-bottom: 1px solid var(--border);
}
.sections button {
  padding: 6px 10px;
  border: none;
  border-bottom: 2px solid transparent;
  background: none;
  color: var(--text-muted);
  cursor: pointer;
}
.sections button.active {
  color: var(--text);
  border-bottom-color: var(--accent);
}
.form {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-height: 300px;
}
.row {
  display: flex;
  gap: 12px;
}
.grow {
  flex: 1;
}
.end {
  justify-content: flex-end;
}
.hint {
  margin: 0;
}
.spacer {
  flex: 1;
}
.warn {
  color: var(--warning);
}
code {
  font-family: var(--font-mono);
  font-size: 11px;
  user-select: text;
}
.linkbtn {
  border: none;
  background: none;
  color: var(--accent);
  cursor: pointer;
  padding: 0 4px;
}
.keys {
  max-height: 380px;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.kgroup {
  margin-top: 8px;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--text-muted);
}
.krow {
  display: flex;
  align-items: center;
  gap: 8px;
}
.klabel {
  flex: 1;
}
.kinput {
  width: 170px;
  cursor: pointer;
  text-align: center;
}
.kinput.changed {
  border-color: var(--accent);
}
.kinput.clash {
  border-color: var(--danger);
  color: var(--danger);
}
.kfixed {
  width: 210px;
  text-align: right;
  color: var(--text-muted);
}
</style>

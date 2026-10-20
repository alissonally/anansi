<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Modal from '../common/Modal.vue'
import Icon from '../common/Icon.vue'
import { useUiStore } from '../../stores/ui'
import { useConnectionsStore } from '../../stores/connections'
import { useMetadataStore } from '../../stores/metadata'
import { useSettingsStore } from '../../stores/settings'
import { toAppError } from '../../api/errors'
import type { Profile, ProfileView } from '../../api/types'
import { clone } from '../../utils/text'
import { globMatcher } from '../../utils/format'

const ui = useUiStore()
const conns = useConnectionsStore()
const meta = useMetadataStore()
const settings = useSettingsStore()

const st = computed(() => ui.databaseFilterDialog)
const profile = computed<ProfileView | null>(() => (st.value ? conns.byId.get(st.value.connectionId) ?? null : null))

const loading = ref(false)
const saving = ref(false)
const error = ref('')
const names = ref<string[]>([])
const picked = ref(new Set<string>())
const search = ref('')

watch(st, async (s) => {
  if (!s) return
  error.value = ''
  search.value = ''
  loading.value = true
  try {
    const dbs = await meta.databases(s.connectionId)
    const visible = dbs.filter((d) => settings.prefs.showSystemSchemas || !d.system)
    names.value = visible.map((d) => d.name).sort((a, b) => a.localeCompare(b))
    const matches = globMatcher(profile.value?.databaseFilter)
    picked.value = new Set(names.value.filter((n) => matches(n)))
  } catch (e) {
    error.value = toAppError(e).message
  } finally {
    loading.value = false
  }
}, { immediate: true })

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  return q ? names.value.filter((n) => n.toLowerCase().includes(q)) : names.value
})

function toggle(name: string) {
  const s = new Set(picked.value)
  if (s.has(name)) s.delete(name)
  else s.add(name)
  picked.value = s
}

function selectAll() {
  picked.value = new Set(names.value)
}

function selectNone() {
  picked.value = new Set()
}

async function save() {
  const p = profile.value
  if (!p) return
  saving.value = true
  try {
    const copy = clone(p) as Partial<ProfileView>
    delete copy.hasStoredPassword
    delete copy.hasStoredSSHSecret
    copy.databaseFilter = picked.value.size === names.value.length ? '' : [...picked.value].sort((a, b) => a.localeCompare(b)).join('; ')
    await conns.save(copy as Profile, null, null)
    ui.toast('success', `Filtro de databases salvo para "${p.name}".`)
    ui.databaseFilterDialog = null
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Modal v-if="st && profile" :title="`Filtrar databases — ${profile.name}`" :width="420" @close="ui.databaseFilterDialog = null">
    <div class="wrap">
      <p class="hint">Desmarque os databases que não devem aparecer na árvore desta conexão. Com todos marcados, novos databases criados depois aparecem automaticamente.</p>
      <div class="toolbar">
        <div class="search">
          <Icon name="search" :size="13" />
          <input v-model="search" class="filter" placeholder="Buscar…" aria-label="Buscar database" />
        </div>
        <button class="btn" type="button" @click="selectAll">Marcar todos</button>
        <button class="btn" type="button" @click="selectNone">Desmarcar todos</button>
      </div>
      <p v-if="error" class="err">{{ error }}</p>
      <p v-else-if="loading" class="faint">Carregando databases…</p>
      <p v-else-if="!names.length" class="faint">Nenhum database encontrado.</p>
      <div v-else class="list">
        <label v-for="n in filtered" :key="n" class="check">
          <input type="checkbox" :checked="picked.has(n)" @change="toggle(n)" />
          <span class="mono">{{ n }}</span>
        </label>
        <p v-if="!filtered.length" class="faint">Nenhum database corresponde à busca.</p>
      </div>
      <p class="faint count">{{ picked.size }} de {{ names.length }} selecionados</p>
    </div>
    <template #footer>
      <button class="btn" @click="ui.databaseFilterDialog = null">Cancelar</button>
      <button class="btn primary" :disabled="saving || loading || !!error" @click="save"><span v-if="saving" class="spinner" /> Salvar</button>
    </template>
  </Modal>
</template>

<style scoped>
.wrap {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 6px;
}
.search {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
  min-width: 0;
  padding: 0 8px;
  height: 28px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--bg-input);
  color: var(--text-faint);
}
.filter {
  flex: 1;
  min-width: 0;
  border: none;
  background: transparent;
  outline: none;
}
.list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 320px;
  overflow: auto;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 6px 8px;
}
.count {
  margin: 0;
}
.err {
  color: var(--danger);
  margin: 0;
}
</style>

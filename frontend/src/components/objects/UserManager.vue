<script setup lang="ts">
// HeidiSQL "User manager": accounts and roles, credentials, limits, TLS
// requirements, privileges per object (with column privileges) and roles.
import { computed, onMounted, ref, watch } from 'vue'
import Icon from '../common/Icon.vue'
import type { Tab } from '../../stores/tabs'
import { useTabsStore } from '../../stores/tabs'
import { useUiStore } from '../../stores/ui'
import { useConnectionsStore } from '../../stores/connections'
import { useMetadataStore } from '../../stores/metadata'
import { backend } from '../../api/backend'
import { toAppError } from '../../api/errors'
import type { Account, DatabaseInfo, Grant, GrantLevel, PrivilegeCatalog, UserDef } from '../../api/types'
import { reportResults, withConfirmation } from '../../composables/objectActions'

const props = defineProps<{ tab: Tab }>()
const ui = useUiStore()
const tabs = useTabsStore()
const conns = useConnectionsStore()
const meta = useMetadataStore()
const conn = computed(() => props.tab.connectionId ?? '')
const maria = computed(() => /mariadb/i.test(conns.state(conn.value).server?.flavor ?? ''))

const accounts = ref<Account[]>([])
const catalog = ref<PrivilegeCatalog | null>(null)
const filter = ref('')
const loading = ref(false)
const error = ref('')
const def = ref<UserDef | null>(null)
const original = ref('')
const selKey = ref('')
const grantIdx = ref(0)
const preview = ref<string[]>([])
const previewError = ref('')
const showPw = ref(false)
const section = ref<'login' | 'privileges' | 'roles' | 'limits'>('privileges')
const dbs = ref<string[]>([])

const acctKey = (a: { user: string; host: string; isRole: boolean }) => `${a.isRole ? 'r' : 'u'}:${a.user}@${a.host}`
const shown = computed(() => {
  const f = filter.value.trim().toLowerCase()
  return accounts.value.filter((a) => !f || `${a.user}@${a.host}`.toLowerCase().includes(f))
})
const roles = computed(() => accounts.value.filter((a) => a.isRole))
const roleName = (a: Account) => (maria.value ? a.user : `${a.user}@${a.host}`)
const dirty = computed(() => !!def.value && JSON.stringify(def.value) !== original.value)
watch(dirty, (d) => {
  const t = tabs.find(props.tab.id)
  if (t) t.objectDirty = d
})

async function reload() {
  loading.value = true
  error.value = ''
  try {
    accounts.value = await backend.listAccounts(conn.value)
    catalog.value ??= await backend.privilegeCatalog(conn.value)
  } catch (e) {
    error.value = toAppError(e).message
  } finally {
    loading.value = false
  }
}
onMounted(async () => {
  await reload()
  dbs.value = (await meta.databases(conn.value).catch(() => [] as DatabaseInfo[])).map((d) => d.name)
})

async function confirmDiscard(): Promise<boolean> {
  if (!dirty.value) return true
  return ui.confirm({ title: 'Descartar alterações?', message: 'As alterações nesta conta não foram salvas.', confirmLabel: 'Descartar', danger: true })
}

function setDef(u: UserDef) {
  u.grants ??= []
  u.roles ??= []
  u.defaultRoles ??= []
  for (const g of u.grants) {
    g.privileges ??= []
    g.columns ??= {}
  }
  def.value = u
  original.value = JSON.stringify(u)
  grantIdx.value = 0
}

async function select(a: Account) {
  if (selKey.value === acctKey(a) || !(await confirmDiscard())) return
  try {
    setDef(await backend.loadAccount(conn.value, a.user, a.host, a.isRole))
    selKey.value = acctKey(a)
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}

function blank(isRole: boolean): UserDef {
  return {
    user: '', host: isRole ? (maria.value ? '' : '%') : '%', origUser: '', origHost: '', isRole, password: '', plugin: '', require: 'NONE',
    cipher: '', issuer: '', subject: '', maxQueries: 0, maxUpdates: 0, maxConnections: 0, maxUserConnections: 0, locked: false,
    passwordExpire: '', expireDays: 0, grants: [], roles: [], defaultRoles: [],
  }
}
async function create(isRole: boolean) {
  if (!(await confirmDiscard())) return
  setDef(blank(isRole))
  original.value = ''
  selKey.value = ''
  section.value = 'login'
}
async function duplicate() {
  if (!def.value || !(await confirmDiscard())) return
  const copy: UserDef = JSON.parse(JSON.stringify(def.value))
  copy.user += '_copia'
  copy.origUser = copy.origHost = ''
  copy.password = ''
  setDef(copy)
  original.value = ''
  selKey.value = ''
  section.value = 'login'
}

async function drop() {
  const u = def.value
  if (!u || !u.origUser) return
  try {
    await withConfirmation((c) => backend.dropAccount(conn.value, u.origUser, u.origHost, u.isRole, c))
    ui.toast('success', `Conta ${u.origUser}@${u.origHost} excluída.`)
    def.value = null
    original.value = ''
    selKey.value = ''
    await reload()
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}

let timer: number | undefined
watch(def, (u) => {
  window.clearTimeout(timer)
  if (!u || !dirty.value) {
    preview.value = []
    previewError.value = ''
    return
  }
  timer = window.setTimeout(async () => {
    try {
      preview.value = await backend.previewAccount(conn.value, u)
      previewError.value = ''
    } catch (e) {
      preview.value = []
      previewError.value = toAppError(e).message
    }
  }, 250)
}, { deep: true })

const saving = ref(false)
async function save() {
  const u = def.value
  if (!u) return
  saving.value = true
  try {
    const res = await withConfirmation((c) => backend.saveAccount(conn.value, u, c))
    if (!res) return
    if (reportResults(res, 'Conta salva.')) return
    await reload()
    const fresh = await backend.loadAccount(conn.value, u.user, u.host, u.isRole)
    setDef(fresh)
    selKey.value = acctKey(fresh)
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  } finally {
    saving.value = false
  }
}
async function discard() {
  if (!def.value) return
  if (!original.value) def.value = null
  else setDef(JSON.parse(original.value))
}

async function genPassword() {
  if (!def.value) return
  def.value.password = await backend.randomPassword(16)
  showPw.value = true
}

// ---- grants ----
const levelLabel: Record<GrantLevel, string> = { global: 'Global (*.*)', database: 'Database', table: 'Tabela', routine: 'Rotina' }
function grantLabel(g: Grant): string {
  switch (g.level) {
    case 'global': return 'Global (*.*)'
    case 'database': return `${g.database}.*`
    case 'table': return `${g.database}.${g.name}`
    default: return `${g.routineType} ${g.database}.${g.name}`
  }
}
const grant = computed(() => def.value?.grants[grantIdx.value])
const levelPrivs = computed(() => {
  const c = catalog.value
  const g = grant.value
  if (!c || !g) return []
  const list = c[g.level] ?? []
  return [...new Set([...list, ...g.privileges.filter((p) => p !== 'ALL PRIVILEGES')])]
})
function hasPriv(p: string) {
  return !!grant.value?.privileges.includes(p)
}
function togglePriv(p: string) {
  const g = grant.value
  if (!g) return
  if (g.privileges.includes('ALL PRIVILEGES')) g.privileges = [...levelPrivs.value]
  g.privileges = g.privileges.includes(p) ? g.privileges.filter((x) => x !== p) : [...g.privileges, p].sort()
}
function setAll(on: boolean) {
  if (grant.value) grant.value.privileges = on ? ['ALL PRIVILEGES'] : []
}
const newGrant = ref<{ level: GrantLevel; database: string; name: string; routineType: string }>({ level: 'database', database: '', name: '', routineType: 'PROCEDURE' })
const objNames = ref<string[]>([])
watch(() => [newGrant.value.level, newGrant.value.database] as const, async ([lvl, db]) => {
  objNames.value = []
  if (!db || (lvl !== 'table' && lvl !== 'routine')) return
  if (lvl === 'table') objNames.value = (await meta.tables(conn.value, db).catch(() => [])).map((t) => t.name)
  else objNames.value = (await meta.routines(conn.value, db).catch(() => [])).map((r) => r.name)
})
function addGrant() {
  const u = def.value
  const n = newGrant.value
  if (!u) return
  if (n.level !== 'global' && !n.database.trim()) return ui.toast('warning', 'Informe o database (aceita curingas % e _).')
  if ((n.level === 'table' || n.level === 'routine') && !n.name.trim()) return ui.toast('warning', 'Informe o objeto.')
  const g: Grant = { level: n.level, database: n.level === 'global' ? '' : n.database.trim(), name: n.level === 'table' || n.level === 'routine' ? n.name.trim() : '',
    routineType: n.level === 'routine' ? n.routineType : '', privileges: [], columns: {}, grantOption: false }
  const dup = u.grants.findIndex((x) => grantLabel(x) === grantLabel(g))
  if (dup >= 0) {
    grantIdx.value = dup
    return
  }
  u.grants.push(g)
  grantIdx.value = u.grants.length - 1
}
function removeGrant(i: number) {
  def.value?.grants.splice(i, 1)
  grantIdx.value = Math.max(0, Math.min(grantIdx.value, (def.value?.grants.length ?? 1) - 1))
}
const colPrivs = computed(() => (catalog.value?.column?.length ? catalog.value.column : ['SELECT', 'INSERT', 'UPDATE', 'REFERENCES']))
function colText(p: string) {
  return (grant.value?.columns[p] ?? []).join(', ')
}
function setCols(p: string, v: string) {
  const g = grant.value
  if (!g) return
  const list = v.split(',').map((s) => s.trim()).filter(Boolean)
  if (list.length) g.columns[p] = list
  else delete g.columns[p]
}

// ---- roles ----
function hasRole(r: string) {
  return !!def.value?.roles.some((x) => x.role === r)
}
function toggleRole(r: string) {
  const u = def.value
  if (!u) return
  if (hasRole(r)) {
    u.roles = u.roles.filter((x) => x.role !== r)
    u.defaultRoles = u.defaultRoles.filter((x) => x !== r)
  } else u.roles.push({ role: r, adminOption: false })
}
function toggleDefault(r: string) {
  const u = def.value
  if (!u) return
  if (u.defaultRoles.includes(r)) u.defaultRoles = u.defaultRoles.filter((x) => x !== r)
  else u.defaultRoles = maria.value ? [r] : [...u.defaultRoles, r]
}

// ---- own password ----
async function changeOwn() {
  const r = await ui.prompt({ title: 'Alterar a minha senha', message: 'Nova senha da conta usada nesta conexão (ALTER USER CURRENT_USER()).', label: 'Nova senha', password: true, confirmLabel: 'Alterar' })
  if (!r?.value) return
  try {
    await backend.changeOwnPassword(conn.value, r.value)
    ui.toast('success', 'Senha alterada. Reconectando…')
    await conns.disconnect(conn.value)
    await conns.connect(conn.value)
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}
</script>

<template>
  <div class="users">
    <aside class="side">
      <div class="bar">
        <button class="btn small" title="Novo usuário" @click="create(false)"><Icon name="plus" :size="11" /> Usuário</button>
        <button v-if="catalog?.roles" class="btn small" title="Nova role" @click="create(true)"><Icon name="plus" :size="11" /> Role</button>
        <button class="icon-btn" title="Atualizar" aria-label="Atualizar" @click="reload"><Icon name="refresh" :size="13" /></button>
      </div>
      <div class="search"><Icon name="search" :size="12" /><input v-model="filter" placeholder="Filtrar contas" aria-label="Filtrar contas" /></div>
      <div class="accts" role="listbox" aria-label="Contas">
        <div v-for="a in shown" :key="acctKey(a)" class="acct" role="option" :aria-selected="selKey === acctKey(a)" :class="{ on: selKey === acctKey(a) }" @click="select(a)">
          <Icon :name="a.isRole ? 'shield' : 'user'" :size="13" />
          <span class="mono ellipsis">{{ a.user || '(anônimo)' }}<span class="faint">@{{ a.host }}</span></span>
          <Icon v-if="a.locked" name="lock" :size="11" title="bloqueada" />
          <Icon v-if="a.emptyPassword && !a.isRole" name="alert" :size="11" class="warn" title="sem senha" />
        </div>
        <p v-if="loading" class="faint">Carregando…</p>
        <p v-else-if="error" class="err">{{ error }}</p>
      </div>
      <button class="btn small own" @click="changeOwn"><Icon name="key" :size="11" /> Alterar a minha senha</button>
    </aside>

    <section v-if="def" class="editor">
      <header class="head">
        <Icon :name="def.isRole ? 'shield' : 'user'" :size="15" />
        <strong class="mono">{{ def.origUser ? `${def.origUser}@${def.origHost}` : def.isRole ? 'Nova role' : 'Novo usuário' }}</strong>
        <span v-if="dirty" class="faint">· alterado</span>
        <span class="spacer" />
        <button v-if="def.origUser" class="btn small" @click="duplicate"><Icon name="copy" :size="11" /> Duplicar</button>
        <button v-if="def.origUser" class="btn small danger" @click="drop"><Icon name="trash" :size="11" /> Excluir</button>
      </header>
      <nav class="subtabs" role="tablist">
        <button v-for="s in ([['login', 'Credenciais'], ['privileges', 'Privilégios'], ['roles', 'Roles'], ['limits', 'Limites e segurança']] as const)" :key="s[0]" role="tab"
          :aria-selected="section === s[0]" :class="{ active: section === s[0] }" :disabled="s[0] === 'roles' && !catalog?.roles" @click="section = s[0]">{{ s[1] }}</button>
      </nav>

      <div v-if="section === 'login'" class="pane form">
        <label class="field"><span>{{ def.isRole ? 'Nome da role' : 'Usuário' }}</span><input v-model="def.user" class="input mono" /></label>
        <label v-if="!(def.isRole && maria)" class="field"><span>Host (% = qualquer, aceita curingas)</span>
          <input v-model="def.host" class="input mono" list="um-hosts" />
          <datalist id="um-hosts"><option value="%" /><option value="localhost" /><option value="127.0.0.1" /><option value="::1" /><option value="192.168.%" /></datalist>
        </label>
        <template v-if="!def.isRole">
          <label class="field"><span>Senha {{ def.origUser ? '(em branco = manter a atual)' : '' }}</span>
            <div class="row">
              <input v-model="def.password" :type="showPw ? 'text' : 'password'" class="input mono grow" autocomplete="new-password" />
              <button class="btn small" :aria-label="showPw ? 'Ocultar' : 'Mostrar'" @click="showPw = !showPw">{{ showPw ? 'ocultar' : 'mostrar' }}</button>
              <button class="btn small" @click="genPassword">gerar</button>
            </div>
          </label>
          <label class="field"><span>Plugin de autenticação</span>
            <select v-model="def.plugin" class="select"><option value="">(padrão do servidor)</option><option v-for="p in catalog?.plugins ?? []" :key="p" :value="p">{{ p }}</option></select>
          </label>
          <label class="check"><input v-model="def.locked" type="checkbox" /> conta bloqueada (ACCOUNT LOCK)</label>
          <label class="field"><span>Expiração da senha</span>
            <div class="row">
              <select v-model="def.passwordExpire" class="select">
                <option value="">padrão do servidor</option><option value="NOW">expirada (trocar no próximo login)</option><option value="NEVER">nunca</option><option value="INTERVAL">a cada N dias</option>
              </select>
              <input v-if="def.passwordExpire === 'INTERVAL'" v-model.number="def.expireDays" type="number" min="1" class="input days" aria-label="Dias" />
            </div>
          </label>
        </template>
      </div>

      <div v-else-if="section === 'privileges'" class="pane privs">
        <div class="glist">
          <div v-for="(g, i) in def.grants" :key="i" class="gitem" :class="{ on: i === grantIdx }" @click="grantIdx = i">
            <Icon :name="g.level === 'global' ? 'server' : g.level === 'database' ? 'database' : g.level === 'table' ? 'table' : 'fn'" :size="12" />
            <span class="mono ellipsis">{{ grantLabel(g) }}</span>
            <span class="faint small">{{ g.privileges.length + Object.keys(g.columns).length }}</span>
            <button class="icon-btn mini" aria-label="Remover objeto" @click.stop="removeGrant(i)"><Icon name="x" :size="10" /></button>
          </div>
          <p v-if="!def.grants.length" class="faint">Sem privilégios.</p>
          <div class="add">
            <div class="lbl">Adicionar objeto</div>
            <select v-model="newGrant.level" class="select"><option v-for="(l, k) in levelLabel" :key="k" :value="k">{{ l }}</option></select>
            <template v-if="newGrant.level !== 'global'">
              <input v-model="newGrant.database" class="input mono" list="um-dbs" placeholder="database" />
              <datalist id="um-dbs"><option v-for="d in dbs" :key="d" :value="d" /></datalist>
            </template>
            <select v-if="newGrant.level === 'routine'" v-model="newGrant.routineType" class="select"><option>PROCEDURE</option><option>FUNCTION</option></select>
            <template v-if="newGrant.level === 'table' || newGrant.level === 'routine'">
              <input v-model="newGrant.name" class="input mono" list="um-objs" placeholder="nome" />
              <datalist id="um-objs"><option v-for="o in objNames" :key="o" :value="o" /></datalist>
            </template>
            <button class="btn small" @click="addGrant"><Icon name="plus" :size="11" /> Adicionar</button>
          </div>
        </div>
        <div v-if="grant" class="gedit">
          <div class="row">
            <strong class="mono">{{ grantLabel(grant) }}</strong>
            <span class="spacer" />
            <button class="linkbtn" @click="setAll(true)">todos (ALL)</button>
            <button class="linkbtn" @click="setAll(false)">nenhum</button>
          </div>
          <p v-if="grant.privileges.includes('ALL PRIVILEGES')" class="small">ALL PRIVILEGES — clique num privilégio para escolher individualmente.</p>
          <div class="pgrid">
            <label v-for="p in levelPrivs" :key="p" class="check small"><input type="checkbox" :checked="hasPriv(p) || grant.privileges.includes('ALL PRIVILEGES')" @change="togglePriv(p)" /> {{ p }}</label>
          </div>
          <label class="check"><input v-model="grant.grantOption" type="checkbox" /> WITH GRANT OPTION (pode repassar estes privilégios)</label>
          <fieldset v-if="grant.level === 'table'">
            <legend>Privilégios por coluna (nomes separados por vírgula)</legend>
            <label v-for="p in colPrivs" :key="p" class="field inline"><span>{{ p }}</span><input :value="colText(p)" class="input mono" @change="setCols(p, ($event.target as HTMLInputElement).value)" /></label>
          </fieldset>
        </div>
      </div>

      <div v-else-if="section === 'roles'" class="pane form">
        <p class="faint small">Roles concedidas a esta conta. {{ maria ? 'No MariaDB há uma única role padrão.' : 'Roles padrão são ativadas no login.' }}</p>
        <div v-for="r in roles.filter((x) => roleName(x) !== (maria ? def!.user : def!.user + '@' + def!.host))" :key="acctKey(r)" class="row">
          <label class="check grow"><input type="checkbox" :checked="hasRole(roleName(r))" @change="toggleRole(roleName(r))" /> <span class="mono">{{ roleName(r) }}</span></label>
          <template v-if="hasRole(roleName(r))">
            <label class="check small"><input v-model="def.roles.find((x) => x.role === roleName(r))!.adminOption" type="checkbox" /> ADMIN OPTION</label>
            <label v-if="!def.isRole" class="check small"><input type="checkbox" :checked="def.defaultRoles.includes(roleName(r))" @change="toggleDefault(roleName(r))" /> padrão</label>
          </template>
        </div>
        <p v-if="!roles.length" class="faint">Nenhuma role no servidor.</p>
      </div>

      <div v-else class="pane form">
        <fieldset>
          <legend>Limites por hora (0 = sem limite)</legend>
          <div class="grid2">
            <label class="field"><span>Consultas</span><input v-model.number="def.maxQueries" type="number" min="0" class="input" /></label>
            <label class="field"><span>Atualizações</span><input v-model.number="def.maxUpdates" type="number" min="0" class="input" /></label>
            <label class="field"><span>Conexões</span><input v-model.number="def.maxConnections" type="number" min="0" class="input" /></label>
            <label class="field"><span>Conexões simultâneas</span><input v-model.number="def.maxUserConnections" type="number" min="0" class="input" /></label>
          </div>
        </fieldset>
        <fieldset v-if="!def.isRole">
          <legend>Exigir conexão segura (REQUIRE)</legend>
          <select v-model="def.require" class="select">
            <option value="NONE">não exigir</option><option value="SSL">SSL/TLS</option><option value="X509">certificado X509 válido</option><option value="SPECIFIED">certificado específico</option>
          </select>
          <template v-if="def.require === 'SPECIFIED'">
            <label class="field"><span>Cipher</span><input v-model="def.cipher" class="input mono" /></label>
            <label class="field"><span>Emissor (issuer)</span><input v-model="def.issuer" class="input mono" placeholder="/C=BR/O=..." /></label>
            <label class="field"><span>Assunto (subject)</span><input v-model="def.subject" class="input mono" /></label>
          </template>
        </fieldset>
      </div>

      <footer class="foot">
        <pre class="sql mono selectable" :class="{ err: previewError }">{{ previewError || (dirty ? preview.join(';\n') || '(nada a alterar)' : '') }}</pre>
        <div class="actions">
          <button class="btn" :disabled="!dirty" @click="discard">Descartar</button>
          <button class="btn primary" :disabled="!dirty || saving || !!previewError" @click="save"><span v-if="saving" class="spinner" /> Salvar</button>
        </div>
      </footer>
    </section>
    <section v-else class="empty faint">
      <Icon name="user" :size="28" />
      <p>Selecione uma conta ou crie um usuário.</p>
    </section>
  </div>
</template>

<style scoped>
.users {
  display: grid;
  grid-template-columns: 260px 1fr;
  height: 100%;
  min-height: 0;
}
.side {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px;
  border-right: 1px solid var(--border);
  min-height: 0;
}
.bar,
.row,
.head {
  display: flex;
  align-items: center;
  gap: 6px;
}
.search {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 8px;
  height: 26px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--bg-input);
}
.search input {
  flex: 1;
  border: none;
  background: none;
  outline: none;
  color: var(--text);
}
.accts {
  flex: 1;
  overflow: auto;
}
.acct,
.gitem {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 3px 6px;
  border-radius: var(--radius-sm);
  cursor: pointer;
}
.acct:hover,
.gitem:hover {
  background: var(--bg-hover);
}
.acct.on,
.gitem.on {
  background: var(--bg-active);
}
.ellipsis {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
}
.faint {
  color: var(--text-faint);
}
.warn {
  color: var(--warning);
}
.err {
  color: var(--danger);
}
.own {
  align-self: flex-start;
}
.editor {
  display: flex;
  flex-direction: column;
  min-height: 0;
  padding: 8px 12px;
  gap: 6px;
}
.spacer {
  flex: 1;
}
.subtabs {
  display: flex;
  gap: 2px;
  border-bottom: 1px solid var(--border);
}
.subtabs button {
  border: none;
  background: none;
  padding: 6px 12px;
  cursor: pointer;
  color: var(--text-muted);
  border-bottom: 2px solid transparent;
}
.subtabs button.active {
  color: var(--text);
  border-bottom-color: var(--accent);
}
.subtabs button:disabled {
  opacity: 0.4;
  cursor: default;
}
.pane {
  flex: 1;
  min-height: 0;
  overflow: auto;
}
.form {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-width: 560px;
}
.grow {
  flex: 1;
}
.days {
  width: 90px;
}
.privs {
  display: grid;
  grid-template-columns: 280px 1fr;
  gap: 12px;
}
.glist {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-height: 0;
}
.add {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-top: 10px;
  padding-top: 8px;
  border-top: 1px solid var(--border);
}
.gedit {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.pgrid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(190px, 1fr));
  gap: 3px 10px;
}
.lbl {
  font-size: 12px;
  color: var(--text-muted);
}
.small {
  font-size: 12px;
  margin: 0;
}
.mini {
  width: 18px;
  height: 18px;
}
.linkbtn {
  border: none;
  background: none;
  color: var(--accent);
  cursor: pointer;
  font-size: 12px;
}
fieldset {
  display: flex;
  flex-direction: column;
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
.field.inline {
  flex-direction: row;
  align-items: center;
  gap: 8px;
}
.field.inline span {
  width: 100px;
}
.field.inline input {
  flex: 1;
}
.grid2 {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 6px 12px;
}
.foot {
  display: flex;
  gap: 10px;
  align-items: flex-end;
  border-top: 1px solid var(--border);
  padding-top: 6px;
}
.sql {
  flex: 1;
  margin: 0;
  padding: 6px 8px;
  min-height: 48px;
  max-height: 120px;
  overflow: auto;
  background: var(--bg-sunken);
  border-radius: var(--radius-sm);
  font-size: 11.5px;
  white-space: pre-wrap;
}
.sql.err {
  color: var(--danger);
}
.actions {
  display: flex;
  gap: 6px;
}
.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
}
</style>

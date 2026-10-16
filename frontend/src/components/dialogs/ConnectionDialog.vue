<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Modal from '../common/Modal.vue'
import Icon from '../common/Icon.vue'
import { useUiStore } from '../../stores/ui'
import { emptyProfile, environmentLabels, useConnectionsStore } from '../../stores/connections'
import { useSettingsStore } from '../../stores/settings'
import { backend } from '../../api/backend'
import { toAppError } from '../../api/errors'
import type { Profile, ProfileView, ServerInfo, TLSMode, UnknownHost } from '../../api/types'
import { clone } from '../../utils/text'

const ui = useUiStore()
const conns = useConnectionsStore()
const settings = useSettingsStore()

const form = ref<Profile>(emptyProfile())
const password = ref('')
const passwordTouched = ref(false)
const params = ref<{ key: string; value: string }[]>([])
const sshSecret = ref('')
const sshTouched = ref(false)
const useSocket = ref(false)
const section = ref<'general' | 'network' | 'security' | 'advanced'>('general')
const testing = ref(false)
const saving = ref(false)
const testResult = ref<{ ok: boolean; message: string; info?: ServerInfo } | null>(null)
const submitted = ref(false)

const editing = computed(() => (ui.connectionDialog.profileId ? conns.byId.get(ui.connectionDialog.profileId) ?? null : null))
const keyringAvailable = computed(() => settings.info?.keyringAvailable ?? false)

const tlsModes: { value: TLSMode; label: string; hint: string }[] = [
  { value: 'preferred', label: 'Preferencial', hint: 'Usa TLS quando o servidor oferece; caso contrário conecta sem criptografia (o estado real aparece na barra de status).' },
  { value: 'required', label: 'Obrigatório', hint: 'Exige TLS, sem verificar o certificado do servidor.' },
  { value: 'verify-ca', label: 'Verificar CA', hint: 'Exige TLS e valida a cadeia do certificado com a CA informada (sem verificar o nome do host).' },
  { value: 'verify-full', label: 'Verificar CA e host', hint: 'Exige TLS, valida a cadeia (CA informada ou do sistema) e o nome do host.' },
  { value: 'disabled', label: 'Desativado', hint: 'Nunca usa TLS. Não recomendado fora de ambientes locais.' },
]
const tlsHint = computed(() => tlsModes.find((m) => m.value === form.value.tls.mode)?.hint ?? '')

watch(
  () => ui.connectionDialog.open,
  (open) => {
    if (!open) return
    const src = editing.value
    if (src) {
      const copy: Partial<ProfileView> = clone(src)
      delete copy.hasStoredPassword
      delete copy.hasStoredSSHSecret
      form.value = { ...emptyProfile(), ...copy, tls: { ...emptyProfile().tls, ...copy.tls }, ssh: { ...emptyProfile().ssh, ...copy.ssh } }
    } else {
      form.value = emptyProfile()
    }
    params.value = Object.entries(form.value.params ?? {}).map(([key, value]) => ({ key, value }))
    password.value = ''
    passwordTouched.value = false
    sshSecret.value = ''
    sshTouched.value = false
    useSocket.value = !!form.value.socket
    testResult.value = null
    submitted.value = false
    section.value = 'general'
  },
  { immediate: true },
)

const errors = computed(() => {
  const e: Record<string, string> = {}
  const f = form.value
  if (!f.name.trim()) e.name = 'Informe um nome.'
  if (useSocket.value) {
    if (!f.socket.trim().startsWith('/')) e.socket = 'Informe o caminho absoluto do socket.'
  } else if (!f.host.trim()) e.host = 'Informe o host.'
  if (f.ssh.enabled) {
    if (!f.ssh.host.trim()) e.sshHost = 'Informe o servidor SSH.'
    if (!f.ssh.user.trim()) e.sshUser = 'Informe o usuário SSH.'
    if (f.ssh.auth === 'key' && !f.ssh.keyFile.trim()) e.sshKey = 'Escolha a chave privada.'
    if (useSocket.value) e.socket = 'Socket Unix e túnel SSH não podem ser usados juntos.'
  }
  if (!Number.isInteger(f.port) || f.port < 1 || f.port > 65535) e.port = 'Porta inválida.'
  if (!f.user.trim()) e.user = 'Informe o usuário.'
  if (f.tls.mode === 'verify-ca' && !f.tls.caFile?.trim()) e.caFile = 'Informe o arquivo da CA.'
  if (!!f.tls.certFile?.trim() !== !!f.tls.keyFile?.trim()) e.certFile = 'Certificado e chave devem ser informados juntos.'
  if (f.allowCleartextPasswords && (f.tls.mode === 'disabled' || f.tls.mode === 'preferred')) e.cleartext = 'Exige TLS obrigatório.'
  return e
})
const valid = computed(() => Object.keys(errors.value).length === 0)

const envColors: Record<Profile['environment'], string> = { local: '#3fb27f', development: '#4a8cf0', staging: '#d99a26', production: '#e5484d' }
// Empty color means "use the environment color".
const colorValue = computed({
  get: () => form.value.color || envColors[form.value.environment],
  set: (v: string) => (form.value.color = v),
})

function payload(): Profile {
  const p = clone(form.value)
  p.params = {}
  for (const { key, value } of params.value) {
    if (key.trim()) p.params[key.trim()] = value
  }
  p.port = Number(p.port)
  p.ssh.port = Number(p.ssh.port) || 22
  if (!useSocket.value) p.socket = ''
  return p
}

/** null keeps the stored SSH secret. */
function sshPayload(): string | null {
  if (!form.value.ssh.enabled || form.value.ssh.auth === 'agent') return null
  if (editing.value && !sshTouched.value) return null
  return sshSecret.value
}

async function pickKey() {
  try {
    const path = await backend.pickKeyFile()
    if (path) form.value.ssh.keyFile = path
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  }
}

function sectionOf(e: Record<string, string>): typeof section.value {
  if (e.caFile || e.certFile || e.cleartext) return 'security'
  if (e.socket || e.sshHost || e.sshUser || e.sshKey) return 'network'
  return 'general'
}

/** null keeps the stored password; a string replaces it. */
function passwordPayload(): string | null {
  if (editing.value && !passwordTouched.value) return null
  return password.value
}

async function test() {
  submitted.value = true
  if (!valid.value) return
  testing.value = true
  testResult.value = null
  try {
    const info = await backend.testConnection(payload(), passwordPayload(), sshPayload())
    testResult.value = { ok: true, info, message: `${info.flavor} ${info.version} — usuário ${info.user}` }
  } catch (e) {
    const err = toAppError(e)
    let message = err.code === 'password_required' ? 'Informe a senha para testar.' : err.message
    if (err.code === 'ssh_secret_required') message = 'Informe a senha (ou frase secreta) SSH para testar.'
    if (err.code === 'ssh_host_unknown') {
      const h = err.details as UnknownHost
      if (await ui.confirm({ title: 'Servidor SSH desconhecido', message: `Confira a impressão digital de ${h.host} com o administrador antes de confiar.`, items: [{ text: `${h.keyType} ${h.fingerprint}` }], confirmLabel: 'Confiar' })) {
        await backend.trustSshHost(h.host, h.fingerprint)
        testing.value = false
        return test()
      }
    }
    testResult.value = { ok: false, message }
  } finally {
    testing.value = false
  }
}

async function save(connectAfter: boolean) {
  submitted.value = true
  if (!valid.value) {
    section.value = sectionOf(errors.value)
    return
  }
  saving.value = true
  try {
    const p = await conns.save(payload(), passwordPayload(), sshPayload())
    ui.closeConnectionDialog()
    ui.toast('success', `Conexão "${p.name}" salva.`)
    if (connectAfter) await conns.connect(p.id)
  } catch (e) {
    ui.toast('error', toAppError(e).message)
  } finally {
    saving.value = false
  }
}

function onKindChange() {
  if (form.value.port === 3306 || !form.value.port) form.value.port = 3306
}
</script>

<template>
  <Modal v-if="ui.connectionDialog.open" :title="editing ? `Editar conexão — ${editing.name}` : 'Nova conexão'" :width="620" :close-on-backdrop="false" @close="ui.closeConnectionDialog()">
    <nav class="sections" role="tablist">
      <button v-for="s in ([['general', 'Geral'], ['network', 'Rede / SSH'], ['security', 'Segurança / TLS'], ['advanced', 'Avançado']] as const)" :key="s[0]" role="tab" :aria-selected="section === s[0]" :class="{ active: section === s[0] }" @click="section = s[0]">
        {{ s[1] }}
      </button>
    </nav>

    <form class="form" autocomplete="off" @submit.prevent="save(false)">
      <template v-if="section === 'general'">
        <div class="row">
          <label class="field grow">
            <span>Nome</span>
            <input v-model="form.name" class="input" :class="{ invalid: submitted && errors.name }" placeholder="MySQL Local" maxlength="100" autofocus />
            <small v-if="submitted && errors.name" class="err">{{ errors.name }}</small>
          </label>
          <label class="field">
            <span>Tipo</span>
            <select v-model="form.kind" class="select" @change="onKindChange">
              <option value="mysql">MySQL</option>
              <option value="mariadb">MariaDB</option>
            </select>
          </label>
        </div>
        <div class="row">
          <label class="field grow">
            <span>Ambiente</span>
            <select v-model="form.environment" class="select">
              <option v-for="(label, key) in environmentLabels" :key="key" :value="key">{{ label }}</option>
            </select>
          </label>
          <label class="field">
            <span>Cor</span>
            <div class="color-row">
              <input v-model="colorValue" type="color" class="color" aria-label="Cor da conexão" />
              <button v-if="form.color" type="button" class="icon-btn" title="Usar a cor do ambiente" aria-label="Usar a cor do ambiente" @click="form.color = ''"><Icon name="x" :size="12" /></button>
            </div>
          </label>
        </div>
        <p v-if="form.environment === 'production'" class="callout danger">
          <Icon name="shield" :size="14" /> Conexões de produção pedem confirmação antes de qualquer alteração de dados ou estrutura e são destacadas em vermelho.
        </p>
        <div class="row">
          <label class="field grow">
            <span>Host</span>
            <input v-model="form.host" class="input" :class="{ invalid: submitted && errors.host }" placeholder="127.0.0.1" spellcheck="false" :disabled="useSocket" :title="useSocket ? 'Usando socket Unix (aba Rede / SSH)' : undefined" />
            <small v-if="submitted && errors.host" class="err">{{ errors.host }}</small>
          </label>
          <label class="field port">
            <span>Porta</span>
            <input v-model.number="form.port" class="input" :class="{ invalid: submitted && errors.port }" type="number" min="1" max="65535" :disabled="useSocket" />
          </label>
        </div>
        <div class="row">
          <label class="field grow">
            <span>Usuário</span>
            <input v-model="form.user" class="input" :class="{ invalid: submitted && errors.user }" spellcheck="false" />
            <small v-if="submitted && errors.user" class="err">{{ errors.user }}</small>
          </label>
          <label class="field grow">
            <span>Senha</span>
            <input
              v-model="password" class="input" type="password" autocomplete="new-password"
              :placeholder="editing?.hasStoredPassword && !passwordTouched ? '•••••••• (inalterada)' : ''"
              @input="passwordTouched = true"
            />
          </label>
        </div>
        <label class="check">
          <input v-model="form.savePassword" type="checkbox" :disabled="!keyringAvailable" />
          Salvar senha no keyring do sistema
        </label>
        <p v-if="!keyringAvailable" class="hint">
          O keyring do sistema (Secret Service) não está disponível. A senha será pedida a cada conexão e mantida apenas na memória.
        </p>
        <label class="field">
          <span>Database inicial (opcional)</span>
          <input v-model="form.database" class="input" spellcheck="false" />
        </label>
      </template>

      <template v-else-if="section === 'network'">
        <label class="check">
          <input v-model="useSocket" type="checkbox" :disabled="form.ssh.enabled" />
          Conectar por socket Unix (servidor na mesma máquina)
        </label>
        <label v-if="useSocket" class="field">
          <span>Caminho do socket</span>
          <input v-model="form.socket" class="input mono" :class="{ invalid: submitted && errors.socket }" placeholder="/var/lib/mysql/mysql.sock" spellcheck="false" />
          <small v-if="submitted && errors.socket" class="err">{{ errors.socket }}</small>
        </label>
        <label class="check">
          <input v-model="form.ssh.enabled" type="checkbox" :disabled="useSocket" />
          Usar túnel SSH
        </label>
        <template v-if="form.ssh.enabled">
          <p class="hint">O host e a porta da aba Geral são vistos a partir do servidor SSH (ex.: 127.0.0.1:3306). Nenhuma porta local é aberta.</p>
          <div class="row">
            <label class="field grow">
              <span>Servidor SSH</span>
              <input v-model="form.ssh.host" class="input" :class="{ invalid: submitted && errors.sshHost }" placeholder="bastion.exemplo.com" spellcheck="false" />
              <small v-if="submitted && errors.sshHost" class="err">{{ errors.sshHost }}</small>
            </label>
            <label class="field port">
              <span>Porta</span>
              <input v-model.number="form.ssh.port" class="input" type="number" min="1" max="65535" />
            </label>
          </div>
          <div class="row">
            <label class="field grow">
              <span>Usuário SSH</span>
              <input v-model="form.ssh.user" class="input" :class="{ invalid: submitted && errors.sshUser }" spellcheck="false" />
              <small v-if="submitted && errors.sshUser" class="err">{{ errors.sshUser }}</small>
            </label>
            <label class="field grow">
              <span>Autenticação</span>
              <select v-model="form.ssh.auth" class="select">
                <option value="password">senha</option>
                <option value="key">chave privada</option>
                <option value="agent">agente SSH (ssh-agent)</option>
              </select>
            </label>
          </div>
          <label v-if="form.ssh.auth === 'key'" class="field">
            <span>Chave privada</span>
            <div class="color-row">
              <input v-model="form.ssh.keyFile" class="input mono grow" :class="{ invalid: submitted && errors.sshKey }" placeholder="~/.ssh/id_ed25519" spellcheck="false" />
              <button type="button" class="btn small" @click="pickKey">Escolher…</button>
            </div>
            <small v-if="submitted && errors.sshKey" class="err">{{ errors.sshKey }}</small>
          </label>
          <template v-if="form.ssh.auth !== 'agent'">
            <label class="field">
              <span>{{ form.ssh.auth === 'key' ? 'Frase secreta da chave (se houver)' : 'Senha SSH' }}</span>
              <input
                v-model="sshSecret" class="input" type="password" autocomplete="new-password"
                :placeholder="editing?.hasStoredSSHSecret && !sshTouched ? '•••••••• (inalterada)' : ''" @input="sshTouched = true"
              />
            </label>
            <label class="check">
              <input v-model="form.ssh.saveSecret" type="checkbox" :disabled="!keyringAvailable" />
              Salvar no keyring do sistema
            </label>
          </template>
          <p class="hint">A chave do servidor SSH é verificada (~/.ssh/known_hosts e a lista do aplicativo); um servidor novo pede confirmação da impressão digital.</p>
        </template>
        <label class="field">
          <span>Keep-alive: enviar ping a cada (s, 0 = desligado)</span>
          <input v-model.number="form.keepAliveSeconds" class="input" type="number" min="0" max="3600" />
        </label>
      </template>

      <template v-else-if="section === 'security'">
        <label class="field">
          <span>Modo TLS</span>
          <select v-model="form.tls.mode" class="select">
            <option v-for="m in tlsModes" :key="m.value" :value="m.value">{{ m.label }}</option>
          </select>
          <small class="hint">{{ tlsHint }}</small>
        </label>
        <template v-if="form.tls.mode !== 'disabled'">
          <label class="field">
            <span>Arquivo da CA (PEM)</span>
            <input v-model="form.tls.caFile" class="input mono" :class="{ invalid: submitted && errors.caFile }" placeholder="/etc/pki/tls/certs/ca.pem" spellcheck="false" />
            <small v-if="submitted && errors.caFile" class="err">{{ errors.caFile }}</small>
          </label>
          <div class="row">
            <label class="field grow">
              <span>Certificado do cliente</span>
              <input v-model="form.tls.certFile" class="input mono" :class="{ invalid: submitted && errors.certFile }" spellcheck="false" />
            </label>
            <label class="field grow">
              <span>Chave do cliente</span>
              <input v-model="form.tls.keyFile" class="input mono" spellcheck="false" />
            </label>
          </div>
          <small v-if="submitted && errors.certFile" class="err">{{ errors.certFile }}</small>
          <label class="field">
            <span>Nome do servidor no certificado (opcional)</span>
            <input v-model="form.tls.serverName" class="input" :placeholder="form.host" spellcheck="false" />
          </label>
        </template>
        <label class="check">
          <input v-model="form.allowCleartextPasswords" type="checkbox" />
          Permitir envio de senha em texto claro (plugin mysql_clear_password, ex.: PAM/LDAP)
        </label>
        <small v-if="submitted && errors.cleartext" class="err">{{ errors.cleartext }}</small>
        <p class="hint">
          Métodos suportados: mysql_native_password, caching_sha2_password, sha256_password e ed25519 (MariaDB).
          Sem TLS, caching_sha2_password usa a chave RSA pública do servidor.
        </p>
      </template>

      <template v-else>
        <div class="row">
          <label class="field">
            <span>Tempo limite de conexão (s)</span>
            <input v-model.number="form.connectTimeoutSeconds" class="input" type="number" min="1" max="300" />
          </label>
        </div>
        <label class="check">
          <input v-model="form.readOnly" type="checkbox" />
          Somente leitura (bloqueia instruções que alteram dados ou estrutura)
        </label>
        <div class="row">
          <label class="field grow">
            <span>Pasta (use / para subpastas)</span>
            <input v-model="form.folder" class="input" list="conn-folders" placeholder="Clientes/ACME" spellcheck="false" />
            <datalist id="conn-folders"><option v-for="f in conns.folders" :key="f" :value="f" /></datalist>
          </label>
          <label class="field grow">
            <span>Databases exibidos (vazio = todos)</span>
            <input v-model="form.databaseFilter" class="input mono" placeholder="loja_*; relatorios" spellcheck="false" />
          </label>
        </div>
        <label class="field">
          <span>Comentário</span>
          <textarea v-model="form.comment" class="input" rows="2" maxlength="2000" />
        </label>
        <label class="field">
          <span>Script inicial (executado em cada nova conexão)</span>
          <textarea v-model="form.initSQL" class="input mono" rows="3" spellcheck="false" placeholder="SET time_zone = '+00:00'; SET SESSION sql_mode = 'TRADITIONAL'" />
          <small class="hint">Instruções separadas por ponto e vírgula. Controle de transações não é aceito; em conexões somente leitura, apenas SET e leituras.</small>
        </label>
        <p v-if="editing?.stats?.connects || editing?.stats?.failures" class="hint">
          {{ editing.stats.connects }} conexão(ões) bem-sucedida(s), {{ editing.stats.failures }} falha(s){{ editing.stats.lastConnectedAt ? ` · última em ${new Date(editing.stats.lastConnectedAt).toLocaleString()}` : '' }}
          {{ editing.createdAt ? ` · criada em ${new Date(editing.createdAt).toLocaleDateString()}` : '' }}
        </p>
        <div class="field">
          <span>Parâmetros de conexão</span>
          <div v-for="(p, i) in params" :key="i" class="param">
            <input v-model="p.key" class="input mono" placeholder="charset" spellcheck="false" />
            <input v-model="p.value" class="input mono" placeholder="utf8mb4" spellcheck="false" />
            <button type="button" class="icon-btn" aria-label="Remover parâmetro" @click="params.splice(i, 1)"><Icon name="x" :size="14" /></button>
          </div>
          <div>
            <button type="button" class="btn small" @click="params.push({ key: '', value: '' })"><Icon name="plus" :size="12" /> Adicionar parâmetro</button>
          </div>
          <small class="hint">
            charset, collation, compress e connectionAttributes configuram o driver; outros nomes são aplicados como variáveis de sessão
            (ex.: <code>time_zone</code> = <code>'+00:00'</code>). Opções inseguras (allowAllFiles, multiStatements…) são recusadas.
          </small>
        </div>
      </template>
    </form>

    <div v-if="testResult" class="test" :class="testResult.ok ? 'ok' : 'fail'" role="status">
      <Icon :name="testResult.ok ? 'check' : 'alert'" :size="15" />
      <span class="selectable">{{ testResult.message }}</span>
      <span v-if="testResult.info" class="badge" :class="testResult.info.tlsActive ? 'tls-on' : 'tls-off'">
        <Icon name="lock" :size="11" /> {{ testResult.info.tlsActive ? 'TLS ativo' : 'sem TLS' }}
      </span>
    </div>

    <template #footer>
      <button class="btn" :disabled="testing" @click="test">
        <span v-if="testing" class="spinner" /> <Icon v-else name="plug" :size="14" /> Testar
      </button>
      <span class="spacer" />
      <button class="btn" @click="ui.closeConnectionDialog()">Cancelar</button>
      <button class="btn" :disabled="saving" @click="save(false)">Salvar</button>
      <button class="btn primary" :disabled="saving" @click="save(true)">Salvar e conectar</button>
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
  gap: 12px;
  min-height: 330px;
}
.row {
  display: flex;
  gap: 12px;
}
.grow {
  flex: 1;
}
.port {
  width: 100px;
}
.color {
  width: 48px;
  height: 30px;
  padding: 2px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius);
  background: var(--bg-input);
}
.color-row {
  display: flex;
  align-items: center;
  gap: 2px;
}
.err {
  color: var(--danger);
  font-size: 12px;
}
.param {
  display: grid;
  grid-template-columns: 1fr 1fr auto;
  gap: 6px;
  margin-bottom: 6px;
}
.callout {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  margin: 0;
  padding: 8px 10px;
  border-radius: var(--radius);
  font-size: 12px;
  line-height: 1.45;
}
.callout.danger {
  background: var(--danger-soft);
  color: var(--danger);
}
.test {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
  padding: 8px 10px;
  border-radius: var(--radius);
  font-size: 12px;
}
.test.ok {
  background: var(--success-soft);
  color: var(--success);
}
.test.fail {
  background: var(--danger-soft);
  color: var(--danger);
}
.test span.selectable {
  flex: 1;
  color: var(--text);
  word-break: break-word;
}
.tls-on {
  background: var(--success-soft);
  color: var(--success);
}
.tls-off {
  background: var(--warning-soft);
  color: var(--warning);
}
.spacer {
  flex: 1;
}
code {
  font-family: var(--font-mono);
  font-size: 11px;
}
@media (max-width: 560px) {
  .row {
    flex-direction: column;
  }
  .port {
    width: auto;
  }
}
</style>

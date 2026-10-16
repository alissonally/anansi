import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { backend } from '../api/backend'
import { AppError, toAppError } from '../api/errors'
import type { Profile, ProfileView, ServerInfo, UnknownHost } from '../api/types'
import { useUiStore } from './ui'
import { useSettingsStore } from './settings'

export type ConnStatus = 'disconnected' | 'connecting' | 'connected' | 'error'

export interface ConnState {
  status: ConnStatus
  server?: ServerInfo
  error?: string
}

export const environmentLabels: Record<Profile['environment'], string> = {
  local: 'Local',
  development: 'Desenvolvimento',
  staging: 'Homologação',
  production: 'Produção',
}

export function emptyProfile(): Profile {
  return {
    id: '',
    name: '',
    kind: 'mysql',
    host: '127.0.0.1',
    port: 3306,
    user: 'root',
    database: '',
    tls: { mode: 'preferred' },
    params: {},
    allowCleartextPasswords: false,
    environment: 'development',
    color: '',
    readOnly: false,
    savePassword: true,
    connectTimeoutSeconds: 10,
    socket: '',
    ssh: { enabled: false, host: '', port: 22, user: '', auth: 'password', keyFile: '', saveSecret: true },
    initSQL: '',
    keepAliveSeconds: 0,
    folder: '',
    comment: '',
    databaseFilter: '',
  }
}

export const useConnectionsStore = defineStore('connections', () => {
  const profiles = ref<ProfileView[]>([])
  const states = ref<Record<string, ConnState>>({})
  const loaded = ref(false)

  const byId = computed(() => new Map(profiles.value.map((p) => [p.id, p])))

  function state(id: string): ConnState {
    return states.value[id] ?? { status: 'disconnected' }
  }

  function setState(id: string, s: ConnState) {
    states.value = { ...states.value, [id]: s }
  }

  async function load() {
    const [list, sessions] = await Promise.all([backend.listConnections(), backend.sessionStates()])
    profiles.value = list
    for (const s of sessions) {
      setState(s.profileId, { status: s.status, server: s.server })
    }
    loaded.value = true
  }

  async function save(profile: Profile, password: string | null, sshSecret: string | null = null) {
    const res = await backend.saveConnection(profile, password, sshSecret)
    const i = profiles.value.findIndex((p) => p.id === res.profile.id)
    if (i >= 0) profiles.value.splice(i, 1, res.profile)
    else profiles.value.push(res.profile)
    if (res.warning) useUiStore().toast('warning', res.warning, 10000)
    return res.profile
  }

  async function duplicate(id: string) {
    const p = await backend.duplicateConnection(id)
    profiles.value.push(p)
    return p
  }

  async function remove(id: string) {
    await backend.deleteConnection(id)
    profiles.value = profiles.value.filter((p) => p.id !== id)
    const next = { ...states.value }
    delete next[id]
    states.value = next
  }

  async function reorder(ids: string[]) {
    const map = byId.value
    profiles.value = ids.map((id) => map.get(id)).filter((p): p is ProfileView => !!p)
    await backend.reorderConnections(ids)
  }

  /**
   * Opens a session. Asks for the password when none is stored. Returns
   * true when connected.
   */
  async function connect(id: string): Promise<boolean> {
    const ui = useUiStore()
    const profile = byId.value.get(id)
    if (!profile) return false
    if (state(id).status === 'connected') return true
    setState(id, { status: 'connecting' })
    let password: string | null = null
    let remember = false
    let trusted = 0
    for (let attempt = 0; attempt < 3; attempt++) {
      try {
        const s = await backend.connect(id, password, remember)
        setState(id, { status: 'connected', server: s.server })
        if (password !== null && remember) {
          const p = byId.value.get(id)
          if (p) p.hasStoredPassword = true
        }
        if (profile.environment === 'production' && !s.server.tlsActive) {
          ui.toast('warning', `${profile.name}: conexão de produção sem TLS.`, 10000)
        }
        return true
      } catch (e) {
        const err = toAppError(e)
        const authFailed = err.db?.category === 'auth'
        if (err.code === 'password_required' || (authFailed && attempt < 2)) {
          const canRemember = profile.savePassword && (useSettingsStore().info?.keyringAvailable ?? false)
          const r = await ui.prompt({
            title: `Conectar a ${profile.name}`,
            label: `Senha de ${profile.user}@${profile.host}`,
            message: authFailed ? `Falha de autenticação: ${err.message}` : err.message,
            password: true,
            confirmLabel: 'Conectar',
            checkbox: canRemember ? { label: 'Salvar senha no keyring do sistema', value: true } : undefined,
          })
          if (!r) {
            setState(id, { status: 'disconnected' })
            return false
          }
          password = r.value
          remember = r.checked
          continue
        }
        if (err.code === 'ssh_secret_required' && attempt < 2) {
          const keyring = profile.ssh.saveSecret && (useSettingsStore().info?.keyringAvailable ?? false)
          const r = await ui.prompt({
            title: `Túnel SSH — ${profile.name}`,
            label: profile.ssh.auth === 'key' ? `Frase secreta da chave ${profile.ssh.keyFile}` : `Senha SSH de ${profile.ssh.user}@${profile.ssh.host}`,
            message: err.message,
            password: true,
            confirmLabel: 'Conectar',
            checkbox: keyring ? { label: 'Salvar no keyring do sistema', value: true } : undefined,
          })
          if (!r) {
            setState(id, { status: 'disconnected' })
            return false
          }
          await backend.setSshSecret(id, r.value, r.checked)
          continue
        }
        if (err.code === 'ssh_host_unknown' && trusted++ === 0) {
          const h = err.details as UnknownHost
          const ok = await ui.confirm({
            title: 'Servidor SSH desconhecido',
            message: `A autenticidade de ${h.host} não pôde ser confirmada. Confira a impressão digital com o administrador do servidor antes de continuar.`,
            items: [{ text: `${h.keyType} ${h.fingerprint}` }],
            confirmLabel: 'Confiar e conectar',
          })
          if (!ok) {
            setState(id, { status: 'disconnected' })
            return false
          }
          await backend.trustSshHost(h.host, h.fingerprint)
          attempt--
          continue
        }
        setState(id, { status: 'error', error: err.message })
        ui.toast('error', `${profile.name}: ${err.message}`)
        return false
      }
    }
    setState(id, { status: 'disconnected' })
    return false
  }

  async function disconnect(id: string) {
    try {
      await backend.disconnect(id)
    } finally {
      setState(id, { status: 'disconnected' })
    }
  }

  /** Marks a connection as lost (e.g. after a network error). */
  async function refreshStates() {
    const sessions = await backend.sessionStates()
    const open = new Set(sessions.map((s) => s.profileId))
    for (const id of Object.keys(states.value)) {
      if (!open.has(id) && state(id).status === 'connected') setState(id, { status: 'disconnected' })
    }
  }

  /** Every folder path in use, with its parents ("A", "A/B"). */
  const folders = computed(() => {
    const out = new Set<string>()
    for (const p of profiles.value) {
      const segs = (p.folder ?? '').split('/').filter(Boolean)
      for (let i = 1; i <= segs.length; i++) out.add(segs.slice(0, i).join('/'))
    }
    return [...out].sort((a, b) => a.localeCompare(b))
  })

  async function exportProfiles(ids: string[] = []) {
    const ui = useUiStore()
    try {
      const path = await backend.exportConnections(ids)
      if (path) ui.toast('success', `Conexões exportadas para ${path} (sem senhas).`, 6000)
    } catch (e) {
      ui.toast('error', toAppError(e).message)
    }
  }

  async function importProfiles() {
    const ui = useUiStore()
    try {
      const r = await backend.importConnections()
      if (!r) return
      await load()
      ui.toast(r.problems?.length ? 'warning' : 'success', `${r.imported} conexão(ões) importada(s).${r.problems?.length ? ' Ignoradas: ' + r.problems.join('; ') : ''}`, 10000)
    } catch (e) {
      ui.toast('error', toAppError(e).message)
    }
  }

  return { profiles, states, loaded, byId, state, load, save, duplicate, remove, reorder, connect, disconnect, refreshStates, folders, exportProfiles, importProfiles }
})

export function isAppError(e: unknown, code: AppError['code']): e is AppError {
  return e instanceof AppError && e.code === code
}

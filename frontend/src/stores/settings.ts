import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { backend } from '../api/backend'
import type { AppInfo, Preferences } from '../api/types'

export const defaultPreferences: Preferences = {
  theme: 'system',
  editorFontSize: 14,
  maxRows: 500,
  queryTimeoutSeconds: 0,
  historyEnabled: true,
  historyLimit: 1000,
  confirmDestructive: true,
  showSystemSchemas: false,
  hideWarnings: false,
  reconnectOnStartup: false,
  editorTabSize: 2,
  editorUseTabs: false,
  editorWordWrap: false,
  editorMinimap: false,
  autocompleteOff: false,
  uppercaseKeywords: false,
  gridMaxColumnWidth: 380,
  gridThousands: false,
  timestampColumns: [],
  sqlLogLimit: 5000,
  shortcuts: {},
}

export const useSettingsStore = defineStore('settings', () => {
  const prefs = ref<Preferences>({ ...defaultPreferences })
  const info = ref<AppInfo | null>(null)
  const systemDark = ref(window.matchMedia?.('(prefers-color-scheme: dark)').matches ?? true)

  const effectiveTheme = computed<'light' | 'dark'>(() =>
    prefs.value.theme === 'system' ? (systemDark.value ? 'dark' : 'light') : prefs.value.theme,
  )

  function applyTheme() {
    document.documentElement.dataset.theme = effectiveTheme.value
  }

  async function load() {
    const media = window.matchMedia?.('(prefers-color-scheme: dark)')
    media?.addEventListener('change', (e) => {
      systemDark.value = e.matches
      applyTheme()
    })
    const [p, i] = await Promise.all([backend.getPreferences(), backend.appInfo()])
    prefs.value = { ...defaultPreferences, ...p }
    info.value = i
    applyTheme()
  }

  async function save(p: Preferences) {
    prefs.value = { ...defaultPreferences, ...(await backend.savePreferences(p)) }
    applyTheme()
  }

  return { prefs, info, effectiveTheme, load, save, applyTheme }
})

// State shared by the object editors (table, view, routine, trigger,
// event): loading, dirty tracking, a debounced SQL preview and saving with
// the backend confirmation flow.
import { computed, onBeforeUnmount, ref, watch, type Ref } from 'vue'
import { toAppError } from '../api/errors'
import type { AdminResult } from '../api/types'
import { useTabsStore, type Tab } from '../stores/tabs'
import { useUiStore } from '../stores/ui'
import { useMetadataStore } from '../stores/metadata'
import { withConfirmation } from './objectActions'
import { clone } from '../utils/text'

export interface EditorBackend<T> {
  /** Loads the current definition; called when the object exists. */
  load: () => Promise<T>
  /** Definition of a new object. */
  blank: () => T
  preview: (def: T) => Promise<string[]>
  save: (def: T, confirmed: boolean) => Promise<AdminResult[]>
  /** Name of the object after saving (to retarget the tab). */
  nameOf: (def: T) => string
}

export function useObjectEditor<T>(tab: () => Tab, b: EditorBackend<T>) {
  const tabs = useTabsStore()
  const ui = useUiStore()
  const meta = useMetadataStore()
  const def = ref(null) as Ref<T | null>
  const original = ref('')
  const loading = ref(false)
  const loadError = ref('')
  const saving = ref(false)
  const failure = ref<{ message: string; sql: string } | null>(null)
  const preview = ref<string[]>([])
  const previewError = ref('')
  const isNew = computed(() => !tab().objectName)

  const dirty = computed(() => !!def.value && JSON.stringify(def.value) !== original.value)
  watch(dirty, (d) => {
    const t = tabs.find(tab().id)
    if (t) t.objectDirty = d
  })

  async function load() {
    loading.value = true
    loadError.value = ''
    failure.value = null
    try {
      const d = isNew.value ? b.blank() : await b.load()
      def.value = d
      original.value = JSON.stringify(d)
    } catch (e) {
      loadError.value = toAppError(e).message
    } finally {
      loading.value = false
    }
  }

  let timer: number | undefined
  let seq = 0
  watch(def, (d) => {
    window.clearTimeout(timer)
    if (!d) return
    timer = window.setTimeout(async () => {
      const my = ++seq
      try {
        const out = await b.preview(clone(d))
        if (my !== seq) return
        preview.value = out
        previewError.value = ''
      } catch (e) {
        if (my !== seq) return
        previewError.value = toAppError(e).message
      }
    }, 300)
  }, { deep: true })
  onBeforeUnmount(() => window.clearTimeout(timer))

  /** Saves; returns true when every statement succeeded. */
  async function save(): Promise<boolean> {
    const d = def.value
    if (!d || saving.value) return false
    saving.value = true
    failure.value = null
    try {
      const res = await withConfirmation((c) => b.save(clone(d), c))
      if (!res) return false
      const bad = res.find((r) => r.error)
      const t = tabs.find(tab().id)
      if (bad) {
        failure.value = { message: bad.error!.message, sql: bad.sql }
        // A partly applied change (several statements) must be reloaded.
        if (res.indexOf(bad) > 0 && !isNew.value) ui.toast('warning', 'Parte das alterações foi aplicada antes do erro; recarregue para ver o estado atual.', 10000)
        return false
      }
      if (!res.length) {
        ui.toast('info', 'Nada a alterar.')
        return true
      }
      const name = b.nameOf(d)
      if (t) {
        t.objectName = name
        t.title = name
      }
      ui.toast('success', `"${name}" salvo.`, 2500)
      await meta.invalidate(t?.connectionId ?? '', t?.database ?? '')
      await load()
      return true
    } catch (e) {
      ui.toast('error', toAppError(e).message)
      return false
    } finally {
      saving.value = false
    }
  }

  async function discard() {
    if (!dirty.value) return
    const ok = await ui.confirm({ title: 'Descartar alterações', message: 'As alterações feitas neste editor serão perdidas.', confirmLabel: 'Descartar', danger: true })
    if (ok && original.value) def.value = JSON.parse(original.value) as T
  }

  return { def, loading, loadError, saving, failure, preview, previewError, dirty, isNew, load, save, discard }
}

import { defineStore } from 'pinia'
import { ref } from 'vue'

export type ToastKind = 'info' | 'success' | 'warning' | 'error'

export interface Toast {
  id: number
  kind: ToastKind
  message: string
}

export interface ConfirmOptions {
  title: string
  message: string
  /** Optional list shown below the message (e.g. statements). */
  items?: { text: string; note?: string }[]
  confirmLabel?: string
  cancelLabel?: string
  danger?: boolean
  /** When set, the user must type this text to enable the confirm button. */
  typeToConfirm?: string
}

export interface PromptOptions {
  title: string
  label: string
  value?: string
  placeholder?: string
  password?: boolean
  confirmLabel?: string
  /** Optional checkbox (e.g. "remember password"). */
  checkbox?: { label: string; value: boolean }
  message?: string
  /** Only the message and checkbox (no text field). */
  hideInput?: boolean
  multiline?: boolean
  /** Shows a select with these options instead of a text field. */
  choices?: { value: string; label: string }[]
}

export interface PromptResult {
  value: string
  checked: boolean
}

export interface DdlDialogState {
  kind: 'database'
  connectionId: string
  database?: string
  /** Called with the created object's name. */
  onCreated?: (name: string) => void
}

export interface ExportDialogState {
  title: string
  columns: import('../api/types').ExportColumn[]
  /** Rows loaded in the grid (in display order). */
  rows: () => import('../api/types').Cell[][]
  /** Selected rows, when the grid has a selection. */
  selected?: () => import('../api/types').Cell[][]
  table?: string
  query?: string
  /** Re-run on the server and stream everything to a file. */
  full?: { kind: 'query'; connectionId: string; database: string; sql: string } | { kind: 'table'; connectionId: string; spec: import('../api/types').BrowseSpec }
}

interface Pending<O, R> {
  options: O
  resolve: (r: R) => void
}

export const useUiStore = defineStore('ui', () => {
  const toasts = ref<Toast[]>([])
  const confirmState = ref<Pending<ConfirmOptions, boolean> | null>(null)
  const promptState = ref<Pending<PromptOptions, PromptResult | null> | null>(null)
  const connectionDialog = ref<{ open: boolean; profileId: string | null }>({ open: false, profileId: null })
  const preferencesOpen = ref(false)
  const ddlDialog = ref<DdlDialogState | null>(null)
  const exportDialog = ref<ExportDialogState | null>(null)
  const dumpDialog = ref<{ connectionId: string; databases: string[]; tables?: string[] } | null>(null)
  const csvDialog = ref<{ connectionId: string; database: string; table?: string } | null>(null)
  const toolsDialog = ref<{ connectionId: string; databases: string[]; tables?: import('../api/types').TableRef[]; tab: 'maintenance' | 'search' | 'bulk' } | null>(null)
  const copyTableDialog = ref<{ connectionId: string; database: string; table: string } | null>(null)
  const generateDialog = ref<{ connectionId: string; database: string; table: string } | null>(null)
  const compareDialog = ref<{ connectionId: string; database: string } | null>(null)
  const databaseFilterDialog = ref<{ connectionId: string } | null>(null)
  let nextId = 1

  function toast(kind: ToastKind, message: string, timeoutMs = kind === 'error' ? 8000 : 4000) {
    const id = nextId++
    toasts.value.push({ id, kind, message })
    window.setTimeout(() => dismiss(id), timeoutMs)
  }

  function dismiss(id: number) {
    toasts.value = toasts.value.filter((t) => t.id !== id)
  }

  /** Shows a confirmation dialog; resolves to true when confirmed. */
  function confirm(options: ConfirmOptions): Promise<boolean> {
    confirmState.value?.resolve(false)
    return new Promise((resolve) => {
      confirmState.value = { options, resolve }
    })
  }

  function resolveConfirm(ok: boolean) {
    const p = confirmState.value
    confirmState.value = null
    p?.resolve(ok)
  }

  /** Shows an input dialog; resolves to null when cancelled. */
  function prompt(options: PromptOptions): Promise<PromptResult | null> {
    promptState.value?.resolve(null)
    return new Promise((resolve) => {
      promptState.value = { options, resolve }
    })
  }

  function resolvePrompt(r: PromptResult | null) {
    const p = promptState.value
    promptState.value = null
    p?.resolve(r)
  }

  function openConnectionDialog(profileId: string | null = null) {
    connectionDialog.value = { open: true, profileId }
  }

  function closeConnectionDialog() {
    connectionDialog.value = { open: false, profileId: null }
  }

  return {
    toasts, toast, dismiss,
    confirmState, confirm, resolveConfirm,
    promptState, prompt, resolvePrompt,
    connectionDialog, openConnectionDialog, closeConnectionDialog,
    preferencesOpen, ddlDialog, exportDialog, dumpDialog, csvDialog,
    toolsDialog, copyTableDialog, generateDialog, compareDialog, databaseFilterDialog,
  }
})

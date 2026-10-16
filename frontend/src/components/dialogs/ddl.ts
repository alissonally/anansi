import { computed, ref, watch, type Ref } from 'vue'
import { backend } from '../../api/backend'
import { toAppError } from '../../api/errors'
import type { ConfirmationDetails, DDLOptions } from '../../api/types'
import { confirmStatements } from '../../stores/tabs'

const optionsCache = new Map<string, DDLOptions>()

/** Charsets, collations, engines and types of a connection (cached per session of the app). */
export function useDdlOptions(connectionId: () => string) {
  const options = ref<DDLOptions | null>(null)
  const error = ref('')
  watch(
    connectionId,
    async (id) => {
      options.value = optionsCache.get(id) ?? null
      error.value = ''
      if (options.value || !id) return
      try {
        const o = await backend.ddlOptions(id)
        optionsCache.set(id, o)
        if (connectionId() === id) options.value = o
      } catch (e) {
        error.value = toAppError(e).message
      }
    },
    { immediate: true },
  )
  const charsets = computed(() => options.value?.charsets ?? [])
  function collationsOf(charset: string): string[] {
    return charsets.value.find((c) => c.name === charset)?.collations ?? []
  }
  function defaultCollationOf(charset: string): string {
    return charsets.value.find((c) => c.name === charset)?.defaultCollation ?? ''
  }
  return { options, error, charsets, collationsOf, defaultCollationOf }
}

/** Regenerates the SQL preview (debounced) whenever the spec changes. */
export function useSqlPreview<T>(spec: Ref<T>, build: (s: T) => Promise<string>) {
  const sql = ref('')
  const error = ref('')
  let timer: number | undefined
  let seq = 0
  watch(
    spec,
    (s) => {
      window.clearTimeout(timer)
      timer = window.setTimeout(async () => {
        const my = ++seq
        try {
          const out = await build(s)
          if (my !== seq) return
          sql.value = out
          error.value = ''
        } catch (e) {
          if (my !== seq) return
          error.value = toAppError(e).message
        }
      }, 200)
    },
    { deep: true, immediate: true },
  )
  return { sql, error }
}

/**
 * Runs a create call; when the backend requires confirmation (production
 * connection) asks the user and retries with confirmed=true.
 * Returns true on success, false when the user declined.
 */
export async function runWithConfirmation(run: (confirmed: boolean) => Promise<void>): Promise<boolean> {
  try {
    await run(false)
    return true
  } catch (e) {
    const err = toAppError(e)
    if (err.code !== 'confirmation_required') throw err
    if (!(await confirmStatements(err.details as ConfirmationDetails))) return false
    await run(true)
    return true
  }
}

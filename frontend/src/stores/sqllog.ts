import { defineStore } from 'pinia'
import { ref } from 'vue'
import { backend } from '../api/backend'
import type { SQLLogEntry } from '../api/types'
import { EventsOn } from '../../wailsjs/runtime/runtime'

const MAX = 3000

/** Every statement sent to the servers (HeidiSQL's SQL log). */
export const useSqlLogStore = defineStore('sqllog', () => {
  const entries = ref<SQLLogEntry[]>([])
  const paused = ref(false)
  let lastId = 0
  let started = false

  function push(batch: SQLLogEntry[]) {
    const fresh = batch.filter((e) => e.id > lastId)
    if (!fresh.length) return
    lastId = fresh[fresh.length - 1]!.id
    if (paused.value) return
    const next = entries.value.concat(fresh)
    entries.value = next.length > MAX ? next.slice(next.length - MAX) : next
  }

  async function start() {
    if (started) return
    started = true
    try {
      push(await backend.listSqlLog(0, MAX))
    } catch {
      // backend unavailable (tests)
    }
    try {
      EventsOn('sqllog:entries', (batch: SQLLogEntry[]) => push(batch ?? []))
    } catch {
      // runtime unavailable
    }
  }

  async function clear() {
    entries.value = []
    await backend.clearSqlLog().catch(() => undefined)
  }

  return { entries, paused, start, clear }
})

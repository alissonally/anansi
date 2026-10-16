import { defineStore } from 'pinia'
import { ref } from 'vue'
import { backend } from '../api/backend'
import type { ColumnInfo, DatabaseInfo, DatabaseSummary, EventInfo, IndexInfo, RoutineDetail, RoutineInfo, TableInfo, TableStatus, TriggerInfo } from '../api/types'

type Entry<T> = { status: 'loading' | 'ready' | 'error'; data: T; error?: string }

/** Frontend view of catalog metadata (the backend keeps the real cache). */
export const useMetadataStore = defineStore('metadata', () => {
  const entries = ref<Record<string, Entry<unknown>>>({})
  // How to reload each key; used to refresh what is on screen after an
  // invalidation instead of leaving expanded tree nodes empty.
  const loaders = new Map<string, () => Promise<unknown>>()

  function key(...parts: string[]) {
    return parts.join('\u0000')
  }

  function get<T>(k: string): Entry<T> | undefined {
    return entries.value[k] as Entry<T> | undefined
  }

  async function load<T>(k: string, fn: () => Promise<T>, empty: T, force = false): Promise<T> {
    loaders.set(k, () => load(k, fn, empty, true))
    const cur = get<T>(k)
    if (cur && !force && cur.status !== 'error') return cur.data
    entries.value[k] = { status: 'loading', data: cur?.data ?? empty }
    try {
      const data = await fn()
      entries.value[k] = { status: 'ready', data }
      return data
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      entries.value[k] = { status: 'error', data: empty, error: msg }
      throw e
    }
  }

  const databases = (conn: string, force = false) => load<DatabaseInfo[]>(key(conn, 'dbs'), () => backend.listDatabases(conn), [], force)
  const tables = (conn: string, db: string, force = false) => load<TableInfo[]>(key(conn, 'tables', db), () => backend.listTables(conn, db), [], force)
  const routines = (conn: string, db: string, force = false) => load<RoutineInfo[]>(key(conn, 'routines', db), () => backend.listRoutines(conn, db), [], force)
  const columns = (conn: string, db: string, t: string, force = false) => load<ColumnInfo[]>(key(conn, 'cols', db, t), () => backend.listColumns(conn, db, t), [], force)
  const indexes = (conn: string, db: string, t: string, force = false) => load<IndexInfo[]>(key(conn, 'idx', db, t), () => backend.listIndexes(conn, db, t), [], force)

  const triggers = (conn: string, db: string, force = false) => load<TriggerInfo[]>(key(conn, 'triggers', db), () => backend.triggers(conn, db), [], force)
  const events = (conn: string, db: string, force = false) => load<EventInfo[]>(key(conn, 'events', db), () => backend.events(conn, db), [], force)
  const routineDetails = (conn: string, db: string, force = false) => load<RoutineDetail[]>(key(conn, 'rdetails', db), () => backend.routineDetails(conn, db), [], force)
  const tableStatus = (conn: string, db: string, force = false) => load<TableStatus[]>(key(conn, 'status', db), () => backend.tableStatus(conn, db), [], force)
  const summaries = (conn: string, force = false) => load<DatabaseSummary[]>(key(conn, 'dbs', 'summaries'), () => backend.databaseSummaries(conn), [], force)

  function entry<T>(...parts: string[]): Entry<T> | undefined {
    return get<T>(key(...parts))
  }

  /**
   * Invalidates cached entries of a connection (db = '' for all) here and,
   * optionally, in the backend. With reload, entries that were loaded are
   * fetched again (keeping the old data visible meanwhile); otherwise they
   * are dropped (e.g. after disconnecting).
   */
  async function invalidate(conn: string, db = '', notifyBackend = true, reload = true) {
    if (notifyBackend) await backend.refreshMetadata(conn, db).catch(() => undefined)
    const prefix = conn + '\u0000'
    const next: Record<string, Entry<unknown>> = {}
    const reloads: Promise<unknown>[] = []
    for (const [k, v] of Object.entries(entries.value)) {
      const parts = k.split('\u0000')
      const hit = k.startsWith(prefix) && (db === '' || parts[1] === 'dbs' || parts[2] === db)
      if (!hit) {
        next[k] = v
      } else if (reload && loaders.has(k)) {
        next[k] = v
        reloads.push(loaders.get(k)!().catch(() => undefined))
      } else {
        loaders.delete(k)
      }
    }
    entries.value = next
    await Promise.all(reloads)
  }

  return { databases, tables, routines, columns, indexes, triggers, events, routineDetails, tableStatus, summaries, entry, invalidate }
})

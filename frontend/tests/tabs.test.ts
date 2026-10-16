import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { ExecuteRequest, ExecuteResponse, ProfileView } from '../src/api/types'

// ---- backend double ---------------------------------------------------------
const calls: ExecuteRequest[] = []
let executeImpl: (req: ExecuteRequest) => Promise<ExecuteResponse>
const released: string[] = []

vi.mock('../src/api/backend', () => ({
  backend: {
    execute: (req: ExecuteRequest) => {
      calls.push(req)
      return executeImpl(req)
    },
    releaseTab: async (id: string) => {
      released.push(id)
    },
    saveWorkspace: async () => undefined,
    loadWorkspace: async () => ({ tabs: [], activeTabId: '', sidebarWidth: 0, resultsHeight: 0 }),
    cancel: async () => undefined,
    fetchMore: async () => ({ rows: [[3]], hasMore: false, limitReached: false, offset: 2, durationMs: 1 }),
    connect: async () => ({ profileId: 'x', status: 'connected', server: { version: '8', flavor: 'MySQL', tlsActive: true, user: 'u' }, openedAt: '' }),
    sessionStates: async () => [],
  },
}))

import { isDirty, useTabsStore } from '../src/stores/tabs'
import { emptyProfile, useConnectionsStore } from '../src/stores/connections'
import { useUiStore } from '../src/stores/ui'

function profile(id: string, env: ProfileView['environment'] = 'development'): ProfileView {
  return {
    ...emptyProfile(), hasStoredSSHSecret: false,
    id, name: id, kind: 'mysql', host: 'h', port: 3306, user: 'u', database: 'db_' + id, tls: { mode: 'preferred' }, params: {},
    allowCleartextPasswords: false, environment: env, color: '', readOnly: false, savePassword: false, connectTimeoutSeconds: 10, hasStoredPassword: true,
  }
}

function response(rows: unknown[][], extra: Partial<ExecuteResponse> = {}): ExecuteResponse {
  return {
    executionId: 'e', database: 'db', inTransaction: false, cancelled: false, durationMs: 1,
    results: [{ index: 0, sql: 'SELECT', line: 1, kind: 'read', columns: [{ name: 'a', type: 'INT', binary: false, numeric: true }], rows: rows as never, hasRows: true, hasMore: false, limitReached: false, durationMs: 1 }],
    ...extra,
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  calls.length = 0
  released.length = 0
  executeImpl = async () => response([[1]])
  const conns = useConnectionsStore()
  conns.profiles = [profile('dev'), profile('prod', 'production')]
  conns.states = { dev: { status: 'connected' }, prod: { status: 'connected' } }
})

describe('tabs store', () => {
  it('keeps results independent per tab even when executions overlap', async () => {
    const tabs = useTabsStore()
    const a = tabs.open({ connectionId: 'dev' })
    const b = tabs.open({ connectionId: 'dev' })
    let releaseA: () => void = () => {}
    executeImpl = (req) =>
      req.tabId === a.id
        ? new Promise((resolve) => { releaseA = () => resolve(response([['A']])) })
        : Promise.resolve(response([['B']]))
    const runA = tabs.run(a.id, { sql: 'SELECT 1', cursorOffset: -1, mode: 'statement', baseLine: 1 })
    await tabs.run(b.id, { sql: 'SELECT 2', cursorOffset: -1, mode: 'statement', baseLine: 1 })
    expect(tabs.find(a.id)!.status).toBe('running')
    expect(tabs.find(b.id)!.results[0]!.rows).toEqual([['B']])
    releaseA()
    await runA
    expect(tabs.find(a.id)!.results[0]!.rows).toEqual([['A']])
    expect(tabs.find(b.id)!.results[0]!.rows).toEqual([['B']])
    expect(calls.map((c) => c.connectionId)).toEqual(['dev', 'dev'])
  })

  it('switching tabs does not affect a running execution', async () => {
    const tabs = useTabsStore()
    const a = tabs.open({ connectionId: 'dev' })
    let done: () => void = () => {}
    executeImpl = () => new Promise((resolve) => { done = () => resolve(response([['A']])) })
    const p = tabs.run(a.id, { sql: 'SELECT 1', cursorOffset: -1, mode: 'statement', baseLine: 1 })
    tabs.open({ connectionId: 'dev' })
    expect(tabs.activeId).not.toBe(a.id)
    done()
    await p
    expect(tabs.find(a.id)!.results[0]!.rows).toEqual([['A']])
    expect(tabs.find(a.id)!.status).toBe('idle')
  })

  it('preserves SQL per tab and sends byte offsets to the backend', async () => {
    const tabs = useTabsStore()
    const a = tabs.open({ connectionId: 'dev' })
    const b = tabs.open({ connectionId: 'dev' })
    tabs.setSql(a.id, "SELECT 'ção'; SELECT 2")
    tabs.setSql(b.id, 'SHOW TABLES')
    tabs.activate(a.id)
    expect(tabs.find(a.id)!.sql).toBe("SELECT 'ção'; SELECT 2")
    expect(tabs.find(b.id)!.sql).toBe('SHOW TABLES')
    const sql = tabs.find(a.id)!.sql
    await tabs.run(a.id, { sql, cursorOffset: sql.indexOf('SELECT 2'), mode: 'statement', baseLine: 1 })
    // "ção" is 3 UTF-16 units but 5 UTF-8 bytes.
    expect(calls[0]!.cursorOffset).toBe(sql.indexOf('SELECT 2') + 2)
  })

  it('asks before switching an open tab to a production connection', async () => {
    const tabs = useTabsStore()
    const ui = useUiStore()
    const t = tabs.open({ connectionId: 'dev', sql: 'DELETE FROM x WHERE id = 1' })
    const p = tabs.setConnection(t.id, 'prod')
    await Promise.resolve()
    expect(ui.confirmState?.options.title).toContain('produção')
    ui.resolveConfirm(false)
    expect(await p).toBe(false)
    expect(tabs.find(t.id)!.connectionId).toBe('dev')

    const p2 = tabs.setConnection(t.id, 'prod')
    await Promise.resolve()
    ui.resolveConfirm(true)
    expect(await p2).toBe(true)
    expect(tabs.find(t.id)!.connectionId).toBe('prod')
    expect(tabs.find(t.id)!.database).toBe('db_prod')
    expect(released).toContain(t.id)
  })

  it('warns before rolling back an open transaction when switching connection', async () => {
    const tabs = useTabsStore()
    const ui = useUiStore()
    const t = tabs.open({ connectionId: 'dev' })
    tabs.find(t.id)!.inTransaction = true
    const p = tabs.setConnection(t.id, null)
    await Promise.resolve()
    expect(ui.confirmState?.options.title).toBe('Transação aberta')
    ui.resolveConfirm(false)
    expect(await p).toBe(false)
    expect(tabs.find(t.id)!.inTransaction).toBe(true)
  })

  it('re-executes with confirmed=true only after the user confirms', async () => {
    const tabs = useTabsStore()
    const ui = useUiStore()
    const t = tabs.open({ connectionId: 'dev' })
    executeImpl = async (req) => {
      if (!req.confirmed) {
        throw JSON.stringify({ code: 'confirmation_required', message: 'confirm', details: { production: false, environment: 'development', connection: 'dev', statements: [{ sql: 'DELETE FROM t', kind: 'write', reason: 'sem WHERE' }] } })
      }
      return response([])
    }
    const p = tabs.run(t.id, { sql: 'DELETE FROM t', cursorOffset: -1, mode: 'statement', baseLine: 1 })
    await vi.waitFor(() => expect(ui.confirmState).not.toBeNull())
    expect(tabs.find(t.id)!.status).toBe('idle')
    ui.resolveConfirm(false)
    await p
    expect(calls.map((c) => c.confirmed)).toEqual([false])
    const p2 = tabs.run(t.id, { sql: 'DELETE FROM t', cursorOffset: -1, mode: 'statement', baseLine: 1 })
    await vi.waitFor(() => expect(ui.confirmState).not.toBeNull())
    ui.resolveConfirm(true)
    await p2
    expect(calls.map((c) => c.confirmed)).toEqual([false, false, true])
  })

  it('places the error marker on the absolute editor line', async () => {
    const tabs = useTabsStore()
    const t = tabs.open({ connectionId: 'dev' })
    executeImpl = async () => ({
      executionId: 'e', database: 'db', inTransaction: false, cancelled: false, durationMs: 1,
      results: [{ index: 1, sql: 'SELECT * FROM\nWHERE', line: 3, kind: 'read', columns: null, rows: null, hasRows: false, hasMore: false, limitReached: false, durationMs: 1,
        error: { code: 1064, message: 'syntax', line: 2, category: 'syntax' } }],
    })
    // Selection starting at editor line 10; statement on line 3 of it; error on line 2 of the statement.
    await tabs.run(t.id, { sql: 'x', cursorOffset: -1, mode: 'script', baseLine: 10 })
    expect(tabs.find(t.id)!.marker?.line).toBe(13)
  })

  it('refuses to run without a connection', async () => {
    const tabs = useTabsStore()
    const t = tabs.open({ connectionId: null })
    await tabs.run(t.id, { sql: 'SELECT 1', cursorOffset: -1, mode: 'statement', baseLine: 1 })
    expect(calls).toHaveLength(0)
    expect(useUiStore().toasts[0]?.message).toContain('Selecione uma conexão')
  })

  it('closing a dirty tab asks for confirmation', async () => {
    const tabs = useTabsStore()
    const ui = useUiStore()
    const t = tabs.open({ connectionId: 'dev', sql: 'SELECT 1' })
    expect(isDirty(t)).toBe(true)
    const p = tabs.close(t.id)
    await vi.waitFor(() => expect(ui.confirmState).not.toBeNull())
    ui.resolveConfirm(false)
    expect(await p).toBe(false)
    expect(tabs.tabs).toHaveLength(1)
    tabs.markSaved(t.id, { filePath: '/tmp/x.sql' })
    expect(isDirty(tabs.find(t.id)!)).toBe(false)
    expect(await tabs.close(t.id)).toBe(true)
    expect(tabs.tabs).toHaveLength(0)
  })

  it('duplicates and renames tabs with stable unique ids', () => {
    const tabs = useTabsStore()
    const t = tabs.open({ connectionId: 'dev', sql: 'SELECT 1' })
    const d = tabs.duplicate(t.id)!
    expect(d.id).not.toBe(t.id)
    expect(d.sql).toBe('SELECT 1')
    expect(d.connectionId).toBe('dev')
    tabs.rename(d.id, '  Relatório  ')
    expect(tabs.find(d.id)!.title).toBe('Relatório')
    expect(tabs.find(t.id)!.title).not.toBe('Relatório')
  })

  it('appends pages from FetchMore to the right result', async () => {
    const tabs = useTabsStore()
    const t = tabs.open({ connectionId: 'dev' })
    executeImpl = async () => {
      const r = response([[1], [2]])
      r.results![0]!.hasMore = true
      r.results![0]!.cursorId = 'c1'
      return r
    }
    await tabs.run(t.id, { sql: 'SELECT', cursorOffset: -1, mode: 'statement', baseLine: 1 })
    await tabs.fetchMore(t.id, 0)
    const r = tabs.find(t.id)!.results[0]!
    expect(r.rows).toEqual([[1], [2], [3]])
    expect(r.hasMore).toBe(false)
  })
})

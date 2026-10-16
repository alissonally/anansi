// Actions on catalog objects shared by the tree, the database view and the
// object tabs. Every call goes through the backend policies: when the
// backend answers confirmation_required the user sees the statements (with
// type-to-confirm on production) and the call is repeated with confirmed.
import { backend } from '../api/backend'
import { toAppError } from '../api/errors'
import type { AdminResult, ConfirmationDetails, ObjectKind, ObjectRef } from '../api/types'
import { useMetadataStore } from '../stores/metadata'
import { confirmStatements, useTabsStore } from '../stores/tabs'
import { useUiStore } from '../stores/ui'
import { useTasksStore } from '../stores/tasks'
import { forceQuoteIdent } from '../utils/text'

/**
 * Runs fn(false); on confirmation_required asks the user and runs
 * fn(true). Returns undefined when the user declined.
 */
export async function withConfirmation<T>(fn: (confirmed: boolean) => Promise<T>): Promise<T | undefined> {
  try {
    return await fn(false)
  } catch (e) {
    const err = toAppError(e)
    if (err.code !== 'confirmation_required') throw err
    if (!(await confirmStatements(err.details as ConfirmationDetails))) return undefined
    return await fn(true)
  }
}

/** Shows failures of a multi-statement action; returns the failure count. */
export function reportResults(results: AdminResult[], what: string): number {
  const ui = useUiStore()
  const failed = results.filter((r) => r.error)
  if (!failed.length) {
    ui.toast('success', what)
    return 0
  }
  const first = failed[0]!
  ui.toast('error', `${failed.length} de ${results.length} falharam. ${first.error!.message} (${first.sql})`, 12000)
  return failed.length
}

export const kindLabels: Record<ObjectKind, string> = {
  database: 'database',
  table: 'tabela',
  view: 'view',
  procedure: 'procedure',
  function: 'function',
  trigger: 'trigger',
  event: 'evento',
}

/** Tab kind used to open an object. */
export function tabKindOf(kind: ObjectKind): 'database' | 'table' | 'view' | 'routine' | 'trigger' | 'event' {
  switch (kind) {
    case 'procedure':
    case 'function':
      return 'routine'
    default:
      return kind
  }
}

export function useObjectActions() {
  const ui = useUiStore()
  const meta = useMetadataStore()
  const tabs = useTabsStore()

  async function drop(conn: string, objects: ObjectRef[]): Promise<boolean> {
    if (!objects.length) return false
    try {
      const res = await withConfirmation((c) => backend.dropObjects(conn, objects, c))
      if (!res) return false
      const label = objects.length === 1 ? `${kindLabels[objects[0]!.kind]} "${objects[0]!.name}" excluído(a).` : `${objects.length} objetos excluídos.`
      reportResults(res, label)
      res.forEach((r, i) => {
        const o = objects[i]
        if (!r.error && o) {
          if (o.kind === 'database') tabs.closeObjectTabs(conn, o.name)
          else tabs.closeObjectTabs(conn, o.database, o.name)
        }
      })
      const dbs = new Set(objects.map((o) => (o.kind === 'database' ? '' : o.database)))
      for (const db of dbs) await meta.invalidate(conn, db)
      return true
    } catch (e) {
      ui.toast('error', toAppError(e).message)
      return false
    }
  }

  async function truncate(conn: string, db: string, tables: string[]): Promise<boolean> {
    if (!tables.length) return false
    const r = await ui.prompt({
      title: tables.length === 1 ? `Esvaziar "${tables[0]}"` : `Esvaziar ${tables.length} tabelas`,
      label: '',
      message: `Todas as linhas serão removidas com TRUNCATE (o AUTO_INCREMENT é reiniciado). Tabelas: ${tables.join(', ')}`,
      checkbox: { label: 'Desativar a verificação de chaves estrangeiras (FOREIGN_KEY_CHECKS = 0)', value: false },
      confirmLabel: 'Continuar',
      hideInput: true,
    })
    if (!r) return false
    try {
      const res = await withConfirmation((c) => backend.truncateTables(conn, db, tables, r.checked, c))
      if (!res) return false
      reportResults(res, tables.length === 1 ? `Tabela "${tables[0]}" esvaziada.` : `${tables.length} tabelas esvaziadas.`)
      await meta.invalidate(conn, db)
      return true
    } catch (e) {
      ui.toast('error', toAppError(e).message)
      return false
    }
  }

  async function rename(conn: string, obj: ObjectRef): Promise<string | null> {
    const r = await ui.prompt({ title: `Renomear ${kindLabels[obj.kind]}`, label: 'Novo nome', value: obj.name, confirmLabel: 'Renomear' })
    const to = r?.value.trim()
    if (!to || to === obj.name) return null
    try {
      const ok = await withConfirmation((c) => backend.renameObject(conn, obj, obj.database, to, c).then(() => true))
      if (!ok) return null
      for (const t of tabs.tabs) {
        if (t.kind !== 'query' && t.connectionId === conn && t.database === obj.database && t.objectName === obj.name) {
          t.objectName = to
          t.title = to
        }
      }
      await meta.invalidate(conn, obj.database)
      ui.toast('success', `Renomeado para "${to}".`)
      return to
    } catch (e) {
      ui.toast('error', toAppError(e).message)
      return null
    }
  }

  async function copyDdl(conn: string, obj: ObjectRef) {
    try {
      const ddl = await backend.createStatement(conn, obj.database, obj.kind, obj.name)
      await navigator.clipboard.writeText(ddl + ';')
      ui.toast('success', 'Código CREATE copiado.', 2000)
    } catch (e) {
      ui.toast('error', toAppError(e).message)
    }
  }

  async function ddlInEditor(conn: string, obj: ObjectRef) {
    try {
      const ddl = await backend.createStatement(conn, obj.database, obj.kind, obj.name)
      const needsDelimiter = obj.kind === 'procedure' || obj.kind === 'function' || obj.kind === 'trigger' || obj.kind === 'event'
      const sql = needsDelimiter ? `DELIMITER $$\n${ddl}$$\nDELIMITER ;\n` : ddl + ';\n'
      tabs.open({ title: `${obj.name} (DDL)`, connectionId: conn, database: obj.database, sql })
    } catch (e) {
      ui.toast('error', toAppError(e).message)
    }
  }

  function open(conn: string, obj: ObjectRef, view?: string) {
    const kind = tabKindOf(obj.kind)
    if (kind === 'database') {
      tabs.openObject({ kind: 'database', connectionId: conn, database: obj.name, view })
      return
    }
    tabs.openObject({
      kind, connectionId: conn, database: obj.database, objectName: obj.name,
      objectType: kind === 'routine' ? obj.kind.toUpperCase() : undefined, view,
    })
  }

  /** Opens the editor of a new object in a tab. */
  function openNew(conn: string, db: string, kind: ObjectKind) {
    const labels: Record<string, string> = { table: 'Nova tabela', view: 'Nova view', procedure: 'Nova procedure', function: 'Nova function', trigger: 'Novo trigger', event: 'Novo evento' }
    const tk = tabKindOf(kind)
    if (tk === 'database') return
    tabs.openObject({
      kind: tk, connectionId: conn, database: db, objectName: '', title: labels[kind],
      objectType: tk === 'routine' ? kind.toUpperCase() : undefined, view: tk === 'table' ? 'structure' : tk === 'view' ? 'definition' : undefined,
    })
  }

  /** Picks a SQL file and runs it in the background (HeidiSQL "Run SQL file"). */
  async function runSqlFile(conn: string, db: string) {
    try {
      const f = await backend.pickSqlFile()
      if (!f) return
      const r = await ui.prompt({
        title: `Executar ${f.name}`,
        label: '',
        hideInput: true,
        message: `${f.path}\n${(f.size / 1048576).toFixed(1)} MiB\n\nO arquivo é lido e executado aos poucos, sem abrir no editor${db ? `, a partir do database ${db}` : ''}. As instruções não são revisadas uma a uma.`,
        checkbox: { label: 'Parar no primeiro erro', value: true },
        confirmLabel: 'Executar',
      })
      if (!r) return
      const id = await withConfirmation((c) => backend.runSqlFile(conn, db, f.path, r.checked, c))
      if (id) useTasksStore().track(id, () => void meta.invalidate(conn, ''))
    } catch (e) {
      ui.toast('error', toAppError(e).message)
    }
  }

  return { drop, truncate, rename, copyDdl, ddlInEditor, open, openNew, runSqlFile, qualified: (db: string, name: string) => `${forceQuoteIdent(db)}.${forceQuoteIdent(name)}` }
}

import { monaco } from './monaco'
import { backend } from '../api/backend'
import type { CompletionItem } from '../api/types'

export interface EditorContext {
  connectionId: string | null
  database: string
}

type Kind = monaco.languages.CompletionItemKind

const kindMap: Record<string, Kind> = {
  keyword: monaco.languages.CompletionItemKind.Keyword,
  function: monaco.languages.CompletionItemKind.Function,
  type: monaco.languages.CompletionItemKind.TypeParameter,
  database: monaco.languages.CompletionItemKind.Module,
  table: monaco.languages.CompletionItemKind.Struct,
  view: monaco.languages.CompletionItemKind.Interface,
  column: monaco.languages.CompletionItemKind.Field,
  alias: monaco.languages.CompletionItemKind.Variable,
  routine: monaco.languages.CompletionItemKind.Method,
}

const kindLabel: Record<string, string> = {
  keyword: 'palavra-chave',
  function: 'função',
  type: 'tipo',
  database: 'database',
  table: 'tabela',
  view: 'view',
  column: 'coluna',
  alias: 'alias',
  routine: 'procedure',
}

export function toMonacoItem(it: CompletionItem, range: monaco.IRange): monaco.languages.CompletionItem {
  return {
    label: { label: it.label, description: kindLabel[it.kind] ?? it.kind, detail: it.detail ? `  ${it.detail}` : undefined },
    kind: kindMap[it.kind] ?? monaco.languages.CompletionItemKind.Text,
    insertText: it.insertText,
    insertTextRules: it.snippet ? monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet : undefined,
    sortText: it.sortText,
    filterText: it.label,
    detail: it.detail,
    range,
  }
}

let registered: monaco.IDisposable | null = null

/**
 * Registers the SQL completion provider. Metadata comes from the backend,
 * which caches it per connection; the provider itself is only invoked by
 * Monaco on explicit triggers (typing a word start, '.', ' ' or Ctrl+Space)
 * and Monaco filters the returned list client-side while the user keeps
 * typing, so there is no request per keystroke.
 */
export function registerCompletion(resolve: (model: monaco.editor.ITextModel) => EditorContext | null) {
  registered?.dispose()
  registered = monaco.languages.registerCompletionItemProvider('mysql', {
    triggerCharacters: ['.', '`', ' ', '('],
    async provideCompletionItems(model, position, context, token) {
      const ctx = resolve(model)
      const offset = model.getOffsetAt(position)
      // A space only triggers after keywords that introduce identifiers.
      if (context.triggerCharacter === ' ') {
        const before = model.getValueInRange({ startLineNumber: Math.max(1, position.lineNumber - 2), startColumn: 1, endLineNumber: position.lineNumber, endColumn: position.column })
        if (!/\b(from|join|update|into|use|table|call|describe|desc|on|where|and|or|by|select|set|having)\s$/i.test(before)) {
          return { suggestions: [] }
        }
      }
      try {
        const resp = await backend.complete(ctx?.connectionId ?? '', model.getValue(), offset, ctx?.database ?? '')
        if (token.isCancellationRequested) return { suggestions: [] }
        const start = model.getPositionAt(offset - resp.replaceLen)
        const range = { startLineNumber: start.lineNumber, startColumn: start.column, endLineNumber: position.lineNumber, endColumn: position.column }
        return { suggestions: (resp.items ?? []).map((it) => toMonacoItem(it, range)), incomplete: resp.incomplete }
      } catch {
        return { suggestions: [] }
      }
    },
  })
  return registered
}

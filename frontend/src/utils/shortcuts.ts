// Configurable application shortcuts (HeidiSQL Preferences > Shortcuts).
// Combos are written "Ctrl+Shift+K"; Ctrl also matches Cmd on macOS.

export interface ShortcutAction {
  id: string
  label: string
  /** Default combo ("" = none). */
  key: string
  group: 'Abas' | 'Arquivos' | 'Painéis' | 'Navegação'
}

export const shortcutActions: ShortcutAction[] = [
  { id: 'newTab', label: 'Nova aba de consulta', key: 'Ctrl+T', group: 'Abas' },
  { id: 'closeTab', label: 'Fechar aba', key: 'Ctrl+W', group: 'Abas' },
  { id: 'nextTab', label: 'Próxima aba', key: 'Ctrl+Tab', group: 'Abas' },
  { id: 'prevTab', label: 'Aba anterior', key: 'Ctrl+Shift+Tab', group: 'Abas' },
  { id: 'nextTabAlt', label: 'Próxima aba (alternativo)', key: 'Ctrl+PageDown', group: 'Abas' },
  { id: 'prevTabAlt', label: 'Aba anterior (alternativo)', key: 'Ctrl+PageUp', group: 'Abas' },
  { id: 'openFile', label: 'Abrir arquivo SQL', key: 'Ctrl+O', group: 'Arquivos' },
  { id: 'saveFile', label: 'Salvar', key: 'Ctrl+S', group: 'Arquivos' },
  { id: 'saveFileAs', label: 'Salvar como', key: 'Ctrl+Shift+S', group: 'Arquivos' },
  { id: 'preferences', label: 'Preferências', key: 'Ctrl+,', group: 'Painéis' },
  { id: 'toggleSidebar', label: 'Mostrar/ocultar barra lateral', key: 'Ctrl+B', group: 'Painéis' },
  { id: 'toggleLog', label: 'Mostrar/ocultar log de SQL', key: 'Ctrl+Alt+L', group: 'Painéis' },
  { id: 'toggleHelpers', label: 'Mostrar/ocultar painel de ajuda', key: 'Ctrl+Shift+H', group: 'Painéis' },
  { id: 'toggleTasks', label: 'Mostrar/ocultar tarefas', key: 'Ctrl+Alt+T', group: 'Painéis' },
  { id: 'focusTree', label: 'Ir para a árvore (filtro de objetos)', key: 'Ctrl+Shift+D', group: 'Navegação' },
  { id: 'focusEditor', label: 'Alternar entre editor e resultados', key: 'F6', group: 'Navegação' },
  { id: 'serverView', label: 'Abrir a visão do servidor da conexão atual', key: 'Ctrl+Shift+U', group: 'Navegação' },
  ...Array.from({ length: 9 }, (_, i) => ({ id: `tab${i + 1}`, label: `Ir para a aba ${i + 1}`, key: `Ctrl+${i + 1}`, group: 'Abas' as const })),
]

/** Shortcuts handled inside the SQL editor and grids (listed, not configurable). */
export const fixedShortcuts: { key: string; label: string }[] = [
  { key: 'Ctrl+Enter', label: 'Executar a instrução sob o cursor' },
  { key: 'Ctrl+Shift+Enter / F5', label: 'Executar o script inteiro (no editor)' },
  { key: 'Shift+Alt+F', label: 'Formatar SQL' },
  { key: 'Ctrl+Shift+E', label: 'EXPLAIN da instrução' },
  { key: 'Ctrl+Space', label: 'Autocompletar' },
  { key: 'F1', label: 'Ajuda da palavra sob o cursor' },
  { key: 'Ctrl+Alt+C', label: 'Cancelar a execução' },
  { key: 'F2 / Enter', label: 'Editar célula (grade de dados)' },
  { key: 'Ctrl+S', label: 'Salvar alterações da grade de dados' },
  { key: 'Ctrl+C', label: 'Copiar células/linhas' },
  { key: 'Delete', label: 'Excluir linhas selecionadas (grade de dados)' },
  { key: 'F5', label: 'Atualizar a lista ou os dados' },
]

/** Normalizes "ctrl + shift + k" → "Ctrl+Shift+K". */
export function normalizeCombo(s: string): string {
  const parts = s.split('+').map((p) => p.trim()).filter(Boolean)
  if (!parts.length) return ''
  const mods = new Set(parts.slice(0, -1).map((m) => m.toLowerCase()))
  let key = parts[parts.length - 1]!
  if (s.trim().endsWith('++')) key = '+'
  key = key.length === 1 ? key.toUpperCase() : key[0]!.toUpperCase() + key.slice(1)
  return [mods.has('ctrl') || mods.has('cmd') || mods.has('meta') ? 'Ctrl' : '', mods.has('alt') ? 'Alt' : '', mods.has('shift') ? 'Shift' : '', key].filter(Boolean).join('+')
}

/** Combo of a keyboard event ("" for a lone modifier). */
export function comboOf(e: KeyboardEvent): string {
  if (['Control', 'Shift', 'Alt', 'Meta'].includes(e.key)) return ''
  let key = e.key
  // Digits and letters by physical key so Shift/Alt do not change them.
  if (/^Digit\d$/.test(e.code)) key = e.code.slice(5)
  else if (/^Key[A-Z]$/.test(e.code)) key = e.code.slice(3)
  else if (key === ' ') key = 'Space'
  return normalizeCombo([(e.ctrlKey || e.metaKey) && 'Ctrl', e.altKey && 'Alt', e.shiftKey && 'Shift', key].filter(Boolean).join('+'))
}

/** Effective combo per action (overrides applied). */
export function effectiveShortcuts(overrides: Record<string, string> | null | undefined): Map<string, string> {
  const m = new Map<string, string>()
  for (const a of shortcutActions) {
    const o = overrides?.[a.id]
    m.set(a.id, o === undefined ? a.key : normalizeCombo(o))
  }
  return m
}

/** Action bound to a combo, if any. */
export function actionFor(combo: string, map: Map<string, string>): string | undefined {
  if (!combo) return undefined
  for (const [id, k] of map) if (k && k === combo) return id
  return undefined
}

/** Combos used by more than one action. */
export function conflicts(map: Map<string, string>): Map<string, string[]> {
  const by = new Map<string, string[]>()
  for (const [id, k] of map) {
    if (!k) continue
    by.set(k, [...(by.get(k) ?? []), id])
  }
  return new Map([...by].filter(([, ids]) => ids.length > 1))
}

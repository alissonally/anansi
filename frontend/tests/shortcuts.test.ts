import { describe, expect, it } from 'vitest'
import { actionFor, comboOf, conflicts, effectiveShortcuts, normalizeCombo } from '../src/utils/shortcuts'

const ev = (key: string, code: string, mods: Partial<KeyboardEvent> = {}) => ({ key, code, ctrlKey: false, metaKey: false, altKey: false, shiftKey: false, ...mods }) as KeyboardEvent

describe('shortcuts', () => {
  it('normalizes combos', () => {
    expect(normalizeCombo('shift + ctrl + k')).toBe('Ctrl+Shift+K')
    expect(normalizeCombo('cmd+,')).toBe('Ctrl+,')
    expect(normalizeCombo('alt+pagedown')).toBe('Alt+Pagedown')
    expect(normalizeCombo('')).toBe('')
  })
  it('reads events by physical key', () => {
    expect(comboOf(ev('!', 'Digit1', { ctrlKey: true, shiftKey: true }))).toBe('Ctrl+Shift+1')
    expect(comboOf(ev('t', 'KeyT', { metaKey: true }))).toBe('Ctrl+T')
    expect(comboOf(ev('Tab', 'Tab', { ctrlKey: true, shiftKey: true }))).toBe('Ctrl+Shift+Tab')
    expect(comboOf(ev('Control', 'ControlLeft', { ctrlKey: true }))).toBe('')
  })
  it('applies overrides and finds conflicts', () => {
    const m = effectiveShortcuts({ newTab: 'Ctrl+N', closeTab: '', openFile: 'Ctrl+B' })
    expect(actionFor('Ctrl+N', m)).toBe('newTab')
    expect(actionFor('Ctrl+T', m)).toBeUndefined()
    expect(actionFor('Ctrl+W', m)).toBeUndefined()
    expect([...conflicts(m).keys()]).toEqual(['Ctrl+B'])
  })
})

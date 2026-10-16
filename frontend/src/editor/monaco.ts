import * as monaco from './monaco-entry'
import EditorWorker from 'monaco-editor/editor/editor.worker?worker'

declare global {
  interface Window {
    MonacoEnvironment?: { getWorker: (workerId: string, label: string) => Worker }
  }
}

window.MonacoEnvironment = {
  getWorker: () => new EditorWorker(),
}

monaco.editor.defineTheme('studio-dark', {
  base: 'vs-dark',
  inherit: true,
  rules: [
    { token: 'keyword', foreground: '7aa2f7', fontStyle: 'bold' },
    { token: 'string', foreground: '9ece6a' },
    { token: 'number', foreground: 'ff9e64' },
    { token: 'comment', foreground: '6b7089', fontStyle: 'italic' },
    { token: 'operator', foreground: '89ddff' },
    { token: 'predefined', foreground: 'bb9af7' },
  ],
  colors: {
    'editor.background': '#16181d',
    'editor.lineHighlightBackground': '#1f222a',
    'editorLineNumber.foreground': '#4b5063',
    'editorLineNumber.activeForeground': '#a9b1d6',
    'editorGutter.background': '#16181d',
    'editorWidget.background': '#1f222a',
    'editorSuggestWidget.background': '#1f222a',
    'editorSuggestWidget.selectedBackground': '#2d3446',
  },
})

monaco.editor.defineTheme('studio-light', {
  base: 'vs',
  inherit: true,
  rules: [
    { token: 'keyword', foreground: '2f5bd3', fontStyle: 'bold' },
    { token: 'string', foreground: '3b7d1f' },
    { token: 'number', foreground: 'b45309' },
    { token: 'comment', foreground: '8a8f98', fontStyle: 'italic' },
    { token: 'predefined', foreground: '7c3aed' },
  ],
  colors: {
    'editor.background': '#ffffff',
    'editor.lineHighlightBackground': '#f4f6fa',
    'editorLineNumber.foreground': '#b0b5bf',
  },
})

export { monaco }
export type Monaco = typeof monaco

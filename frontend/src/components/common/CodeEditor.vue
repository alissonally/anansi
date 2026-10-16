<script setup lang="ts">
// Small Monaco wrapper for DDL viewers and object bodies (routines,
// triggers, views). The SQL tabs use their own single shared editor.
import { onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { monaco } from '../../editor/monaco'
import { useSettingsStore } from '../../stores/settings'

const props = withDefaults(defineProps<{ modelValue: string; readonly?: boolean; language?: string; lineNumbers?: boolean; minimal?: boolean }>(), {
  readonly: false, language: 'mysql', lineNumbers: true, minimal: false,
})
const emit = defineEmits<{ 'update:modelValue': [value: string]; save: [] }>()
const settings = useSettingsStore()
const host = ref<HTMLElement | null>(null)
const editor = shallowRef<monaco.editor.IStandaloneCodeEditor | null>(null)

onMounted(() => {
  if (!host.value) return
  const ed = monaco.editor.create(host.value, {
    value: props.modelValue,
    language: props.language,
    readOnly: props.readonly,
    theme: settings.effectiveTheme === 'dark' ? 'studio-dark' : 'studio-light',
    fontSize: settings.prefs.editorFontSize,
    fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', 'Source Code Pro', 'DejaVu Sans Mono', monospace",
    automaticLayout: true,
    minimap: { enabled: false },
    scrollBeyondLastLine: false,
    fixedOverflowWidgets: true,
    wordBasedSuggestions: 'off',
    lineNumbers: props.lineNumbers ? 'on' : 'off',
    lineNumbersMinChars: 3,
    folding: !props.minimal,
    glyphMargin: false,
    renderLineHighlight: props.readonly ? 'none' : 'line',
    padding: { top: 6 },
    tabSize: 2,
    wordWrap: 'on',
  })
  editor.value = ed
  ed.onDidChangeModelContent(() => {
    const v = ed.getValue()
    if (v !== props.modelValue) emit('update:modelValue', v)
  })
  ed.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => emit('save'))
})

watch(() => props.modelValue, (v) => {
  const ed = editor.value
  if (ed && ed.getValue() !== v) ed.setValue(v)
})
watch(() => props.readonly, (r) => editor.value?.updateOptions({ readOnly: r }))
watch(() => settings.effectiveTheme, (t) => monaco.editor.setTheme(t === 'dark' ? 'studio-dark' : 'studio-light'))
watch(() => settings.prefs.editorFontSize, (s) => editor.value?.updateOptions({ fontSize: s }))

onBeforeUnmount(() => {
  editor.value?.getModel()?.dispose()
  editor.value?.dispose()
})

defineExpose({ focus: () => editor.value?.focus() })
</script>

<template>
  <div ref="host" class="code-editor" />
</template>

<style scoped>
.code-editor {
  width: 100%;
  height: 100%;
  min-height: 60px;
  user-select: text;
}
</style>

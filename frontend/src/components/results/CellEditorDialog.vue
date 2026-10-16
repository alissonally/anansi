<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Modal from '../common/Modal.vue'
import Icon from '../common/Icon.vue'
import type { Cell, CellValue, CellValueKind } from '../../api/types'
import { insertableExpressions, isCellValue, type GridColumn } from '../../utils/grid'
import { formatBytes } from '../../utils/format'
import { backend } from '../../api/backend'
import { toAppError } from '../../api/errors'

const props = defineProps<{
  column: GridColumn
  value: CellValue | Cell
  readOnly?: boolean
  loading?: boolean
  error?: string
  /** True when the value shown is partial (full value not loaded). */
  partial?: boolean
}>()
const emit = defineEmits<{ save: [value: CellValue]; close: [] }>()

const kind = ref<CellValueKind>('value')
const text = ref('')
const wrap = ref(true)
const asText = ref(false)

const isBinary = computed(() => props.column.binary || props.column.editor === 'binary' || props.column.editor === 'spatial')

function init() {
  const v = props.value
  if (isCellValue(v)) {
    kind.value = v.kind === 'hex' ? 'value' : v.kind
    text.value = v.text
  } else if (v === null) {
    kind.value = 'null'
    text.value = ''
  } else {
    kind.value = 'value'
    const s = String(v)
    text.value = isBinary.value && /^0x[0-9a-f]*$/i.test(s) ? s.slice(2) : s
  }
}
watch(() => props.value, init, { immediate: true })

// ---- JSON ----
const jsonError = computed(() => {
  if (props.column.editor !== 'json' || kind.value !== 'value' || !text.value.trim()) return ''
  try {
    JSON.parse(text.value)
    return ''
  } catch (e) {
    return e instanceof Error ? e.message : String(e)
  }
})
function formatJson(compact: boolean) {
  try {
    text.value = JSON.stringify(JSON.parse(text.value), null, compact ? undefined : 2)
  } catch {
    // keep invalid text as typed
  }
}

// ---- binary ----
const hexError = computed(() => isBinary.value && kind.value === 'value' && !/^[0-9a-f\s]*$/i.test(text.value) ? 'Use apenas dígitos hexadecimais.' : '')
const hexClean = computed(() => text.value.replace(/\s+/g, ''))
const binaryAsText = computed(() => {
  if (!isBinary.value || hexError.value || hexClean.value.length % 2) return ''
  const bytes = new Uint8Array(hexClean.value.match(/../g)?.map((h) => parseInt(h, 16)) ?? [])
  return new TextDecoder('utf-8', { fatal: false }).decode(bytes)
})
// Image preview (PNG, JPEG, GIF, WebP, BMP) like HeidiSQL's image panel.
const imageUrl = computed(() => {
  if (!isBinary.value || hexError.value || hexClean.value.length < 16) return ''
  const h = hexClean.value.slice(0, 24).toUpperCase()
  const mime = h.startsWith('89504E47') ? 'image/png' : h.startsWith('FFD8FF') ? 'image/jpeg' : h.startsWith('47494638') ? 'image/gif'
    : h.startsWith('52494646') && h.slice(16, 24) === '57454250' ? 'image/webp' : h.startsWith('424D') ? 'image/bmp' : ''
  if (!mime) return ''
  let bin = ''
  for (let i = 0; i + 1 < hexClean.value.length; i += 2) bin += String.fromCharCode(parseInt(hexClean.value.slice(i, i + 2), 16))
  return `data:${mime};base64,${btoa(bin)}`
})

function textToHex(s: string) {
  text.value = Array.from(new TextEncoder().encode(s)).map((b) => b.toString(16).padStart(2, '0').toUpperCase()).join('')
}

// ---- SET ----
const setValues = computed({
  get: () => new Set(text.value.split(',').filter(Boolean)),
  set: (s: Set<string>) => (text.value = (props.column.options ?? []).filter((o) => s.has(o)).join(',')),
})
function toggleSet(o: string) {
  const s = new Set(setValues.value)
  if (s.has(o)) s.delete(o)
  else s.add(o)
  setValues.value = s
}

const size = computed(() => {
  if (kind.value !== 'value') return ''
  if (isBinary.value) return formatBytes(Math.floor(hexClean.value.length / 2))
  const bytes = new TextEncoder().encode(text.value).length
  return `${text.value.length.toLocaleString('pt-BR')} caracteres · ${formatBytes(bytes)}`
})
const lineBreaks = computed(() => {
  const crlf = (text.value.match(/\r\n/g) ?? []).length
  const lf = (text.value.match(/\n/g) ?? []).length - crlf
  if (!crlf && !lf) return ''
  return crlf && lf ? 'quebras de linha mistas' : crlf ? 'quebras CRLF (Windows)' : 'quebras LF (Unix)'
})

async function loadFile() {
  try {
    const f = await backend.loadFileForCell()
    if (!f) return
    kind.value = 'value'
    text.value = f.hex
    asText.value = false
  } catch (e) {
    fileError.value = toAppError(e).message
  }
}

async function saveFile(binary: boolean) {
  try {
    const path = await backend.saveCellToFile(props.column.name, binary ? hexClean.value : text.value, binary)
    if (path) fileError.value = ''
  } catch (e) {
    fileError.value = toAppError(e).message
  }
}
const fileError = ref('')

const expressions = computed(() => insertableExpressions.filter((x) => x.kinds.includes(props.column.editor)))
const invalid = computed(() => !!hexError.value || (kind.value === 'expression' && !text.value.trim()))

function save() {
  if (invalid.value || props.readOnly) return
  if (kind.value === 'value' && isBinary.value) emit('save', { kind: 'hex', text: hexClean.value })
  else emit('save', { kind: kind.value, text: kind.value === 'value' || kind.value === 'expression' ? text.value : '' })
}

function onKey(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
    e.preventDefault()
    save()
  }
}
</script>

<template>
  <Modal :title="`${column.name} — ${column.info?.columnType ?? column.type}`" :width="760" :close-on-backdrop="false" @close="emit('close')">
    <div class="ce" @keydown="onKey">
      <div class="kinds" role="radiogroup">
        <label class="check"><input v-model="kind" type="radio" value="value" :disabled="readOnly" /> Valor</label>
        <label class="check" :class="{ off: column.info && !column.info.nullable }"><input v-model="kind" type="radio" value="null" :disabled="readOnly || (column.info && !column.info.nullable)" /> NULL</label>
        <label class="check"><input v-model="kind" type="radio" value="default" :disabled="readOnly" /> DEFAULT</label>
        <label class="check" title="Expressão avaliada pelo servidor, sem aspas (ex.: NOW(), UUID())"><input v-model="kind" type="radio" value="expression" :disabled="readOnly" /> Expressão SQL</label>
        <span class="spacer" />
        <span class="faint small">{{ size }}<template v-if="lineBreaks"> · {{ lineBreaks }}</template></span>
      </div>

      <div v-if="loading" class="loading"><span class="spinner" /> Carregando o valor completo…</div>
      <div v-else-if="error || fileError" class="err"><Icon name="alert" :size="14" /> {{ error || fileError }}</div>
      <p v-if="partial && !loading" class="warn"><Icon name="alert" :size="13" /> Mostrando apenas o início do valor; salvar substituirá o valor inteiro.</p>

      <template v-if="!loading && kind === 'value'">
        <div v-if="column.editor === 'set'" class="setbox">
          <label v-for="o in column.options" :key="o" class="check"><input type="checkbox" :checked="setValues.has(o)" :disabled="readOnly" @change="toggleSet(o)" /> {{ o }}</label>
        </div>
        <template v-else-if="isBinary">
          <div class="tools">
            <label class="check"><input v-model="asText" type="checkbox" /> ver e editar como texto (UTF-8)</label>
            <span class="spacer" />
            <button v-if="!readOnly" class="btn small" type="button" title="Carregar o conteúdo de um arquivo (até 16 MiB)" @click="loadFile"><Icon name="upload" :size="11" /> Carregar arquivo…</button>
            <button class="btn small" type="button" :disabled="!!hexError" @click="saveFile(true)"><Icon name="download" :size="11" /> Salvar em arquivo…</button>
          </div>
          <div v-if="imageUrl" class="img"><img :src="imageUrl" alt="Prévia da imagem" /></div>
          <textarea v-if="!asText" v-model="text" class="area mono" :class="{ invalid: hexError }" :readonly="readOnly" spellcheck="false" placeholder="Bytes em hexadecimal, ex.: 48656C6C6F" />
          <textarea v-else :value="binaryAsText" class="area mono" :readonly="readOnly" spellcheck="false" @input="textToHex(($event.target as HTMLTextAreaElement).value)" />
          <p v-if="hexError" class="errtext">{{ hexError }}</p>
        </template>
        <template v-else>
          <div class="tools">
            <label class="check"><input v-model="wrap" type="checkbox" /> quebrar linhas</label>
            <span class="spacer" />
            <button class="btn small" type="button" @click="saveFile(false)"><Icon name="download" :size="11" /> Salvar em arquivo…</button>
            <template v-if="column.editor === 'json'">
              <button class="btn small" type="button" :disabled="readOnly" @click="formatJson(false)">Formatar JSON</button>
              <button class="btn small" type="button" :disabled="readOnly" @click="formatJson(true)">Compactar</button>
              <span v-if="jsonError" class="errtext">JSON inválido: {{ jsonError }}</span>
              <span v-else-if="text.trim()" class="ok">JSON válido</span>
            </template>
          </div>
          <textarea v-model="text" class="area mono" :class="{ nowrap: !wrap }" :readonly="readOnly" spellcheck="false" autofocus />
        </template>
      </template>
      <template v-else-if="!loading && kind === 'expression'">
        <input v-model="text" class="input mono" placeholder="ex.: NOW()" :readonly="readOnly" />
        <div v-if="expressions.length" class="exprs">
          <button v-for="x in expressions" :key="x.expr" class="btn small" type="button" :disabled="readOnly" @click="text = x.expr">{{ x.label }}</button>
        </div>
      </template>
      <p v-else-if="!loading" class="faint">{{ kind === 'null' ? 'A célula receberá NULL.' : 'A célula receberá o valor padrão da coluna.' }}</p>
    </div>
    <template #footer>
      <span class="hint">Ctrl+Enter aplica</span>
      <button class="btn" @click="emit('close')">{{ readOnly ? 'Fechar' : 'Cancelar' }}</button>
      <button v-if="!readOnly" class="btn primary" :disabled="invalid || loading" @click="save">Aplicar</button>
    </template>
  </Modal>
</template>

<style scoped>
.ce {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.kinds {
  display: flex;
  align-items: center;
  gap: 14px;
}
.off {
  opacity: 0.5;
}
.spacer {
  flex: 1;
}
.small {
  font-size: 11px;
}
.tools .spacer {
  flex: 1;
}
.tools {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.area {
  width: 100%;
  height: 340px;
  padding: 8px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius);
  background: var(--bg-input);
  resize: vertical;
  font-size: 12px;
  user-select: text;
}
.area.nowrap {
  white-space: pre;
  overflow-x: auto;
}
.area.invalid {
  border-color: var(--danger);
}
.area:focus {
  outline: none;
  border-color: var(--accent);
}
.img {
  display: flex;
  justify-content: center;
  max-height: 260px;
  padding: 6px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: repeating-conic-gradient(var(--bg-hover) 0 25%, transparent 0 50%) 0 0 / 16px 16px;
}
.img img {
  max-width: 100%;
  max-height: 248px;
  object-fit: contain;
}
.setbox {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 16px;
  padding: 8px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  max-height: 340px;
  overflow: auto;
}
.exprs {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.loading,
.err,
.warn {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0;
  font-size: 12px;
}
.err,
.errtext {
  color: var(--danger);
  font-size: 12px;
}
.warn {
  color: var(--warning);
}
.ok {
  color: var(--success);
  font-size: 12px;
}
.hint {
  margin-right: auto;
  color: var(--text-faint);
  font-size: 11px;
  align-self: center;
}
</style>

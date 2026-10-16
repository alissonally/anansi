<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { Tab } from '../../stores/tabs'
import { useMetadataStore } from '../../stores/metadata'
import { useUiStore } from '../../stores/ui'
import { backend } from '../../api/backend'
import type { CheckInfo, ColumnDef, ColumnInfo, DatabaseInfo, ForeignKeyDef, ForeignKeyInfo, IndexDef, IndexInfo, TableDef, TableInfo, TableStatus } from '../../api/types'
import CodeEditor from '../common/CodeEditor.vue'
import Icon from '../common/Icon.vue'
import ContextMenu, { type MenuItem } from '../common/ContextMenu.vue'
import { useDdlOptions } from '../dialogs/ddl'
import { useObjectEditor } from '../../composables/objectEditor'

const props = defineProps<{ tab: Tab; columns: ColumnInfo[]; indexes: IndexInfo[]; foreignKeys: ForeignKeyInfo[]; checks: CheckInfo[]; status?: TableStatus; ddl: string }>()
const emit = defineEmits<{ saved: [] }>()
const meta = useMetadataStore()
const ui = useUiStore()

const conn = computed(() => props.tab.connectionId ?? '')
const db = computed(() => props.tab.database)
const { options, charsets } = useDdlOptions(() => conn.value)

const extraTypes = ['MULTIPOINT', 'MULTILINESTRING', 'MULTIPOLYGON', 'GEOMETRYCOLLECTION']
const types = computed(() => [...(options.value?.types ?? []), ...extraTypes])
const numericTypes = new Set(['TINYINT', 'SMALLINT', 'MEDIUMINT', 'INT', 'INTEGER', 'BIGINT', 'DECIMAL', 'NUMERIC', 'FLOAT', 'DOUBLE', 'REAL'])
const integerTypes = new Set(['TINYINT', 'SMALLINT', 'MEDIUMINT', 'INT', 'INTEGER', 'BIGINT'])
const textTypes = new Set(['CHAR', 'VARCHAR', 'TINYTEXT', 'TEXT', 'MEDIUMTEXT', 'LONGTEXT', 'ENUM', 'SET'])
const temporalTypes = new Set(['DATETIME', 'TIMESTAMP'])
const noLength = new Set(['TINYTEXT', 'TEXT', 'MEDIUMTEXT', 'LONGTEXT', 'TINYBLOB', 'BLOB', 'MEDIUMBLOB', 'LONGBLOB', 'DATE', 'YEAR', 'JSON', 'BOOLEAN', 'BOOL', 'GEOMETRY', 'POINT', 'LINESTRING', 'POLYGON', 'UUID', 'INET4', 'INET6', ...extraTypes])
const suggested: Record<string, string> = { VARCHAR: '255', VARBINARY: '255', DECIMAL: '10,2', NUMERIC: '10,2', ENUM: "'a','b'", SET: "'a','b'", BIT: '1' }
const collations = computed(() => charsets.value.flatMap((c) => c.collations ?? []).sort())

function blankColumn(name = ''): ColumnDef {
  return { origName: '', name, type: 'INT', length: '', unsigned: false, zerofill: false, nullable: true, defaultKind: 'null', default: '', autoIncrement: false,
    onUpdate: '', comment: '', charset: '', collation: '', generated: '', generatedKind: '', invisible: false, srid: '' }
}

const ed = useObjectEditor<TableDef>(() => props.tab, {
  load: () => backend.loadTableDef(conn.value, db.value, props.tab.objectName ?? ''),
  blank: () => ({
    database: db.value, name: 'nova_tabela', origName: '', mariadb: false,
    columns: [{ ...blankColumn('id'), type: 'INT', unsigned: true, nullable: false, defaultKind: '', autoIncrement: true }],
    indexes: [{ origName: '', name: '', kind: 'PRIMARY', parts: [{ column: 'id', length: 0, desc: false, expression: '' }], algorithm: '', comment: '', invisible: false }],
    foreignKeys: [], checks: [],
    options: { engine: options.value?.defaultEngine ?? '', collation: options.value?.defaultCollation ?? '', comment: '', rowFormat: '', autoIncrement: '', avgRowLength: '', maxRows: '', checksum: false, partition: '', convertData: false },
  }),
  preview: (d) => backend.previewTableDef(conn.value, d),
  save: (d, c) => backend.saveTableDef(conn.value, d, c),
  nameOf: (d) => d.name,
})
const { def, loading, loadError, saving, failure, preview, previewError, dirty, isNew } = ed
void ed.load()
watch(() => props.tab.objectName, (n, o) => { if (n !== o && !ed.saving.value) void ed.load() })

type Section = 'columns' | 'indexes' | 'fks' | 'checks' | 'options' | 'partition' | 'sql'
const section = ref<Section>('columns')
const selCol = ref(0)
const menu = ref<{ x: number; y: number; items: MenuItem[] } | null>(null)

const cols = computed(() => def.value?.columns ?? [])
const colNames = computed(() => cols.value.map((c) => c.name).filter(Boolean))
function isPk(name: string) {
  return (def.value?.indexes ?? []).some((i) => i.kind === 'PRIMARY' && (i.parts ?? []).some((p) => p.column === name))
}

// ---- columns ----
function addColumn() {
  const d = def.value
  if (!d) return
  const list = d.columns ?? (d.columns = [])
  const src = list[selCol.value]
  const c = blankColumn(`coluna${list.length + 1}`)
  if (src) {
    c.type = src.type
    c.length = src.length
    c.unsigned = src.unsigned
    c.nullable = src.nullable || src.autoIncrement
    c.defaultKind = c.nullable ? 'null' : ''
  }
  list.splice(selCol.value + 1, 0, c)
  selCol.value = Math.min(selCol.value + 1, list.length - 1)
}

function removeColumn(i = selCol.value) {
  const d = def.value
  const c = d?.columns?.[i]
  if (!d || !c) return
  d.columns!.splice(i, 1)
  // Drop index parts / FK columns that used it.
  for (const ix of d.indexes ?? []) ix.parts = (ix.parts ?? []).filter((p) => p.column !== c.name || !!p.expression)
  d.indexes = (d.indexes ?? []).filter((ix) => (ix.parts ?? []).length)
  selCol.value = Math.max(0, Math.min(i, d.columns!.length - 1))
}

function moveColumn(delta: number) {
  const list = def.value?.columns
  const i = selCol.value
  const j = i + delta
  if (!list || j < 0 || j >= list.length) return
  ;[list[i], list[j]] = [list[j]!, list[i]!]
  selCol.value = j
}

function renameRefs(oldName: string, newName: string) {
  if (!oldName || oldName === newName) return
  for (const ix of def.value?.indexes ?? []) for (const p of ix.parts ?? []) if (p.column === oldName) p.column = newName
  for (const fk of def.value?.foreignKeys ?? []) fk.columns = (fk.columns ?? []).map((c) => (c === oldName ? newName : c))
}

function onName(c: ColumnDef, e: Event) {
  const v = (e.target as HTMLInputElement).value
  renameRefs(c.name, v)
  c.name = v
}

function onType(c: ColumnDef) {
  const t = c.type.toUpperCase()
  c.type = t
  // Like HeidiSQL: changing the type resets the length to the type's usual one.
  c.length = noLength.has(t) ? '' : (suggested[t] ?? '')
  if (!numericTypes.has(t)) {
    c.unsigned = false
    c.zerofill = false
  }
  if (!integerTypes.has(t)) c.autoIncrement = false
  if (!textTypes.has(t)) c.collation = ''
  if (!temporalTypes.has(t)) c.onUpdate = ''
  if (c.defaultKind === 'value' && c.default && numericTypes.has(t) && !/^-?\d+(\.\d+)?$/.test(c.default)) c.default = ''
}

type DefaultChoice = '' | 'null' | 'value' | 'expression' | 'auto'
function defaultChoice(c: ColumnDef): DefaultChoice {
  return c.autoIncrement ? 'auto' : c.defaultKind
}
function setDefaultChoice(c: ColumnDef, v: DefaultChoice) {
  c.autoIncrement = v === 'auto'
  c.defaultKind = v === 'auto' ? '' : v
  if (v === 'null') c.nullable = true
  if (v === 'expression' && !c.default && temporalTypes.has(c.type)) c.default = 'CURRENT_TIMESTAMP'
}

function colMenu(e: MouseEvent, i: number) {
  e.preventDefault()
  selCol.value = i
  const c = cols.value[i]!
  const idx = def.value?.indexes ?? []
  const mk = (kind: IndexDef['kind']) => () => {
    if (kind === 'PRIMARY' && idx.some((x) => x.kind === 'PRIMARY')) {
      ui.toast('warning', 'A tabela já tem chave primária; use "Adicionar ao índice".')
      return
    }
    if (kind === 'PRIMARY') {
      c.nullable = false
      if (c.defaultKind === 'null') c.defaultKind = ''
    }
    idx.push({ origName: '', name: kind === 'PRIMARY' ? '' : `${kind === 'UNIQUE' ? 'uq' : kind === 'FULLTEXT' ? 'ft' : 'idx'}_${c.name}`, kind, parts: [{ column: c.name, length: 0, desc: false, expression: '' }], algorithm: '', comment: '', invisible: false })
    def.value!.indexes = idx
    ui.toast('info', `Índice criado na aba Índices.`, 2000)
  }
  menu.value = {
    x: e.clientX, y: e.clientY, items: [
      { label: 'Adicionar coluna', icon: 'plus', action: addColumn },
      { label: 'Remover coluna', icon: 'trash', danger: true, action: () => removeColumn(i) },
      { label: 'Mover para cima', icon: 'arrowUp', action: () => moveColumn(-1), disabled: i === 0 },
      { label: 'Mover para baixo', icon: 'arrowDown', action: () => moveColumn(1), disabled: i === cols.value.length - 1 },
      { separator: true, label: '' },
      { label: 'Criar índice', icon: 'index', children: [
        { label: 'PRIMARY KEY', action: mk('PRIMARY') },
        { label: 'UNIQUE', action: mk('UNIQUE') },
        { label: 'KEY (índice simples)', action: mk('KEY') },
        { label: 'FULLTEXT', action: mk('FULLTEXT') },
        { label: 'SPATIAL', action: mk('SPATIAL') },
      ] },
      { label: 'Adicionar ao índice', disabled: !idx.length, children: idx.map((x) => ({
        label: x.kind === 'PRIMARY' ? 'PRIMARY' : x.name || '(sem nome)', hint: (x.parts ?? []).map((p) => p.column || p.expression).join(', '),
        action: () => { (x.parts ?? (x.parts = [])).push({ column: c.name, length: 0, desc: false, expression: '' }) },
      })) },
    ],
  }
}

// ---- indexes ----
function addIndex() {
  const d = def.value
  if (!d) return
  const c = cols.value[selCol.value]
  ;(d.indexes ?? (d.indexes = [])).push({ origName: '', name: c ? `idx_${c.name}` : 'novo_indice', kind: 'KEY', parts: c ? [{ column: c.name, length: 0, desc: false, expression: '' }] : [], algorithm: '', comment: '', invisible: false })
}
function addPart(ix: IndexDef, expression = false) {
  ;(ix.parts ?? (ix.parts = [])).push({ column: expression ? '' : (colNames.value[0] ?? ''), length: 0, desc: false, expression: expression ? '(expressão)' : '' })
}
function moveInList<T>(list: T[] | null, i: number, d: number) {
  if (!list) return
  const j = i + d
  if (j < 0 || j >= list.length) return
  ;[list[i], list[j]] = [list[j]!, list[i]!]
}

// ---- foreign keys ----
const dbs = computed(() => (meta.entry<DatabaseInfo[]>(conn.value, 'dbs')?.data ?? []).map((d) => d.name))
function tablesOf(database: string) {
  return (meta.entry<TableInfo[]>(conn.value, 'tables', database || db.value)?.data ?? []).filter((t) => t.type === 'table').map((t) => t.name)
}
function refColumnsOf(fk: ForeignKeyDef) {
  return (meta.entry<ColumnInfo[]>(conn.value, 'cols', fk.refDatabase || db.value, fk.refTable)?.data ?? []).map((c) => c.name)
}
watch(() => (def.value?.foreignKeys ?? []).map((f) => `${f.refDatabase}.${f.refTable}`), () => {
  for (const fk of def.value?.foreignKeys ?? []) {
    const d = fk.refDatabase || db.value
    void meta.tables(conn.value, d).catch(() => undefined)
    if (fk.refTable) void meta.columns(conn.value, d, fk.refTable).catch(() => undefined)
  }
}, { immediate: true })
void meta.databases(conn.value).catch(() => undefined)
void meta.tables(conn.value, db.value).catch(() => undefined)

function addFk() {
  const d = def.value
  if (!d) return
  const c = cols.value[selCol.value]
  ;(d.foreignKeys ?? (d.foreignKeys = [])).push({ origName: '', name: '', columns: c ? [c.name] : [], refDatabase: '', refTable: '', refColumns: [], onUpdate: '', onDelete: '' })
}
function toggleIn(list: string[] | null, v: string): string[] {
  const s = list ?? []
  return s.includes(v) ? s.filter((x) => x !== v) : [...s, v]
}

function addCheck() {
  const d = def.value
  if (!d) return
  ;(d.checks ?? (d.checks = [])).push({ origName: '', name: '', clause: '' })
}

const counts = computed(() => ({
  columns: cols.value.length, indexes: def.value?.indexes?.length ?? 0, fks: def.value?.foreignKeys?.length ?? 0, checks: def.value?.checks?.length ?? 0,
}))
const sqlText = computed(() => (previewError.value ? `-- ${previewError.value}` : preview.value.length ? preview.value.map((s) => s + ';').join('\n\n') : '-- Nenhuma alteração.'))

async function save() {
  if (await ed.save()) emit('saved')
}

function onKey(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
    e.preventDefault()
    void save()
  }
}
</script>

<template>
  <div class="te" @keydown="onKey">
    <div v-if="loading && !def" class="center"><span class="spinner" /> Carregando estrutura…</div>
    <div v-else-if="loadError" class="center err"><Icon name="alert" :size="16" /> {{ loadError }}</div>
    <template v-else-if="def">
      <div class="top">
        <label class="field name"><span>Nome</span><input v-model="def.name" class="input" spellcheck="false" /></label>
        <label class="field grow"><span>Comentário</span><input v-model="def.options.comment" class="input" /></label>
        <div class="actions">
          <span v-if="previewError" class="perr ellipsis" :title="previewError"><Icon name="alert" :size="12" /> {{ previewError }}</span>
          <button class="btn small" :disabled="!dirty" @click="ed.discard">Descartar</button>
          <button class="btn small primary" :disabled="(!dirty && !isNew) || saving || !!previewError" title="Ctrl+S" @click="save">
            <span v-if="saving" class="spinner" /><Icon v-else name="save" :size="12" /> {{ isNew ? 'Criar tabela' : 'Salvar' }}
          </button>
        </div>
      </div>
      <div v-if="failure" class="banner">
        <Icon name="alert" :size="13" />
        <div class="selectable"><strong>Erro:</strong> {{ failure.message }}<code class="mono">{{ failure.sql }}</code></div>
        <button class="icon-btn" aria-label="Fechar" @click="failure = null"><Icon name="x" :size="12" /></button>
      </div>
      <nav class="sections" role="tablist">
        <button v-for="s in ([['columns', 'Colunas'], ['indexes', 'Índices'], ['fks', 'Chaves estrangeiras'], ['checks', 'CHECK'], ['options', 'Opções'], ['partition', 'Particionamento'], ['sql', isNew ? 'CREATE' : 'ALTER']] as [Section, string][])" :key="s[0]"
          role="tab" :class="{ active: section === s[0] }" :aria-selected="section === s[0]" @click="section = s[0]">
          {{ s[1] }}<span v-if="s[0] in counts" class="n">{{ counts[s[0] as keyof typeof counts] }}</span>
          <span v-if="s[0] === 'sql' && preview.length && !isNew" class="n warn">{{ preview.length }}</span>
        </button>
      </nav>

      <!-- columns -->
      <div v-if="section === 'columns'" class="pane">
        <div class="bar">
          <button class="btn small" @click="addColumn"><Icon name="plus" :size="12" /> Adicionar</button>
          <button class="btn small" :disabled="!cols.length" @click="removeColumn()"><Icon name="trash" :size="12" /> Remover</button>
          <button class="btn small" :disabled="selCol === 0" @click="moveColumn(-1)"><Icon name="arrowUp" :size="12" /></button>
          <button class="btn small" :disabled="selCol >= cols.length - 1" @click="moveColumn(1)"><Icon name="arrowDown" :size="12" /></button>
          <span class="hint">Clique direito numa coluna para criar índices.</span>
        </div>
        <div class="scroll">
          <table class="grid">
            <thead>
              <tr>
                <th>#</th><th>Nome</th><th>Tipo</th><th>Tamanho / valores</th><th title="UNSIGNED">Uns.</th><th title="ZEROFILL">Zero</th><th>NULL</th>
                <th>Padrão</th><th>ON UPDATE</th><th>Collation</th><th>Coluna gerada</th><th title="INVISIBLE">Invis.</th><th>Comentário</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(c, i) in cols" :key="i" :class="{ sel: selCol === i, added: !c.origName }" @mousedown="selCol = i" @contextmenu="colMenu($event, i)">
                <td class="num">{{ i + 1 }}<Icon v-if="isPk(c.name)" name="key" :size="11" class="pk" /></td>
                <td><input :value="c.name" class="cell mono" spellcheck="false" @change="onName(c, $event)" /></td>
                <td>
                  <select v-model="c.type" class="cell" @change="onType(c)">
                    <option v-if="!types.includes(c.type)" :value="c.type">{{ c.type }}</option>
                    <option v-for="t in types" :key="t" :value="t">{{ t }}</option>
                  </select>
                </td>
                <td><input v-model="c.length" class="cell mono" :disabled="noLength.has(c.type) && !c.length" spellcheck="false" :placeholder="suggested[c.type] ?? ''" /></td>
                <td class="ck"><input v-model="c.unsigned" type="checkbox" :disabled="!numericTypes.has(c.type)" /></td>
                <td class="ck"><input v-model="c.zerofill" type="checkbox" :disabled="!numericTypes.has(c.type)" /></td>
                <td class="ck"><input v-model="c.nullable" type="checkbox" :disabled="isPk(c.name)" @change="!c.nullable && c.defaultKind === 'null' && (c.defaultKind = '')" /></td>
                <td class="dflt">
                  <select :value="defaultChoice(c)" class="cell kind" :disabled="!!c.generated" @change="setDefaultChoice(c, ($event.target as HTMLSelectElement).value as DefaultChoice)">
                    <option value="">sem padrão</option>
                    <option value="null" :disabled="!c.nullable">NULL</option>
                    <option value="value">valor</option>
                    <option value="expression">expressão</option>
                    <option value="auto" :disabled="!integerTypes.has(c.type)">AUTO_INCREMENT</option>
                  </select>
                  <input v-if="c.defaultKind === 'value' || c.defaultKind === 'expression'" v-model="c.default" class="cell mono" spellcheck="false" :placeholder="c.defaultKind === 'expression' ? 'CURRENT_TIMESTAMP' : 'valor'" />
                </td>
                <td>
                  <select v-model="c.onUpdate" class="cell" :disabled="!temporalTypes.has(c.type)">
                    <option value="" />
                    <option value="CURRENT_TIMESTAMP">CURRENT_TIMESTAMP</option>
                    <option v-for="n in [1, 2, 3, 4, 5, 6]" :key="n" :value="`CURRENT_TIMESTAMP(${n})`">CURRENT_TIMESTAMP({{ n }})</option>
                  </select>
                </td>
                <td>
                  <select v-model="c.collation" class="cell" :disabled="!textTypes.has(c.type)">
                    <option value="">(padrão da tabela)</option>
                    <option v-for="x in collations" :key="x" :value="x">{{ x }}</option>
                  </select>
                </td>
                <td class="gen">
                  <input v-model="c.generated" class="cell mono" placeholder="expressão" spellcheck="false" />
                  <select v-if="c.generated" v-model="c.generatedKind" class="cell kind">
                    <option value="VIRTUAL">VIRTUAL</option>
                    <option value="STORED">{{ def.mariadb ? 'PERSISTENT' : 'STORED' }}</option>
                  </select>
                </td>
                <td class="ck"><input v-model="c.invisible" type="checkbox" /></td>
                <td><input v-model="c.comment" class="cell" /></td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- indexes -->
      <div v-else-if="section === 'indexes'" class="pane">
        <div class="bar"><button class="btn small" @click="addIndex"><Icon name="plus" :size="12" /> Novo índice</button></div>
        <div class="scroll cards">
          <div v-for="(ix, i) in def.indexes ?? []" :key="i" class="card" :class="{ added: !ix.origName }">
            <div class="row">
              <select v-model="ix.kind" class="select compact">
                <option v-for="k in ['PRIMARY', 'KEY', 'UNIQUE', 'FULLTEXT', 'SPATIAL']" :key="k" :value="k">{{ k }}</option>
              </select>
              <input v-if="ix.kind !== 'PRIMARY'" v-model="ix.name" class="input compact mono" placeholder="nome (automático)" spellcheck="false" />
              <select v-model="ix.algorithm" class="select compact" title="Algoritmo">
                <option value="">algoritmo padrão</option><option value="BTREE">BTREE</option><option value="HASH">HASH</option>
              </select>
              <input v-model="ix.comment" class="input compact" placeholder="comentário" />
              <label v-if="ix.kind !== 'PRIMARY'" class="check" :title="def.mariadb ? 'IGNORED' : 'INVISIBLE'"><input v-model="ix.invisible" type="checkbox" /> invisível</label>
              <span class="spacer" />
              <button class="icon-btn" title="Remover índice" aria-label="Remover índice" @click="def.indexes!.splice(i, 1)"><Icon name="trash" :size="13" /></button>
            </div>
            <table class="parts">
              <tr v-for="(p, j) in ix.parts ?? []" :key="j">
                <td>
                  <input v-if="p.expression" v-model="p.expression" class="input compact mono" title="Parte funcional (MySQL 8.0.13+)" />
                  <select v-else v-model="p.column" class="select compact">
                    <option v-for="n in colNames" :key="n" :value="n">{{ n }}</option>
                  </select>
                </td>
                <td><input v-model.number="p.length" type="number" min="0" class="input compact num" title="Prefixo (caracteres)" :disabled="!!p.expression" /></td>
                <td><label class="check"><input v-model="p.desc" type="checkbox" /> DESC</label></td>
                <td>
                  <button class="icon-btn" aria-label="Subir" @click="moveInList(ix.parts, j, -1)"><Icon name="arrowUp" :size="12" /></button>
                  <button class="icon-btn" aria-label="Descer" @click="moveInList(ix.parts, j, 1)"><Icon name="arrowDown" :size="12" /></button>
                  <button class="icon-btn" aria-label="Remover parte" @click="ix.parts!.splice(j, 1)"><Icon name="x" :size="12" /></button>
                </td>
              </tr>
            </table>
            <div class="row">
              <button class="btn small" @click="addPart(ix)"><Icon name="plus" :size="11" /> Coluna</button>
              <button class="btn small" title="Índice funcional (MySQL 8.0.13+)" @click="addPart(ix, true)"><Icon name="plus" :size="11" /> Expressão</button>
            </div>
          </div>
          <p v-if="!def.indexes?.length" class="faint">Nenhum índice. Tabelas InnoDB sem chave primária usam uma chave interna oculta.</p>
        </div>
      </div>

      <!-- foreign keys -->
      <div v-else-if="section === 'fks'" class="pane">
        <div class="bar"><button class="btn small" @click="addFk"><Icon name="plus" :size="12" /> Nova chave estrangeira</button></div>
        <div class="scroll cards">
          <div v-for="(fk, i) in def.foreignKeys ?? []" :key="i" class="card" :class="{ added: !fk.origName }">
            <div class="row">
              <input v-model="fk.name" class="input compact mono" placeholder="nome (automático)" spellcheck="false" />
              <span class="lbl">ON UPDATE</span>
              <select v-model="fk.onUpdate" class="select compact"><option v-for="a in ['', 'RESTRICT', 'CASCADE', 'SET NULL', 'NO ACTION']" :key="a" :value="a">{{ a || '(padrão)' }}</option></select>
              <span class="lbl">ON DELETE</span>
              <select v-model="fk.onDelete" class="select compact"><option v-for="a in ['', 'RESTRICT', 'CASCADE', 'SET NULL', 'NO ACTION']" :key="a" :value="a">{{ a || '(padrão)' }}</option></select>
              <span class="spacer" />
              <button class="icon-btn" aria-label="Remover chave" @click="def.foreignKeys!.splice(i, 1)"><Icon name="trash" :size="13" /></button>
            </div>
            <div class="fkgrid">
              <div>
                <div class="lbl">Colunas desta tabela (na ordem)</div>
                <div class="chips">
                  <label v-for="n in colNames" :key="n" class="check"><input type="checkbox" :checked="fk.columns?.includes(n)" @change="fk.columns = toggleIn(fk.columns, n)" /> {{ n }}</label>
                </div>
              </div>
              <div>
                <div class="lbl">Referencia</div>
                <div class="row">
                  <select v-model="fk.refDatabase" class="select compact" title="Database"><option value="">{{ db }}</option><option v-for="d in dbs.filter((x) => x !== db)" :key="d" :value="d">{{ d }}</option></select>
                  <select v-model="fk.refTable" class="select compact" @change="fk.refColumns = []"><option value="" disabled>tabela…</option><option v-for="t in tablesOf(fk.refDatabase)" :key="t" :value="t">{{ t }}</option></select>
                </div>
                <div class="chips">
                  <label v-for="n in refColumnsOf(fk)" :key="n" class="check"><input type="checkbox" :checked="fk.refColumns?.includes(n)" @change="fk.refColumns = toggleIn(fk.refColumns, n)" /> {{ n }}</label>
                </div>
              </div>
            </div>
            <p class="mono faint small">({{ (fk.columns ?? []).join(', ') }}) → {{ fk.refTable || '?' }} ({{ (fk.refColumns ?? []).join(', ') }})</p>
          </div>
          <p v-if="!def.foreignKeys?.length" class="faint">Nenhuma chave estrangeira.</p>
        </div>
      </div>

      <!-- checks -->
      <div v-else-if="section === 'checks'" class="pane">
        <div class="bar"><button class="btn small" @click="addCheck"><Icon name="plus" :size="12" /> Nova restrição</button></div>
        <div class="scroll cards">
          <div v-for="(c, i) in def.checks ?? []" :key="i" class="card row" :class="{ added: !c.origName }">
            <input v-model="c.name" class="input compact mono" placeholder="nome (automático)" spellcheck="false" />
            <span class="lbl">CHECK (</span>
            <input v-model="c.clause" class="input compact mono grow" placeholder="ex.: preco >= 0" spellcheck="false" />
            <span class="lbl">)</span>
            <button class="icon-btn" aria-label="Remover restrição" @click="def.checks!.splice(i, 1)"><Icon name="trash" :size="13" /></button>
          </div>
          <p v-if="!def.checks?.length" class="faint">Nenhuma restrição CHECK (MySQL 8.0.16+ e MariaDB 10.2+).</p>
        </div>
      </div>

      <!-- options -->
      <div v-else-if="section === 'options'" class="pane scroll form">
        <label class="field"><span>Engine</span>
          <select v-model="def.options.engine" class="select"><option value="">(padrão do servidor)</option><option v-for="e in options?.engines ?? []" :key="e" :value="e">{{ e }}</option></select>
        </label>
        <label class="field"><span>Collation padrão</span>
          <select v-model="def.options.collation" class="select"><option value="">(padrão do database)</option><option v-for="x in collations" :key="x" :value="x">{{ x }}</option></select>
        </label>
        <label v-if="!isNew" class="check"><input v-model="def.options.convertData" type="checkbox" /> Converter os dados existentes (CONVERT TO CHARACTER SET) em vez de mudar só o padrão</label>
        <label class="field"><span>Formato de linha</span>
          <select v-model="def.options.rowFormat" class="select"><option v-for="f in ['', 'DEFAULT', 'DYNAMIC', 'COMPACT', 'COMPRESSED', 'REDUNDANT', 'FIXED']" :key="f" :value="f">{{ f || '(padrão)' }}</option></select>
        </label>
        <label class="field"><span>Próximo AUTO_INCREMENT</span><input v-model="def.options.autoIncrement" class="input mono" inputmode="numeric" /></label>
        <label class="field"><span>Tamanho médio de linha (AVG_ROW_LENGTH)</span><input v-model="def.options.avgRowLength" class="input mono" inputmode="numeric" /></label>
        <label class="field"><span>Máximo de linhas (MAX_ROWS)</span><input v-model="def.options.maxRows" class="input mono" inputmode="numeric" /></label>
        <label class="check"><input v-model="def.options.checksum" type="checkbox" /> CHECKSUM (manter checksum das linhas, MyISAM/Aria)</label>
      </div>

      <!-- partition -->
      <div v-else-if="section === 'partition'" class="pane col">
        <p class="hint pad">Cláusula completa, ex.: <code class="mono">PARTITION BY RANGE (ano) (PARTITION p0 VALUES LESS THAN (2020), PARTITION p1 VALUES LESS THAN MAXVALUE)</code>. Deixe vazio para remover o particionamento (REMOVE PARTITIONING).</p>
        <div class="grow"><CodeEditor v-model="def.options.partition" /></div>
      </div>

      <!-- sql -->
      <div v-else class="pane col"><div class="grow"><CodeEditor :model-value="sqlText" readonly /></div></div>
    </template>
    <ContextMenu v-if="menu" :x="menu.x" :y="menu.y" :items="menu.items" @close="menu = null" />
  </div>
</template>

<style scoped>
.te {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}
.center {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  height: 100%;
}
.center.err {
  color: var(--danger);
}
.top {
  display: flex;
  align-items: flex-end;
  gap: 10px;
  padding: 8px 10px;
  border-bottom: 1px solid var(--border);
  flex-wrap: wrap;
}
.top .name {
  width: 240px;
}
.grow {
  flex: 1;
  min-width: 0;
  min-height: 0;
}
.actions {
  display: flex;
  align-items: center;
  gap: 6px;
}
.perr {
  color: var(--danger);
  font-size: 12px;
  max-width: 380px;
}
.banner {
  display: flex;
  gap: 8px;
  padding: 6px 10px;
  background: var(--danger-soft);
  font-size: 12px;
  border-bottom: 1px solid var(--border);
}
.banner > div {
  flex: 1;
}
.banner > .icon {
  color: var(--danger);
}
.banner code {
  display: block;
  white-space: pre-wrap;
  font-size: 11px;
  margin-top: 2px;
}
.sections {
  display: flex;
  gap: 2px;
  padding: 0 8px;
  border-bottom: 1px solid var(--border);
  overflow-x: auto;
}
.sections button {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 10px;
  border: none;
  border-bottom: 2px solid transparent;
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  white-space: nowrap;
}
.sections button.active {
  color: var(--text);
  border-bottom-color: var(--accent);
}
.n {
  font-size: 10px;
  padding: 0 5px;
  border-radius: 8px;
  background: var(--bg-active);
}
.n.warn {
  background: var(--warning-soft);
  color: var(--warning);
}
.pane {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.pane.col .grow {
  flex: 1;
}
.bar {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 8px;
  border-bottom: 1px solid var(--border);
}
.hint {
  color: var(--text-muted);
  font-size: 12px;
}
.hint.pad {
  margin: 0;
  padding: 8px 10px;
}
.scroll {
  flex: 1;
  min-height: 0;
  overflow: auto;
}
table.grid {
  border-collapse: collapse;
  font-size: 12px;
  min-width: 100%;
}
.grid th {
  position: sticky;
  top: 0;
  z-index: 1;
  background: var(--bg-elev);
  text-align: left;
  font-weight: 600;
  padding: 4px 6px;
  border-bottom: 1px solid var(--border-strong);
  white-space: nowrap;
}
.grid td {
  padding: 1px 2px;
  border-bottom: 1px solid var(--border);
  white-space: nowrap;
}
.grid tr.sel td {
  background: var(--selection);
}
.grid tr.added td:first-child {
  box-shadow: inset 3px 0 0 var(--success);
}
.grid td.num {
  text-align: right;
  color: var(--text-faint);
  padding-right: 6px;
}
.pk {
  display: inline;
  color: var(--warning);
  margin-left: 3px;
}
.grid td.ck {
  text-align: center;
}
.cell {
  height: 24px;
  width: 100%;
  min-width: 70px;
  padding: 0 5px;
  border: 1px solid transparent;
  border-radius: 3px;
  background: transparent;
  user-select: text;
}
.cell:hover,
.cell:focus {
  border-color: var(--border-strong);
  background: var(--bg-input);
  outline: none;
}
.cell:disabled {
  opacity: 0.4;
}
td .cell.mono {
  min-width: 120px;
}
.dflt,
.gen {
  display: flex;
  gap: 2px;
}
.cell.kind {
  min-width: 96px;
  width: auto;
}
.cards {
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.card {
  padding: 8px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--bg-elev);
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.card.added {
  border-left: 3px solid var(--success);
}
.card.row,
.row {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-direction: row;
  flex-wrap: wrap;
}
.input.compact {
  height: 26px;
}
.input.num {
  width: 70px;
}
.spacer {
  flex: 1;
}
.lbl {
  font-size: 11px;
  color: var(--text-muted);
}
.parts td {
  padding: 1px 4px;
}
.fkgrid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}
.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  margin-top: 4px;
}
.small {
  font-size: 11px;
  margin: 0;
}
.form {
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-width: 560px;
}
</style>

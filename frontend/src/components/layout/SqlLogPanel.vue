<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useVirtualizer } from '@tanstack/vue-virtual'
import { useSqlLogStore } from '../../stores/sqllog'
import { useConnectionsStore } from '../../stores/connections'
import { useTabsStore } from '../../stores/tabs'
import { useUiStore } from '../../stores/ui'
import Icon from '../common/Icon.vue'
import { formatDuration } from '../../utils/text'
import { textMatcher } from '../../utils/format'

const emit = defineEmits<{ close: [] }>()
const log = useSqlLogStore()
const conns = useConnectionsStore()
const tabs = useTabsStore()
const ui = useUiStore()

const conn = ref('')
const filter = ref('')
const onlyErrors = ref(false)
const hideMeta = ref(false)
const follow = ref(true)
const scrollEl = ref<HTMLElement | null>(null)

// Statements issued by the app itself to read the catalog.
const metaRe = /information_schema\.|^SHOW (FULL |GLOBAL |SESSION )?(VARIABLES|STATUS|PROCESSLIST|CREATE)|^SELECT (CONNECTION_ID|DATABASE|VERSION)\(\)|information_schema_stats_expiry|^SET @OLD_FOREIGN|^SET FOREIGN_KEY_CHECKS = @OLD/i

const visible = computed(() => {
  const m = textMatcher(filter.value)
  return log.entries.filter((e) => (!conn.value || e.connectionId === conn.value) && (!onlyErrors.value || e.error) &&
    (!hideMeta.value || !metaRe.test(e.sql)) && m(e.sql))
})

const ROW = 20
const virtualizer = useVirtualizer(computed(() => ({ count: visible.value.length, getScrollElement: () => scrollEl.value, estimateSize: () => ROW, overscan: 10 })))

watch(() => visible.value.length, async () => {
  if (!follow.value) return
  await nextTick()
  scrollEl.value?.scrollTo({ top: scrollEl.value.scrollHeight })
})

function onScroll() {
  const el = scrollEl.value
  if (el) follow.value = el.scrollTop + el.clientHeight >= el.scrollHeight - ROW * 2
}

function time(at: string) {
  const d = new Date(at)
  return d.toLocaleTimeString('pt-BR', { hour12: false }) + '.' + String(d.getMilliseconds()).padStart(3, '0')
}

function connName(id: string) {
  return conns.byId.get(id)?.name ?? id.slice(0, 8)
}

async function copyAll() {
  const text = visible.value.map((e) => `/* ${time(e.at)} ${connName(e.connectionId)} ${formatDuration(e.durationMs)} */ ${e.sql};${e.error ? `\n/* erro: ${e.error} */` : ''}`).join('\n')
  await navigator.clipboard.writeText(text).then(() => ui.toast('success', 'Log copiado.', 2000)).catch(() => ui.toast('error', 'Não foi possível copiar.'))
}

function toEditor(sql: string, connectionId: string) {
  tabs.open({ connectionId, sql: sql.replace(/\s+\/\* .* \*\/$/, '') + ';\n' })
}

onMounted(() => void log.start())
</script>

<template>
  <section class="log" aria-label="Log de SQL">
    <header>
      <strong><Icon name="terminal" :size="13" /> Log de SQL</strong>
      <select v-model="conn" class="select compact" aria-label="Conexão">
        <option value="">Todas as conexões</option>
        <option v-for="p in conns.profiles" :key="p.id" :value="p.id">{{ p.name }}</option>
      </select>
      <input v-model="filter" class="input compact" placeholder="Filtrar (regex; ; = ou)" aria-label="Filtrar log" />
      <label class="check"><input v-model="onlyErrors" type="checkbox" /> só erros</label>
      <label class="check" title="Oculta as consultas internas de catálogo"><input v-model="hideMeta" type="checkbox" /> ocultar catálogo</label>
      <span class="faint count">{{ visible.length }} / {{ log.entries.length }}</span>
      <span class="spacer" />
      <button class="icon-btn" :title="log.paused ? 'Retomar' : 'Pausar'" :aria-label="log.paused ? 'Retomar' : 'Pausar'" @click="log.paused = !log.paused">
        <Icon :name="log.paused ? 'play' : 'stop'" :size="12" />
      </button>
      <button class="icon-btn" title="Copiar entradas visíveis" aria-label="Copiar log" @click="copyAll"><Icon name="copy" :size="13" /></button>
      <button class="icon-btn" title="Limpar log" aria-label="Limpar log" @click="log.clear()"><Icon name="trash" :size="13" /></button>
      <button class="icon-btn" title="Fechar (Ctrl+Shift+L)" aria-label="Fechar log" @click="emit('close')"><Icon name="x" :size="13" /></button>
    </header>
    <div ref="scrollEl" class="body mono selectable" @scroll.passive="onScroll">
      <div :style="{ height: virtualizer.getTotalSize() + 'px', position: 'relative' }">
        <div
          v-for="it in virtualizer.getVirtualItems()" :key="visible[it.index]!.id" class="line" :class="{ err: !!visible[it.index]!.error }"
          :style="{ transform: `translateY(${it.start}px)` }" :title="visible[it.index]!.error ?? visible[it.index]!.sql"
          @dblclick="toEditor(visible[it.index]!.sql, visible[it.index]!.connectionId)"
        >
          <span class="at">{{ time(visible[it.index]!.at) }}</span>
          <span class="cn ellipsis">{{ connName(visible[it.index]!.connectionId) }}</span>
          <span class="du">{{ formatDuration(visible[it.index]!.durationMs) }}</span>
          <span class="sql ellipsis">{{ visible[it.index]!.sql.replace(/\s+/g, ' ') }}<template v-if="visible[it.index]!.error"> — {{ visible[it.index]!.error }}</template></span>
        </div>
      </div>
      <div v-if="!visible.length" class="empty faint">Nenhuma instrução registrada.</div>
    </div>
  </section>
</template>

<style scoped>
.log {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  background: var(--bg);
  border-top: 1px solid var(--border);
}
header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 3px 6px 3px 10px;
  border-bottom: 1px solid var(--border);
  background: var(--bg-elev);
  font-size: 12px;
  flex: none;
}
header strong {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.input.compact {
  height: 24px;
  width: 200px;
}
.count {
  font-size: 11px;
}
.spacer {
  flex: 1;
}
.body {
  flex: 1;
  overflow: auto;
  font-size: 11.5px;
}
.line {
  position: absolute;
  left: 0;
  right: 0;
  top: 0;
  display: flex;
  gap: 10px;
  height: 20px;
  line-height: 20px;
  padding: 0 10px;
  white-space: nowrap;
}
.line:hover {
  background: var(--bg-hover);
}
.line.err {
  color: var(--danger);
}
.at {
  color: var(--text-faint);
  flex: none;
}
.cn {
  width: 120px;
  color: var(--text-muted);
  flex: none;
}
.du {
  width: 64px;
  text-align: right;
  color: var(--text-faint);
  flex: none;
}
.sql {
  min-width: 0;
}
.empty {
  padding: 10px;
}
</style>

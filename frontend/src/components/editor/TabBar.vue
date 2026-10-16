<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { isDirty, tabKindLabels, useTabsStore, type Tab } from '../../stores/tabs'
import { useConnectionsStore } from '../../stores/connections'
import { useUiStore } from '../../stores/ui'
import Icon, { type IconName } from '../common/Icon.vue'
import ContextMenu, { type MenuItem } from '../common/ContextMenu.vue'

const tabs = useTabsStore()
const conns = useConnectionsStore()
const ui = useUiStore()
const menu = ref<{ x: number; y: number; items: MenuItem[] } | null>(null)
const strip = ref<HTMLElement | null>(null)

const kindIcons: Record<string, IconName> = { host: 'server', database: 'database', table: 'table', view: 'view', routine: 'fn', trigger: 'zap', event: 'clock', users: 'user' }

function envOf(t: Tab) {
  return t.connectionId ? conns.byId.get(t.connectionId) : undefined
}

async function rename(t: Tab) {
  const r = await ui.prompt({ title: 'Renomear aba', label: 'Título', value: t.title, confirmLabel: 'Renomear' })
  if (r) tabs.rename(t.id, r.value)
}

function openMenu(e: MouseEvent, t: Tab) {
  e.preventDefault()
  menu.value = {
    x: e.clientX,
    y: e.clientY,
    items: [
      { label: 'Renomear', icon: 'edit', action: () => rename(t) },
      { label: 'Duplicar', icon: 'copy', action: () => tabs.duplicate(t.id), disabled: t.kind !== 'query' },
      { separator: true, label: '' },
      { label: 'Fechar', icon: 'x', action: () => tabs.close(t.id), shortcut: 'Ctrl+W' },
      { label: 'Fechar outras abas', action: () => tabs.closeOthers(t.id), disabled: tabs.tabs.length < 2 },
      { label: 'Fechar abas à direita', action: () => tabs.closeRight(t.id), disabled: tabs.tabs[tabs.tabs.length - 1]?.id === t.id },
      { label: 'Fechar todas', action: () => tabs.closeAll() },
    ],
  }
}

async function newTab() {
  tabs.open()
  await nextTick()
  strip.value?.scrollTo({ left: strip.value.scrollWidth, behavior: 'smooth' })
}

function onKey(e: KeyboardEvent, i: number) {
  if (e.key === 'ArrowRight' || e.key === 'ArrowLeft') {
    const next = tabs.tabs[(i + (e.key === 'ArrowRight' ? 1 : -1) + tabs.tabs.length) % tabs.tabs.length]
    if (next) {
      tabs.activate(next.id)
      void nextTick(() => strip.value?.querySelector<HTMLElement>(`[data-tab="${next.id}"]`)?.focus())
    }
  } else if (e.key === 'Delete') {
    const t = tabs.tabs[i]
    if (t) void tabs.close(t.id)
  } else if (e.key === 'F2') {
    const t = tabs.tabs[i]
    if (t) void rename(t)
  }
}
</script>

<template>
  <div class="tabbar">
    <div ref="strip" class="strip" role="tablist" aria-label="Abas SQL">
      <div
        v-for="(t, i) in tabs.tabs" :key="t.id" class="tab" role="tab" :data-tab="t.id" :tabindex="t.id === tabs.activeId ? 0 : -1"
        :aria-selected="t.id === tabs.activeId" :class="{ active: t.id === tabs.activeId, prod: envOf(t)?.environment === 'production' }"
        :title="`${t.kind !== 'query' ? tabKindLabels[t.kind] + ': ' : ''}${t.title}${t.kind !== 'query' && t.kind !== 'host' && t.kind !== 'database' ? ' (' + t.database + ')' : ''}${envOf(t) ? ' — ' + envOf(t)!.name : ' — sem conexão'}${t.filePath ? '\n' + t.filePath : ''}`"
        @mousedown.left="tabs.activate(t.id)" @mouseup.middle="tabs.close(t.id)" @dblclick="rename(t)" @contextmenu="openMenu($event, t)" @keydown="onKey($event, i)"
      >
        <span class="dot" :class="envOf(t) ? `env-${envOf(t)!.environment}` : 'none'" :style="envOf(t)?.color ? { background: envOf(t)!.color } : undefined" />
        <Icon v-if="t.kind !== 'query'" :name="kindIcons[t.kind] ?? 'table'" :size="12" class="kind" />
        <span class="title ellipsis">{{ t.title }}</span>
        <span v-if="t.status !== 'idle'" class="spinner" aria-label="executando" />
        <span v-else-if="t.inTransaction" class="tx" title="Transação aberta"><Icon name="lock" :size="11" /></span>
        <button class="close" :class="{ dirty: isDirty(t) }" :aria-label="`Fechar ${t.title}`" tabindex="-1" @mousedown.stop @click.stop="tabs.close(t.id)">
          <span class="dirty-dot" />
          <Icon name="x" :size="12" />
        </button>
      </div>
    </div>
    <button class="icon-btn add" title="Nova aba (Ctrl+T)" aria-label="Nova aba" @click="newTab"><Icon name="plus" /></button>
    <ContextMenu v-if="menu" :x="menu.x" :y="menu.y" :items="menu.items" @close="menu = null" />
  </div>
</template>

<style scoped>
.tabbar {
  display: flex;
  align-items: stretch;
  height: var(--tabbar-h);
  background: var(--bg-sunken);
  border-bottom: 1px solid var(--border);
  flex: none;
}
.strip {
  display: flex;
  min-width: 0;
  overflow-x: auto;
  scrollbar-width: thin;
}
.strip::-webkit-scrollbar {
  height: 3px;
}
.tab {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 110px;
  max-width: 220px;
  padding: 0 4px 0 10px;
  border-right: 1px solid var(--border);
  color: var(--text-muted);
  cursor: pointer;
  position: relative;
  flex: none;
}
.tab:hover {
  background: var(--bg-hover);
}
.tab.active {
  background: var(--bg);
  color: var(--text);
}
.tab.active::after {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  top: 0;
  height: 2px;
  background: var(--accent);
}
.tab.prod.active::after {
  background: var(--env-production);
}
.kind {
  color: var(--accent);
  margin-left: -2px;
}
.title {
  flex: 1;
  min-width: 0;
}
.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex: none;
}
.dot.none {
  border: 1px solid var(--text-faint);
}
.tx {
  color: var(--warning);
}
.close {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  padding: 0;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
}
.close:hover {
  background: var(--bg-active);
  color: var(--text);
}
.close .icon {
  opacity: 0;
}
.tab:hover .close .icon,
.tab.active .close .icon {
  opacity: 1;
}
.dirty-dot {
  position: absolute;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--text-muted);
  display: none;
}
.close.dirty .dirty-dot {
  display: block;
}
.close.dirty .icon {
  opacity: 0;
}
.tab:hover .close.dirty .dirty-dot {
  display: none;
}
.tab:hover .close.dirty .icon {
  opacity: 1;
}
.add {
  align-self: center;
  margin: 0 4px;
}
</style>

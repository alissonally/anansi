<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Icon, { type IconName } from './Icon.vue'

export interface MenuItem {
  label: string
  icon?: IconName
  action?: () => void
  danger?: boolean
  disabled?: boolean
  separator?: boolean
  shortcut?: string
  /** Submenu items (opened on hover, click or ArrowRight). */
  children?: MenuItem[]
  /** Muted helper text shown after the label. */
  hint?: string
}

const props = defineProps<{ items: MenuItem[]; x: number; y: number; nested?: boolean }>()
// close: this menu closes (a nested menu returns focus to its parent);
// done: an action ran, the whole chain closes.
const emit = defineEmits<{ close: []; done: [] }>()
const el = ref<HTMLElement | null>(null)
const pos = ref({ left: props.x, top: props.y })
const sub = ref<{ index: number; x: number; y: number } | null>(null)
let subTimer: number | undefined

async function place() {
  await nextTick()
  const r = el.value?.getBoundingClientRect()
  if (!r) return
  let left = props.x
  if (props.nested && left + r.width > window.innerWidth - 4) left = Math.max(4, props.x - r.width * 2 + 4)
  pos.value = {
    left: Math.max(4, Math.min(left, window.innerWidth - r.width - 4)),
    top: Math.max(4, Math.min(props.y, window.innerHeight - r.height - 4)),
  }
  el.value?.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus()
}

function finish() {
  emit('done')
  emit('close')
}

function openSub(i: number, target: HTMLElement, immediate: boolean) {
  window.clearTimeout(subTimer)
  const item = props.items[i]
  if (!item?.children?.length || item.disabled) {
    if (sub.value) subTimer = window.setTimeout(() => (sub.value = null), immediate ? 0 : 200)
    return
  }
  const r = target.getBoundingClientRect()
  const show = () => (sub.value = { index: i, x: r.right - 2, y: r.top - 4 })
  if (immediate) show()
  else subTimer = window.setTimeout(show, 120)
}

function closeSub() {
  const i = sub.value?.index
  sub.value = null
  if (i !== undefined) el.value?.querySelector<HTMLButtonElement>(`[data-index='${i}']`)?.focus()
}

function onDocDown(e: MouseEvent) {
  const t = e.target as HTMLElement
  if (!t.closest?.('.ctx-menu')) emit('close')
}

function onKey(e: KeyboardEvent) {
  // Only the innermost open menu handles keys.
  if (sub.value) return
  const buttons = Array.from(el.value?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)') ?? [])
  const cur = document.activeElement as HTMLButtonElement
  if (e.key === 'Escape' || (props.nested && e.key === 'ArrowLeft')) {
    e.preventDefault()
    e.stopPropagation()
    emit('close')
  } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault()
    const i = buttons.indexOf(cur)
    buttons[(i + (e.key === 'ArrowDown' ? 1 : -1) + buttons.length) % buttons.length]?.focus()
  } else if (e.key === 'ArrowRight' && buttons.includes(cur)) {
    e.preventDefault()
    openSub(Number(cur.dataset.index), cur, true)
  }
}

function run(item: MenuItem, i: number, ev: MouseEvent) {
  if (item.children?.length) {
    openSub(i, ev.currentTarget as HTMLElement, true)
    return
  }
  finish()
  item.action?.()
}

onMounted(() => {
  void place()
  if (!props.nested) {
    document.addEventListener('mousedown', onDocDown, true)
    window.addEventListener('blur', () => emit('close'), { once: true })
  }
  document.addEventListener('keydown', onKey, true)
})
onBeforeUnmount(() => {
  window.clearTimeout(subTimer)
  document.removeEventListener('mousedown', onDocDown, true)
  document.removeEventListener('keydown', onKey, true)
})
watch(() => [props.x, props.y], place)
</script>

<template>
  <Teleport to="body">
    <div ref="el" class="menu ctx-menu" role="menu" :style="{ left: pos.left + 'px', top: pos.top + 'px' }">
      <template v-for="(item, i) in items" :key="i">
        <div v-if="item.separator" class="sep" role="separator" />
        <button
          v-else class="item" :class="{ danger: item.danger, open: sub?.index === i }" role="menuitem" :data-index="i" :disabled="item.disabled"
          :aria-haspopup="item.children?.length ? 'menu' : undefined" @click="run(item, i, $event)" @mouseenter="openSub(i, $event.currentTarget as HTMLElement, false)"
        >
          <Icon v-if="item.icon" :name="item.icon" :size="14" />
          <span v-else class="icon-space" />
          <span class="label">{{ item.label }}<small v-if="item.hint" class="hint"> {{ item.hint }}</small></span>
          <span v-if="item.shortcut" class="shortcut">{{ item.shortcut }}</span>
          <Icon v-if="item.children?.length" name="chevronRight" :size="12" class="more" />
        </button>
      </template>
    </div>
    <ContextMenu v-if="sub && items[sub.index]?.children" nested :items="items[sub.index]!.children!" :x="sub.x" :y="sub.y" @close="closeSub" @done="finish" />
  </Teleport>
</template>

<style scoped>
.menu {
  position: fixed;
  z-index: 1000;
  min-width: 200px;
  max-width: 460px;
  max-height: calc(100vh - 8px);
  overflow-y: auto;
  padding: 4px;
  background: var(--bg-elev);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius);
  box-shadow: var(--shadow);
}
.item {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-height: 28px;
  padding: 0 8px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  text-align: left;
  cursor: pointer;
}
.item:hover:not(:disabled),
.item:focus-visible,
.item.open {
  background: var(--bg-hover);
  outline: none;
}
.item:disabled {
  opacity: 0.45;
  cursor: default;
}
.item.danger {
  color: var(--danger);
}
.icon-space {
  width: 14px;
  flex: none;
}
.label {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.hint {
  color: var(--text-faint);
  font-family: var(--font-mono);
  font-size: 11px;
}
.shortcut {
  color: var(--text-faint);
  font-size: 11px;
}
.more {
  color: var(--text-faint);
}
.sep {
  height: 1px;
  margin: 4px 2px;
  background: var(--border);
}
</style>

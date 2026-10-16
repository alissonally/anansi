<script setup lang="ts">
import { computed } from 'vue'

const paths = {
  plus: 'M12 5v14M5 12h14',
  x: 'M18 6 6 18M6 6l12 12',
  play: 'M7 4.5v15l12-7.5z',
  playAll: 'M3 5v14l8-7zM12 5v14l8-7z',
  stop: 'M6 6h12v12H6z',
  chevronRight: 'm9 18 6-6-6-6',
  chevronDown: 'm6 9 6 6 6-6',
  database: 'M3 5c0-1.7 4-3 9-3s9 1.3 9 3-4 3-9 3-9-1.3-9-3zM3 5v14c0 1.7 4 3 9 3s9-1.3 9-3V5M3 12c0 1.7 4 3 9 3s9-1.3 9-3',
  table: 'M3 4h18v16H3zM3 10h18M3 15h18M9 10v10',
  view: 'M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12zM12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6z',
  column: 'M4 6h16M4 12h10M4 18h7',
  key: 'M15 4a5 5 0 1 1-4.6 7L4 17.4V20h3v-2h2v-2h2l1.3-1.3A5 5 0 0 1 15 4zM16.5 7.5h.01',
  index: 'M3 6h18M7 12h14M11 18h10',
  refresh: 'M21 12a9 9 0 1 1-2.6-6.4L21 8M21 3v5h-5',
  settings: 'M4 21v-7M4 10V3M12 21v-9M12 8V3M20 21v-5M20 12V3M1 14h6M9 8h6M17 16h6',
  history: 'M12 7v5l3 3M3 12a9 9 0 1 0 2.6-6.4L3 8M3 3v5h5',
  save: 'M5 3h11l5 5v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2zM7 3v5h8M7 21v-7h10v7',
  folder: 'M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z',
  format: 'M4 6h16M8 10h12M4 14h16M8 18h12',
  copy: 'M9 9h11v11H9zM5 15H4V4h11v1',
  trash: 'M3 6h18M8 6V4h8v2M6 6l1 14h10l1-14',
  edit: 'M4 20h4L19 9l-4-4L4 16zM13.5 6.5l4 4',
  plug: 'M9 2v6M15 2v6M6 8h12v4a6 6 0 0 1-12 0zM12 18v4',
  unplug: 'M9 2v4M15 2v6M8 8h10v4a6 6 0 0 1-1.2 3.6M12 18v4M6 8v4a6 6 0 0 0 9 5.2M3 3l18 18',
  more: 'M5 12h.01M12 12h.01M19 12h.01',
  search: 'M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14zM21 21l-5-5',
  alert: 'M12 3 2 21h20zM12 10v5M12 18h.01',
  check: 'm5 12 5 5L20 7',
  sun: 'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8zM12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4',
  moon: 'M20 14.5A8 8 0 0 1 9.5 4 8 8 0 1 0 20 14.5z',
  bookmark: 'M6 3h12v18l-6-4-6 4z',
  download: 'M12 3v12M7 10l5 5 5-5M4 21h16',
  lock: 'M6 11h12v10H6zM8 11V7a4 4 0 0 1 8 0v4',
  fn: 'M9 20c2 0 2-2 3-8s1-8 3-8M7 10h8',
  shield: 'M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6z',
  file: 'M14 3H6v18h12V7zM14 3v4h4',
  info: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 11v5M12 8h.01',
  terminal: 'M4 5h16v14H4zM8 10l3 2-3 2M13 15h3',
  commit: 'm5 12 5 5L20 7',
  rollback: 'M9 14 4 9l5-5M4 9h11a5 5 0 0 1 0 10h-3',
  server: 'M3 4h18v6H3zM3 14h18v6H3zM7 7h.01M7 17h.01',
  zap: 'M13 2 4 14h7l-1 8 9-12h-7z',
  clock: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 7v5l3 2',
  user: 'M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM4 21a8 8 0 0 1 16 0',
  filter: 'M3 5h18l-7 8v6l-4 2v-8z',
  minus: 'M5 12h14',
  upload: 'M12 21V9M7 14l5-5 5 5M4 3h16',
  wrench: 'M14.7 6.3a4 4 0 0 0-5.4 5.4L3 18l3 3 6.3-6.3a4 4 0 0 0 5.4-5.4l-2.5 2.5-2.4-.6-.6-2.4z',
  columns: 'M3 4h18v16H3zM9 4v16M15 4v16',
  arrowUp: 'M12 19V5M5 12l7-7 7 7',
  arrowDown: 'M12 5v14M19 12l-7 7-7-7',
  star: 'M12 3l2.8 5.7 6.2.9-4.5 4.4 1 6.2L12 17.3 6.5 20.2l1-6.2L3 9.6l6.2-.9z',
  link: 'M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1',
  undo: 'M9 14 4 9l5-5M4 9h11a5 5 0 0 1 0 10h-3',
} as const

export type IconName = keyof typeof paths

const props = withDefaults(defineProps<{ name: IconName; size?: number; strokeWidth?: number }>(), { size: 16, strokeWidth: 1.8 })
const d = computed(() => paths[props.name])
const filled = computed(() => props.name === 'play' || props.name === 'playAll' || props.name === 'stop')
</script>

<template>
  <svg
    :width="size" :height="size" viewBox="0 0 24 24" :fill="filled ? 'currentColor' : 'none'" stroke="currentColor"
    :stroke-width="name === 'more' ? 3.2 : strokeWidth" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"
    focusable="false" class="icon"
  >
    <path :d="d" />
  </svg>
</template>

<style scoped>
.icon {
  flex: none;
  display: block;
}
</style>

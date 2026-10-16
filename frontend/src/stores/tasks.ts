import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { backend } from '../api/backend'
import type { Task } from '../api/types'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { useUiStore } from './ui'

/** Background tasks (exports, dumps, imports, SQL files, tools). */
export const useTasksStore = defineStore('tasks', () => {
  const tasks = ref<Task[]>([])
  const panelOpen = ref(false)
  let started = false
  // The panel opened by track() closes by itself when everything finished well.
  let autoOpened = false
  const listeners = new Map<string, (t: Task) => void>()

  function upsert(t: Task) {
    const i = tasks.value.findIndex((x) => x.id === t.id)
    const prev = i >= 0 ? tasks.value[i] : undefined
    if (i >= 0) tasks.value.splice(i, 1, t)
    else tasks.value.unshift(t)
    if (prev?.status === 'running' && t.status !== 'running') {
      const ui = useUiStore()
      if (t.status === 'done') ui.toast(t.warnings ? 'warning' : 'success', `${t.title}: ${t.result || 'concluído'}${t.warnings ? ` (${t.warnings} avisos)` : ''}`, 6000)
      else if (t.status === 'error') ui.toast('error', `${t.title}: ${t.error}`, 12000)
      else ui.toast('info', `${t.title}: cancelado`)
      listeners.get(t.id)?.(t)
      listeners.delete(t.id)
      if (autoOpened && t.status === 'done' && !t.warnings && !tasks.value.some((x) => x.status === 'running')) {
        panelOpen.value = false
        autoOpened = false
      }
    }
  }

  async function start() {
    if (started) return
    started = true
    try {
      for (const t of (await backend.listTasks()).reverse()) upsert(t)
    } catch {
      // backend unavailable
    }
    try {
      EventsOn('task:update', (t: Task) => upsert(t))
    } catch {
      // runtime unavailable
    }
  }

  /** Registers a task started by a call; opens the panel. */
  function track(id: string, onDone?: (t: Task) => void) {
    if (!id) return
    if (!panelOpen.value) autoOpened = true
    panelOpen.value = true
    if (onDone) listeners.set(id, onDone)
  }

  async function dismiss(id: string) {
    await backend.dismissTask(id).catch(() => undefined)
    tasks.value = tasks.value.filter((t) => t.id !== id)
  }

  async function clearFinished() {
    for (const t of tasks.value.filter((x) => x.status !== 'running')) await dismiss(t.id)
  }

  const running = computed(() => tasks.value.filter((t) => t.status === 'running').length)
  return { tasks, panelOpen, running, start, track, dismiss, clearFinished, cancel: (id: string) => backend.cancelTask(id) }
})

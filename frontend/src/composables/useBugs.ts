import { ref } from 'vue'
import { call, onEvent } from '../lib/wails'
import type { Bug, KindChange } from '../lib/types'

const bugs = ref<Bug[]>([])
const loading = ref(false)
const error = ref('')
let started = false

function upsert(b: Bug) {
  if (!b?.id) return
  const i = bugs.value.findIndex((x) => x.id === b.id)
  if (i >= 0) bugs.value.splice(i, 1, b)
  else bugs.value.unshift(b)
}

export function useBugs() {
  if (!started) {
    started = true
    onEvent<Bug>('bug:changed', upsert)
    onEvent<{ id: string }>('bug:deleted', ({ id }) => {
      bugs.value = bugs.value.filter((b) => b.id !== id)
    })
  }

  async function load() {
    loading.value = true
    error.value = ''
    try {
      const list = await call<Bug[]>('main.BugService.List')
      bugs.value = list ?? []
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  async function save(b: Bug): Promise<Bug> {
    return call<Bug>('main.BugService.Save', b)
  }

  async function updateStatus(id: string, status: string) {
    const bug = bugs.value.find((b) => b.id === id)
    const previous = bug?.status
    if (bug) bug.status = status
    try {
      await call('main.BugService.UpdateStatus', id, status)
    } catch (e) {
      if (bug && previous) bug.status = previous
      throw e
    }
  }

  async function remove(id: string) {
    await call('main.BugService.Delete', id)
  }

  async function reanalyze(id: string) {
    await call('main.BugService.Reanalyze', id)
  }

  async function setKind(id: string, kind: 'bug' | 'todo', lock = true) {
    await call('main.BugService.SetKind', id, kind, lock)
  }

  async function undoKindChange(change: KindChange) {
    await call('main.BugService.UndoKindChange', {
      id: change.id,
      from: change.from,
      to: change.to,
      fromStatus: change.fromStatus,
      fromDueAt: change.fromDueAt,
      toDueAt: change.toDueAt,
    })
  }

  async function fixPrompt(id: string): Promise<string> {
    return call<string>('main.BugService.GenerateFixPrompt', id)
  }

  return {
    bugs, loading, error, load, save, updateStatus, remove, reanalyze, setKind, undoKindChange, fixPrompt,
  }
}

import { computed, ref } from 'vue'
import { call, onEvent } from '../lib/wails'
import type { Bug, CaptureItem, Snapshot } from '../lib/types'

const snapshot = ref<Snapshot>({ selectedId: '', items: [] })
const selectedId = ref('')
const readyTick = ref(0)
let started = false

function apply(snap: Snapshot | undefined) {
  if (!snap) return
  snapshot.value = snap
  if (!snap.items.some((i) => i.id === selectedId.value)) {
    selectedId.value = snap.selectedId || snap.items[0]?.id || ''
  }
}

export function useCapture() {
  if (!started) {
    started = true
    onEvent<Snapshot>('capture:ready', (s) => {
      apply(s)
      selectedId.value = s?.selectedId || s?.items[0]?.id || ''
      readyTick.value++
    })
    onEvent<Snapshot>('capture:update', apply)
  }

  const items = computed(() => snapshot.value.items ?? [])
  const selected = computed(() => items.value.find((i) => i.id === selectedId.value) ?? null)

  function select(id: string) {
    selectedId.value = id
  }

  // 捕获窗打开时兜底拉一次（事件可能在窗口未挂载时发出）
  async function sync() {
    try {
      apply(await call<Snapshot>('main.CaptureService.Snapshot'))
      if (!selected.value && items.value.length) selectedId.value = items.value[0].id
    } catch {
      /* 忽略 */
    }
  }

  function itemToAttachment(item: CaptureItem) {
    return item.type === 'image'
      ? { type: 'image' as const, path: item.path }
      : { type: 'text' as const, path: '', text: item.text }
  }

  // 组装并保存 bug；成功后消费该剪贴板条目并隐藏捕获窗
  async function save(opts: { title: string; description: string; descriptionText: string }) {
    const item = selected.value
    const bug: Bug = {
      id: '',
      title: opts.title,
      description: opts.description,
      descriptionText: opts.descriptionText,
      status: 'todo',
      category: 'uncategorized',
      severity: 'P2',
      tags: [],
      attachments: item ? [itemToAttachment(item)] : [],
      sourceApp: item?.sourceApp || '',
      ai: null,
      aiStatus: 'none',
      createdAt: 0,
      updatedAt: 0,
      resolvedAt: null,
    }
    const saved = await call<Bug>('main.BugService.Save', bug)
    if (item) {
      await call('main.CaptureService.Consume', item.id)
    }
    await call('main.CaptureService.Cancel')
    return saved
  }

  async function cancel() {
    await call('main.CaptureService.Cancel')
  }

  return { items, selected, selectedId, readyTick, select, sync, save, cancel }
}

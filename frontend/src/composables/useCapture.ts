import { computed, ref } from 'vue'
import { call, onEvent } from '../lib/wails'
import type { Bug, CaptureItem, Kind, KindSource, Snapshot } from '../lib/types'

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

  // 组装并保存事项；成功后消费该剪贴板条目并隐藏捕获窗
  async function save(opts: {
    title: string
    description: string
    descriptionText: string
    kind: Kind
    kindSource: KindSource
    dueAt: number
    dueAllDay: boolean
  }) {
    const item = selected.value
    const bug: Bug = {
      id: '',
      kind: opts.kind,
      kindSource: opts.kindSource,
      title: opts.title,
      description: opts.description,
      descriptionText: opts.descriptionText,
      status: 'todo',
      category: 'uncategorized',
      severity: 'P2',
      dueAt: opts.kind === 'todo' ? opts.dueAt : 0,
      dueAllDay: opts.kind === 'todo' ? opts.dueAllDay : false,
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
    // 记录已经落库，后续清理失败不能误报“保存失败”或让窗口卡住。
    if (item) {
      try {
        await call('main.CaptureService.Consume', item.id)
      } catch {
        /* 下一次触发仍可看到该条目，但不影响本次保存 */
      }
    }
    try {
      await call('main.CaptureService.Cancel')
    } catch {
      /* 窗口可能已经被系统关闭 */
    }
    return saved
  }

  async function cancel() {
    await call('main.CaptureService.Cancel')
  }

  return { items, selected, selectedId, readyTick, select, sync, save, cancel }
}

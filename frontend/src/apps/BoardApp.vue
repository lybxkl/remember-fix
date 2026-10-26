<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus, Search, Settings2, WandSparkles, Wrench, X } from 'lucide-vue-next'
import { call, onEvent } from '../lib/wails'
import { useBugs } from '../composables/useBugs'
import StatusColumn from '../components/StatusColumn.vue'
import BugDrawer from '../components/BugDrawer.vue'
import SettingsDialog from '../components/SettingsDialog.vue'
import EmptyArt from '../components/EmptyArt.vue'
import {
  CATEGORIES, SEVERITIES, boardColumns, dueBucket, dueMeta, isDoneStatus, isOpen, kindLabel,
} from '../lib/constants'
import type { BoardView } from '../lib/constants'
import type { Bug, KindChange } from '../lib/types'
import { defaultHotkey } from '../lib/platform'

const { bugs, error, load, updateStatus, undoKindChange } = useBugs()

const views: { key: BoardView; label: string }[] = [
  { key: 'all', label: '全部' },
  { key: 'bug', label: 'Bug' },
  { key: 'todo', label: '待办' },
]

const keyword = ref('')
const filterCategory = ref('')
const filterSeverity = ref('')
const onlyPending = ref(false)
const dueFilter = ref<'' | 'today' | 'overdue'>('')
const viewMode = ref<BoardView>('all')
const drawerId = ref('')
const settingsOpen = ref(false)
const hotkey = ref(defaultHotkey)
const loaded = ref(false)
const searchRef = ref<HTMLInputElement>()
const clock = ref(Date.now())

const kindQueue = ref<KindChange[]>([])
const kindToast = ref<KindChange | null>(null)
let kindToastTimer: number | undefined
let kindPumpTimer: number | undefined
let clockTimer: number | undefined
let offSettings: (() => void) | undefined
let offPending: (() => void) | undefined
let offKind: (() => void) | undefined
let offDeleted: (() => void) | undefined

onMounted(async () => {
  const saved = localStorage.getItem('rf.board.view')
  if (saved === 'bug' || saved === 'todo' || saved === 'all') viewMode.value = saved
  offSettings = onEvent('ui:open-settings', () => {
    settingsOpen.value = true
  })
  offPending = onEvent<{ due?: string } | null>('ui:open-pending', (data) => {
    clearFilters()
    viewMode.value = 'all'
    localStorage.setItem('rf.board.view', 'all')
    onlyPending.value = true
    if (data?.due === 'today' || data?.due === 'overdue') dueFilter.value = data.due
  })
  offKind = onEvent<KindChange>('bug:kind-changed', onKindChanged)
  offDeleted = onEvent<{ id: string }>('bug:deleted', onItemDeleted)
  window.addEventListener('keydown', onKeydown)
  document.addEventListener('visibilitychange', flushKindChanges)
  clockTimer = window.setInterval(() => {
    clock.value = Date.now()
  }, 60_000)
  await initialLoad()
})
onBeforeUnmount(() => {
  offSettings?.()
  offPending?.()
  offKind?.()
  offDeleted?.()
  window.removeEventListener('keydown', onKeydown)
  document.removeEventListener('visibilitychange', flushKindChanges)
  if (kindToastTimer) window.clearTimeout(kindToastTimer)
  if (kindPumpTimer) window.clearTimeout(kindPumpTimer)
  if (clockTimer) window.clearInterval(clockTimer)
})

async function initialLoad() {
  loaded.value = false
  await load()
  loaded.value = true
  await loadHotkey()
}

async function loadHotkey() {
  try {
    const cfg = await call<Record<string, string>>('main.ConfigService.GetAll')
    if (cfg?.hotkey) hotkey.value = cfg.hotkey
  } catch {
    /* 保持默认显示 */
  }
}

function setView(view: BoardView) {
  viewMode.value = view
  localStorage.setItem('rf.board.view', view)
  if (view === 'todo') filterCategory.value = ''
  if (view === 'bug') dueFilter.value = ''
}

const counts = computed(() => ({
  all: bugs.value.length,
  bug: bugs.value.filter((b) => b.kind === 'bug').length,
  todo: bugs.value.filter((b) => b.kind === 'todo').length,
}))

const openItems = computed(() => bugs.value.filter((b) => isOpen(b.status)))
const dueCounts = computed(() => ({
  today: openItems.value.filter((b) => dueBucket(b, clock.value) === 'today').length,
  overdue: openItems.value.filter((b) => dueBucket(b, clock.value) === 'overdue').length,
}))

const filtered = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  const now = clock.value
  return bugs.value.filter((b) => {
    if (viewMode.value === 'bug' && b.kind !== 'bug') return false
    if (viewMode.value === 'todo' && b.kind !== 'todo') return false
    if (onlyPending.value && !isOpen(b.status)) return false
    if (dueFilter.value && dueBucket(b, now) !== dueFilter.value) return false
    if (filterCategory.value && (b.kind !== 'bug' || b.category !== filterCategory.value)) return false
    if (filterSeverity.value && b.severity !== filterSeverity.value) return false
    if (!kw) return true
    const due = b.kind === 'todo' ? (dueMeta(b.dueAt, b.dueAllDay, false, now)?.text ?? '') : ''
    const hay = `${b.title} ${b.descriptionText} ${b.tags.join(' ')} ${b.sourceApp} ${kindLabel(b.kind)} ${due}`.toLowerCase()
    return hay.includes(kw)
  })
})

const columns = computed(() => boardColumns(viewMode.value))

function sortItems(a: Bug, b: Bug): number {
  const aDue = isOpen(a.status) && a.kind === 'todo' && a.dueAt > 0
  const bDue = isOpen(b.status) && b.kind === 'todo' && b.dueAt > 0
  if (aDue && bDue && a.dueAt !== b.dueAt) return a.dueAt - b.dueAt
  if (aDue !== bDue) return aDue ? -1 : 1
  return b.createdAt - a.createdAt
}

const columnItems = computed(() => {
  const cols = columns.value
  return cols.map((col, index) => {
    const items = filtered.value.filter((b) => col.statuses.includes(b.status))
    if (index === 0) {
      // 旧数据或未知状态兜底到第一列，避免记录凭空消失。
      items.push(...filtered.value.filter((b) => !cols.some((c) => c.statuses.includes(b.status))))
    }
    return { ...col, bugs: items.sort(sortItems) }
  })
})

const activeFilters = computed(
  () => onlyPending.value || !!dueFilter.value || !!keyword.value.trim() || !!filterCategory.value || !!filterSeverity.value,
)

const stats = computed(() => {
  const now = new Date(clock.value)
  const mondayOffset = (now.getDay() + 6) % 7
  const weekStart = Math.floor(
    new Date(now.getFullYear(), now.getMonth(), now.getDate() - mondayOffset).getTime() / 1000,
  )
  const open = openItems.value.length
  const closedWeek = bugs.value.filter(
    (b) => isDoneStatus(b.status) && !!b.resolvedAt && b.resolvedAt >= weekStart,
  ).length
  const aiPending = bugs.value.filter((b) => b.aiStatus === 'pending').length
  return { open, closedWeek, overdue: dueCounts.value.overdue, total: bugs.value.length, aiPending }
})

function clearFilters() {
  keyword.value = ''
  filterCategory.value = ''
  filterSeverity.value = ''
  onlyPending.value = false
  dueFilter.value = ''
}

function toggleDue(filter: 'today' | 'overdue') {
  dueFilter.value = dueFilter.value === filter ? '' : filter
}

function onKeydown(e: KeyboardEvent) {
  const target = e.target as HTMLElement | null
  const typing =
    !!target &&
    (target.tagName === 'INPUT' ||
      target.tagName === 'TEXTAREA' ||
      target.isContentEditable)
  if (e.key === '/' && !typing) {
    e.preventDefault()
    searchRef.value?.focus()
    return
  }
  if (e.key === 'Escape' && typing && target === searchRef.value) {
    if (keyword.value) keyword.value = ''
    else searchRef.value?.blur()
  }
}

async function onDrop(bugId: string, status: string) {
  const bug = bugs.value.find((b) => b.id === bugId)
  if (!bug || bug.status === status) return
  // 「全部」视图的已完成列同时包含已验证，原地放下不应把已验证降级。
  if (status === 'fixed' && bug.status === 'verified') return
  try {
    await updateStatus(bugId, status)
  } catch (e) {
    ElMessage.error('移动失败：' + (e instanceof Error ? e.message : String(e)))
  }
}

async function onComplete(bug: Bug) {
  try {
    await updateStatus(bug.id, 'fixed')
    ElMessage.success('已完成')
  } catch (e) {
    ElMessage.error('标记完成失败：' + (e instanceof Error ? e.message : String(e)))
  }
}

function onKindChanged(change: KindChange | null) {
  if (!change?.id || !change.to) return
  kindQueue.value.push(change)
  pumpKindToast()
}

function onItemDeleted(payload: { id?: string } | null) {
  if (!payload?.id) return
  kindQueue.value = kindQueue.value.filter((c) => c.id !== payload.id)
  if (kindToast.value?.id === payload.id) dismissKindToast()
}

function pumpKindToast() {
  if (kindToast.value || document.hidden) return
  const next = kindQueue.value.shift()
  if (!next) return
  kindToast.value = next
  if (kindToastTimer) window.clearTimeout(kindToastTimer)
  kindToastTimer = window.setTimeout(dismissKindToast, 9000)
}

function flushKindChanges() {
  if (!document.hidden) pumpKindToast()
}

function dismissKindToast() {
  if (kindToastTimer) {
    window.clearTimeout(kindToastTimer)
    kindToastTimer = undefined
  }
  if (kindPumpTimer) {
    window.clearTimeout(kindPumpTimer)
    kindPumpTimer = undefined
  }
  kindToast.value = null
  if (!document.hidden) {
    kindPumpTimer = window.setTimeout(() => {
      kindPumpTimer = undefined
      pumpKindToast()
    }, 200)
  }
}

async function undoKind() {
  const change = kindToast.value
  if (!change) return
  const current = bugs.value.find((b) => b.id === change.id)
  if (!current || current.kind !== change.to) {
    dismissKindToast()
    return
  }
  try {
    await undoKindChange(change)
    ElMessage.success(`已改回${kindLabel(change.from)}`)
  } catch (e) {
    ElMessage.error('撤销失败：' + (e instanceof Error ? e.message : String(e)))
  }
  dismissKindToast()
}

function openDrawer(id: string) {
  drawerId.value = id
}

async function newBug() {
  try {
    await call('main.CaptureService.Trigger')
  } catch (e) {
    ElMessage.error('打开捕获窗失败：' + (e instanceof Error ? e.message : String(e)))
  }
}

function onSettingsClose() {
  settingsOpen.value = false
  loadHotkey()
}
</script>

<template>
  <div class="board">
    <header class="topbar">
      <div class="brand">
        <span class="brand-mark"><Wrench :size="17" :stroke-width="2.2" /></span>
        <div class="brand-copy">
          <span class="brand-name">记得修</span>
          <span class="brand-sub">REMEMBER / EVERYTHING</span>
        </div>
      </div>

      <div class="stats">
        <div class="stat">
          <span class="stat-num">{{ stats.open }}</span>
          <span class="stat-cap">未完成</span>
        </div>
        <span class="stat-sep" />
        <div class="stat">
          <span class="stat-num">{{ stats.closedWeek }}</span>
          <span class="stat-cap">本周完成</span>
        </div>
        <span class="stat-sep" />
        <div class="stat" :class="{ alert: stats.overdue > 0 }">
          <span class="stat-num">{{ stats.overdue }}</span>
          <span class="stat-cap">逾期</span>
        </div>
        <span class="stat-sep" />
        <div class="stat">
          <span class="stat-num">{{ stats.total }}</span>
          <span class="stat-cap">总记录</span>
        </div>
        <span v-if="stats.aiPending" class="ai-chip">
          <span class="pulse-dot" />AI 分析中 {{ stats.aiPending }}
        </span>
      </div>

      <div class="top-actions">
        <button class="record-btn" @click="newBug">
          <Plus :size="15" :stroke-width="2.4" />
          记一条
        </button>
        <button class="icon-btn" title="设置" @click="settingsOpen = true">
          <Settings2 :size="16" />
        </button>
      </div>
    </header>

    <div class="filterbar">
      <div class="view-tabs" role="tablist" aria-label="事项视图">
        <button
          v-for="v in views"
          :key="v.key"
          class="view-tab"
          :class="{ active: viewMode === v.key }"
          role="tab"
          :aria-selected="viewMode === v.key"
          @click="setView(v.key)"
        >
          {{ v.label }}
          <span v-if="counts[v.key]" class="view-count">{{ counts[v.key] }}</span>
        </button>
      </div>

      <div class="search-box" :class="{ filled: !!keyword }">
        <Search :size="14" class="search-icon" />
        <input
          ref="searchRef"
          v-model="keyword"
          placeholder="搜索标题、描述、标签…"
          spellcheck="false"
        />
        <kbd v-if="!keyword">/</kbd>
        <button v-else class="clear-input" title="清空" @click="keyword = ''"><X :size="12" /></button>
      </div>

      <el-select
        v-if="viewMode !== 'todo'"
        v-model="filterCategory"
        placeholder="全部分类"
        clearable
        style="width: 112px"
      >
        <el-option v-for="c in CATEGORIES" :key="c.key" :value="c.key" :label="c.label" />
      </el-select>
      <el-select v-model="filterSeverity" placeholder="全部优先级" clearable style="width: 124px">
        <el-option v-for="s in SEVERITIES" :key="s" :value="s" :label="s" />
      </el-select>

      <div v-if="viewMode !== 'bug'" class="due-filters">
        <button
          class="due-filter today"
          :class="{ active: dueFilter === 'today', empty: !dueCounts.today }"
          @click="toggleDue('today')"
        >
          今天<span v-if="dueCounts.today">{{ dueCounts.today }}</span>
        </button>
        <button
          class="due-filter overdue"
          :class="{ active: dueFilter === 'overdue', empty: !dueCounts.overdue }"
          @click="toggleDue('overdue')"
        >
          逾期<span v-if="dueCounts.overdue">{{ dueCounts.overdue }}</span>
        </button>
      </div>

      <button v-if="onlyPending" class="pending-filter" title="移除未完成筛选" @click="onlyPending = false">
        只看未完成
        <X :size="11" />
      </button>
      <button v-if="activeFilters" class="clear-filters" @click="clearFilters">清除筛选</button>
      <span class="filter-spacer" />
      <span v-if="activeFilters" class="result-count">{{ filtered.length }} 条匹配</span>
      <span class="hotkey-hint">随时按 <kbd>{{ hotkey }}</kbd> 记一条</span>
    </div>

    <main class="board-main">
      <div v-if="!loaded" class="board-state">正在翻开账本…</div>
      <div v-else-if="error" class="board-state error-state">
        <span>加载失败：{{ error }}</span>
        <button @click="initialLoad">重试</button>
      </div>
      <EmptyArt v-else-if="!bugs.length" :hotkey="hotkey" @record="newBug" />
      <div v-else class="flow" :class="`view-${viewMode}`">
        <StatusColumn
          v-for="col in columnItems"
          :key="col.key"
          :status="col.dropStatus"
          :label="col.label"
          :code="col.code"
          :color="col.color"
          :bugs="col.bugs"
          :show-kind="viewMode === 'all'"
          :now="clock"
          @open="openDrawer"
          @drop="onDrop"
          @complete="onComplete"
        />
      </div>
    </main>

    <BugDrawer :bug-id="drawerId" @close="drawerId = ''" />
    <SettingsDialog :open="settingsOpen" @close="onSettingsClose" />

    <transition name="kind-toast">
      <div v-if="kindToast" class="kind-toast" role="status" aria-live="polite">
        <span class="kind-toast-mark"><WandSparkles :size="14" /></span>
        <div class="kind-toast-copy">
          <span class="kind-toast-title">
            AI 判断更像「{{ kindLabel(kindToast.to) }}」，已自动切换
          </span>
          <span class="kind-toast-sub" :title="kindToast.title">
            {{ kindToast.title || '未命名事项' }} · 原为{{ kindLabel(kindToast.from) }} · 置信度 {{ Math.round(kindToast.confidence * 100) }}%
          </span>
        </div>
        <button class="kind-toast-undo" @click="undoKind">撤销</button>
        <button class="kind-toast-close" title="关闭" @click="dismissKindToast"><X :size="13" /></button>
      </div>
    </transition>
  </div>
</template>

<style scoped>
.board {
  height: 100vh;
  display: flex;
  flex-direction: column;
  background: var(--rf-bg);
}
.topbar {
  height: 62px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 18px;
  padding: 0 18px;
  border-bottom: 1px solid var(--rf-border);
  background: linear-gradient(180deg, #fdfbf7, var(--rf-bg));
}
.brand { display: flex; align-items: center; gap: 10px; flex-shrink: 0; }
.brand-mark {
  width: 31px;
  height: 31px;
  border-radius: 9px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--rf-accent);
  background: var(--rf-accent-soft);
  border: 1px solid rgba(168, 99, 19, 0.24);
}
.brand-copy { display: flex; flex-direction: column; line-height: 1.12; }
.brand-name { font-size: 16px; font-weight: 700; letter-spacing: -0.01em; }
.brand-sub {
  font-family: var(--rf-mono);
  font-size: 8px;
  letter-spacing: 0.14em;
  color: var(--rf-text-faint);
  margin-top: 2px;
}
.stats { display: flex; align-items: center; gap: 14px; margin-left: 4px; }
.stat { display: flex; align-items: baseline; gap: 5px; }
.stat-num { font-family: var(--rf-mono); font-size: 15px; font-weight: 600; color: var(--rf-text); }
.stat-cap { font-size: 10px; color: var(--rf-text-faint); letter-spacing: .02em; }
.stat.alert .stat-num { color: var(--rf-danger); }
.stat-sep { width: 1px; height: 14px; background: var(--rf-border); }
.ai-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin-left: 2px;
  font-size: 10px;
  color: var(--rf-accent);
  border: 1px solid rgba(168, 99, 19, 0.2);
  background: var(--rf-accent-soft);
  border-radius: 999px;
  padding: 2px 8px;
}
.pulse-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--rf-accent);
  animation: pulse 1.3s ease-in-out infinite;
}
@keyframes pulse { 50% { opacity: .25; } }
.top-actions { margin-left: auto; display: flex; align-items: center; gap: 8px; }
.record-btn {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  border: 1px solid rgba(168, 99, 19, 0.48);
  color: var(--rf-accent);
  background: transparent;
  border-radius: 8px;
  font-size: 12.5px;
  font-weight: 600;
  padding: 7px 13px 7px 11px;
  cursor: pointer;
  transition: background .15s ease, border-color .15s ease;
}
.record-btn:hover { background: var(--rf-accent-soft); border-color: var(--rf-accent); }
.icon-btn {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  border: 1px solid transparent;
  background: transparent;
  color: var(--rf-text-dim);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
}
.icon-btn:hover { color: var(--rf-text); border-color: var(--rf-border); background: var(--rf-card); }

.filterbar {
  height: 46px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 18px;
  border-bottom: 1px solid var(--rf-border);
  background: var(--rf-bg);
}
.view-tabs {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  padding: 2px;
  border: 1px solid var(--rf-border);
  border-radius: 8px;
  background: var(--rf-panel);
  flex-shrink: 0;
}
.view-tab {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: none;
  background: transparent;
  color: var(--rf-text-faint);
  font-size: 11.5px;
  font-weight: 600;
  border-radius: 6px;
  padding: 4px 8px;
  cursor: pointer;
  transition: color .14s ease, background .14s ease;
}
.view-tab:hover { color: var(--rf-text-dim); }
.view-tab.active { color: var(--rf-text); background: var(--rf-card); box-shadow: 0 1px 3px rgba(76, 62, 43, .08); }
.view-count {
  font-family: var(--rf-mono);
  font-size: 9px;
  color: var(--rf-text-faint);
  background: var(--rf-bg-soft);
  border-radius: 999px;
  padding: 0 4px;
  line-height: 14px;
  min-width: 15px;
  text-align: center;
}
.view-tab.active .view-count { color: var(--rf-accent); background: var(--rf-accent-soft); }
.search-box {
  display: flex;
  align-items: center;
  gap: 7px;
  width: 232px;
  height: 30px;
  padding: 0 9px;
  border: 1px solid var(--rf-border);
  border-radius: 7px;
  background: var(--rf-panel);
  transition: border-color .14s ease, background .14s ease;
}
.search-box:focus-within { border-color: var(--rf-accent); background: var(--rf-well); }
.search-icon { color: var(--rf-text-faint); flex-shrink: 0; }
.search-box input {
  flex: 1;
  min-width: 0;
  border: none;
  outline: none;
  background: transparent;
  color: var(--rf-text);
  font-size: 12px;
}
.search-box input::placeholder { color: var(--rf-text-faint); }
.search-box kbd { flex-shrink: 0; }
.clear-input { border: none; background: transparent; color: var(--rf-text-faint); cursor: pointer; display: flex; padding: 2px; }
.clear-input:hover { color: var(--rf-text); }
.due-filters { display: inline-flex; gap: 4px; flex-shrink: 0; }
.due-filter {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 1px solid var(--rf-border);
  background: transparent;
  color: var(--rf-text-faint);
  font-size: 10.5px;
  border-radius: 999px;
  padding: 3px 8px;
  cursor: pointer;
}
.due-filter span {
  font-family: var(--rf-mono);
  font-size: 9px;
  line-height: 13px;
  padding: 0 4px;
  border-radius: 999px;
  background: var(--rf-bg-soft);
}
.due-filter:hover { color: var(--rf-text-dim); border-color: var(--rf-border-strong); }
.due-filter.today.active { color: var(--rf-accent); border-color: rgba(168, 99, 19, .4); background: var(--rf-accent-soft); }
.due-filter.overdue.active { color: var(--rf-danger); border-color: rgba(180, 63, 54, .38); background: rgba(180, 63, 54, .06); }
.due-filter.active span { background: rgba(255, 255, 255, .65); }
.due-filter.empty { opacity: .62; }
.clear-filters {
  border: none;
  background: transparent;
  color: var(--rf-text-faint);
  font-size: 11px;
  cursor: pointer;
  padding: 4px 6px;
  border-radius: 5px;
}
.clear-filters:hover { color: var(--rf-text); background: var(--rf-card); }
.pending-filter {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 1px solid rgba(168, 99, 19, 0.3);
  background: var(--rf-accent-soft);
  color: var(--rf-accent);
  font-size: 11px;
  padding: 4px 7px;
  border-radius: 999px;
  cursor: pointer;
}
.pending-filter:hover { border-color: var(--rf-accent); }
.filter-spacer { flex: 1; }
.result-count { font-family: var(--rf-mono); font-size: 10px; color: var(--rf-text-dim); }
.hotkey-hint { font-size: 10px; color: var(--rf-text-faint); display: flex; align-items: center; gap: 4px; margin-left: 10px; }
.hotkey-hint kbd { color: var(--rf-text-dim); }

.board-main {
  flex: 1;
  min-height: 0;
  padding: 13px 18px 16px;
  background-image: radial-gradient(rgba(38, 34, 27, 0.055) 1px, transparent 1px);
  background-size: 18px 18px;
  overflow: hidden;
}
.flow { display: flex; gap: 10px; height: 100%; overflow-x: auto; overflow-y: hidden; padding-bottom: 2px; }
.flow.view-todo .column { flex-basis: 240px; }
.board-state {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  color: var(--rf-text-faint);
  font-size: 12px;
}
.error-state { color: var(--rf-danger); flex-direction: column; }
.error-state button {
  border: 1px solid var(--rf-border-strong);
  border-radius: 6px;
  background: transparent;
  color: var(--rf-text-dim);
  font-size: 11px;
  padding: 4px 10px;
  cursor: pointer;
}

.kind-toast {
  position: fixed;
  left: 50%;
  bottom: 22px;
  transform: translateX(-50%);
  z-index: 3000;
  display: flex;
  align-items: center;
  gap: 10px;
  max-width: min(620px, calc(100vw - 40px));
  padding: 9px 10px 9px 12px;
  border: 1px solid rgba(168, 99, 19, .28);
  border-radius: 11px;
  background: #fffdf8;
  box-shadow: 0 16px 44px rgba(76, 62, 43, .2);
}
.kind-toast-mark {
  width: 24px;
  height: 24px;
  border-radius: 7px;
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--rf-accent);
  background: var(--rf-accent-soft);
}
.kind-toast-copy { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.kind-toast-title { font-size: 12px; font-weight: 700; color: var(--rf-text); }
.kind-toast-sub {
  font-size: 10.5px;
  color: var(--rf-text-faint);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.kind-toast-undo {
  flex-shrink: 0;
  border: 1px solid rgba(168, 99, 19, .4);
  background: transparent;
  color: var(--rf-accent);
  border-radius: 6px;
  padding: 4px 10px;
  font-size: 11px;
  font-weight: 700;
  cursor: pointer;
}
.kind-toast-undo:hover { background: var(--rf-accent-soft); border-color: var(--rf-accent); }
.kind-toast-close {
  flex-shrink: 0;
  width: 24px;
  height: 24px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--rf-text-faint);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
}
.kind-toast-close:hover { color: var(--rf-text); background: var(--rf-bg-soft); }
.kind-toast-enter-active, .kind-toast-leave-active { transition: opacity .18s ease, transform .18s ease; }
.kind-toast-enter-from, .kind-toast-leave-to { opacity: 0; transform: translate(-50%, 8px); }

@media (max-width: 1180px) {
  .hotkey-hint { display: none; }
  .search-box { width: 200px; }
}
</style>

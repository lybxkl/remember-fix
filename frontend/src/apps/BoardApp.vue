<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus, Search, Settings2, Wrench, X } from 'lucide-vue-next'
import { call, onEvent } from '../lib/wails'
import { useBugs } from '../composables/useBugs'
import StatusColumn from '../components/StatusColumn.vue'
import BugDrawer from '../components/BugDrawer.vue'
import SettingsDialog from '../components/SettingsDialog.vue'
import EmptyArt from '../components/EmptyArt.vue'
import { CATEGORIES, SEVERITIES, STATUS_COLUMNS } from '../lib/constants'
import { defaultHotkey } from '../lib/platform'

const { bugs, error, load, updateStatus } = useBugs()

const keyword = ref('')
const filterCategory = ref('')
const filterSeverity = ref('')
const onlyPending = ref(false)
const drawerId = ref('')
const settingsOpen = ref(false)
const hotkey = ref(defaultHotkey)
const loaded = ref(false)
const searchRef = ref<HTMLInputElement>()
let offSettings: (() => void) | undefined
let offPending: (() => void) | undefined

onMounted(async () => {
  offSettings = onEvent('ui:open-settings', () => {
    settingsOpen.value = true
  })
  offPending = onEvent('ui:open-pending', () => {
    clearFilters()
    onlyPending.value = true
  })
  window.addEventListener('keydown', onKeydown)
  await initialLoad()
})
onBeforeUnmount(() => {
  offSettings?.()
  offPending?.()
  window.removeEventListener('keydown', onKeydown)
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

const filtered = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return bugs.value.filter((b) => {
    if (onlyPending.value && b.status !== 'todo' && b.status !== 'doing') return false
    if (filterCategory.value && b.category !== filterCategory.value) return false
    if (filterSeverity.value && b.severity !== filterSeverity.value) return false
    if (!kw) return true
    const hay = `${b.title} ${b.descriptionText} ${b.tags.join(' ')} ${b.sourceApp}`.toLowerCase()
    return hay.includes(kw)
  })
})

const byStatus = computed(() => {
  const map: Record<string, typeof bugs.value> = {}
  for (const c of STATUS_COLUMNS) map[c.key] = []
  for (const b of filtered.value) (map[b.status] ?? map.todo).push(b)
  return map
})

const activeFilters = computed(
  () => onlyPending.value || !!keyword.value.trim() || !!filterCategory.value || !!filterSeverity.value,
)

const stats = computed(() => {
  const weekAgo = Math.floor((Date.now() - 7 * 86400000) / 1000)
  const open = bugs.value.filter((b) => b.status === 'todo' || b.status === 'doing').length
  const closedWeek = bugs.value.filter(
    (b) =>
      (b.status === 'fixed' || b.status === 'verified') &&
      !!b.resolvedAt &&
      b.resolvedAt >= weekAgo,
  ).length
  const aiPending = bugs.value.filter((b) => b.aiStatus === 'pending').length
  return { open, closedWeek, total: bugs.value.length, aiPending }
})

function clearFilters() {
  keyword.value = ''
  filterCategory.value = ''
  filterSeverity.value = ''
  onlyPending.value = false
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
  try {
    await updateStatus(bugId, status)
  } catch (e) {
    ElMessage.error('移动失败：' + (e instanceof Error ? e.message : String(e)))
  }
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
          <span class="brand-sub">REMEMBER / FIX</span>
        </div>
      </div>

      <div class="stats">
        <div class="stat">
          <span class="stat-num">{{ stats.open }}</span>
          <span class="stat-cap">待修</span>
        </div>
        <span class="stat-sep" />
        <div class="stat">
          <span class="stat-num">{{ stats.closedWeek }}</span>
          <span class="stat-cap">本周清掉</span>
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
      <el-select v-model="filterCategory" placeholder="全部分类" clearable style="width: 126px">
        <el-option v-for="c in CATEGORIES" :key="c.key" :value="c.key" :label="c.label" />
      </el-select>
      <el-select v-model="filterSeverity" placeholder="全部严重度" clearable style="width: 122px">
        <el-option v-for="s in SEVERITIES" :key="s" :value="s" :label="s" />
      </el-select>
      <button
        v-if="onlyPending"
        class="pending-filter"
        title="移除待处理筛选"
        @click="onlyPending = false"
      >
        只看待处理
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
      <div v-else class="flow">
        <StatusColumn
          v-for="c in STATUS_COLUMNS"
          :key="c.key"
          :status="c.key"
          :label="c.label"
          :code="c.code"
          :color="c.color"
          :bugs="byStatus[c.key]"
          @open="openDrawer"
          @drop="onDrop"
        />
      </div>
    </main>

    <BugDrawer :bug-id="drawerId" @close="drawerId = ''" />
    <SettingsDialog :open="settingsOpen" @close="onSettingsClose" />
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
  letter-spacing: 0.16em;
  color: var(--rf-text-faint);
  margin-top: 2px;
}
.stats { display: flex; align-items: center; gap: 14px; margin-left: 4px; }
.stat { display: flex; align-items: baseline; gap: 5px; }
.stat-num { font-family: var(--rf-mono); font-size: 15px; font-weight: 600; color: var(--rf-text); }
.stat-cap { font-size: 10px; color: var(--rf-text-faint); letter-spacing: .02em; }
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
.search-box {
  display: flex;
  align-items: center;
  gap: 7px;
  width: 286px;
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
</style>

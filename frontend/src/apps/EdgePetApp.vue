<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { call, onEvent, startNativeDrag } from '../lib/wails'
import { defaultHotkey } from '../lib/platform'

type Dock = 'right' | 'left' | 'top' | 'bottom'

interface ReminderState {
  pending?: number
  oldestDays?: number
  dueToday?: number
  overdue?: number
  nextDueAt?: number
  pulse?: boolean
}

const DOCKS: Dock[] = ['right', 'left', 'top', 'bottom']
const expanded = ref(false)
const busy = ref(false)
const dragging = ref(false)
const dock = ref<Dock>('right')
const hotkey = ref(defaultHotkey)
const pending = ref(0)
const oldestDays = ref(0)
const dueToday = ref(0)
const overdue = ref(0)
const pulsing = ref(false)
const shellEl = ref<HTMLElement | null>(null)
let leaveTimer: number | undefined
let dragEndTimer: number | undefined
let pulseTimer: number | undefined
let pulseFrame: number | undefined
let dueTimer: number | undefined
let midnightTimer: number | undefined
let reminderEventVersion = 0
let reminderLoadSeq = 0
let offCollapse: (() => void) | undefined
let offDock: (() => void) | undefined
let offDragEnd: (() => void) | undefined
let offReminder: (() => void) | undefined
let pointerId: number | null = null
let captureEl: HTMLElement | null = null
let startX = 0
let startY = 0
let dragArmed = false
let expandTask: Promise<boolean> | null = null

function isDock(value: unknown): value is Dock {
  return typeof value === 'string' && (DOCKS as string[]).includes(value)
}

const hasPending = computed(() => pending.value > 0)
const hasOverdue = computed(() => overdue.value > 0)
const hasDueToday = computed(() => dueToday.value > 0)
const badgeLabel = computed(() => (pending.value > 9 ? '9+' : String(pending.value)))
const reminderKicker = computed(() =>
  hasOverdue.value
    ? 'OVERDUE / 已逾期'
    : hasDueToday.value
      ? 'TODAY / 今天'
      : hasPending.value
        ? 'REMINDER / 未完成'
        : 'REMEMBER / EVERYTHING',
)
const reminderTitle = computed(() => {
  if (hasOverdue.value) return `${overdue.value} 条已逾期`
  if (hasDueToday.value) return `今天有 ${dueToday.value} 件要办`
  return hasPending.value ? `${pending.value} 条还挂着` : '又想起一件事？'
})
const reminderSub = computed(() => {
  if (hasOverdue.value) return `共 ${pending.value} 条未完成`
  if (hasDueToday.value) return `总共 ${pending.value} 条未完成`
  return oldestDays.value > 0 ? `最久的一条 ${oldestDays.value} 天了` : '都是今天刚记的'
})
const recordLabel = computed(() =>
  hasPending.value
    ? `${pending.value} 条未完成；点击记一条，按住可拖动位置`
    : '记一条，按住可拖动调整位置',
)
const announcement = computed(() => {
  if (hasOverdue.value) return `${overdue.value} 条已逾期，共 ${pending.value} 条未完成`
  if (hasDueToday.value) return `今天有 ${dueToday.value} 件要办，共 ${pending.value} 条未完成`
  if (!hasPending.value) return '没有未完成记录'
  return oldestDays.value > 0
    ? `${pending.value} 条未完成，最久 ${oldestDays.value} 天`
    : `${pending.value} 条未完成，都是今天记的`
})

function applyReminder(data: ReminderState | null) {
  if (!data) return
  const count = Number(data.pending)
  const age = Number(data.oldestDays)
  const today = Number(data.dueToday)
  const late = Number(data.overdue)
  pending.value = Number.isFinite(count) && count > 0 ? Math.floor(count) : 0
  oldestDays.value = Number.isFinite(age) && age > 0 ? Math.floor(age) : 0
  dueToday.value = Number.isFinite(today) && today > 0 ? Math.floor(today) : 0
  overdue.value = Number.isFinite(late) && late > 0 ? Math.floor(late) : 0
  scheduleDueRefresh(Number(data.nextDueAt))
  if (!hasPending.value) {
    if (pulseTimer) window.clearTimeout(pulseTimer)
    if (pulseFrame) window.cancelAnimationFrame(pulseFrame)
    pulseTimer = undefined
    pulseFrame = undefined
    pulsing.value = false
  } else if (data.pulse) {
    playPulse()
  }
}

// 到点后“今日到期”会变成“已逾期”，即使提醒关闭也要刷新徽标文案。
function scheduleDueRefresh(nextDueAt: number) {
  if (dueTimer) {
    window.clearTimeout(dueTimer)
    dueTimer = undefined
  }
  if (!Number.isFinite(nextDueAt) || nextDueAt <= 0) return
  const delay = Math.min(
    Math.max(nextDueAt * 1000 - Date.now() + 1500, 1000),
    12 * 60 * 60 * 1000,
  )
  dueTimer = window.setTimeout(() => {
    dueTimer = undefined
    loadReminder()
  }, delay)
}

// 全天到期与“今天/逾期”的天数口径在午夜切换；每天刷新一次即可。
function scheduleMidnightRefresh() {
  if (midnightTimer) window.clearTimeout(midnightTimer)
  const now = new Date()
  const next = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1, 0, 0, 2)
  const delay = Math.max(next.getTime() - now.getTime(), 1000)
  midnightTimer = window.setTimeout(() => {
    midnightTimer = undefined
    loadReminder()
    scheduleMidnightRefresh()
  }, delay)
}

function playPulse() {
  if (pulseTimer) window.clearTimeout(pulseTimer)
  if (pulseFrame) window.cancelAnimationFrame(pulseFrame)
  pulsing.value = false
  // 同一帧里先移除再加回 class 不会重播动画。
  pulseFrame = window.requestAnimationFrame(() => {
    pulseFrame = undefined
    pulsing.value = true
    pulseTimer = window.setTimeout(() => {
      pulsing.value = false
      pulseTimer = undefined
    }, 2300)
  })
}

async function loadReminder() {
  const seq = ++reminderLoadSeq
  const version = reminderEventVersion
  try {
    const state = await call<ReminderState>('main.EdgeService.ReminderState')
    // 事件或更新的一次请求优先，避免旧响应覆盖新状态。
    if (seq !== reminderLoadSeq || version !== reminderEventVersion) return
    applyReminder(state)
  } catch {
    /* 保持无待处理显示 */
  }
}

async function loadConfig() {
  try {
    const cfg = await call<Record<string, string>>('main.ConfigService.GetAll')
    if (cfg?.hotkey) hotkey.value = cfg.hotkey
    if (isDock(cfg?.['edge.dock'])) dock.value = cfg['edge.dock']
  } catch {
    /* 保持默认显示 */
  }
}

onMounted(() => {
  loadConfig()
  loadReminder()
  scheduleMidnightRefresh()
  offCollapse = onEvent('edge:collapse', () => { expanded.value = false })
  offDock = onEvent<unknown>('edge:dock', (value) => {
    if (isDock(value)) dock.value = value
  })
  offReminder = onEvent<ReminderState | null>('edge:reminder', (data) => {
    reminderEventVersion++
    applyReminder(data)
  })
  // Windows 原生拖动期间 WebView 收不到 pointerup，由后端吸附完成后通知复位。
  offDragEnd = onEvent('edge:drag-end', () => {
    endDragState()
    if (dragEndTimer) window.clearTimeout(dragEndTimer)
    // 吸附后鼠标仍停在卡片上就保持展开；离开时交回正常的 hover 收起逻辑。
    dragEndTimer = window.setTimeout(() => {
      dragEndTimer = undefined
      if (dragging.value || busy.value) return
      if (!shellEl.value?.matches(':hover')) collapse()
    }, 420)
  })
})

onBeforeUnmount(() => {
  if (leaveTimer) window.clearTimeout(leaveTimer)
  if (dragEndTimer) window.clearTimeout(dragEndTimer)
  if (pulseTimer) window.clearTimeout(pulseTimer)
  if (pulseFrame) window.cancelAnimationFrame(pulseFrame)
  if (dueTimer) window.clearTimeout(dueTimer)
  if (midnightTimer) window.clearTimeout(midnightTimer)
  offCollapse?.()
  offDock?.()
  offDragEnd?.()
  offReminder?.()
})

function cancelLeave() {
  if (leaveTimer) {
    window.clearTimeout(leaveTimer)
    leaveTimer = undefined
  }
}

function cancelDragEndCheck() {
  if (dragEndTimer) {
    window.clearTimeout(dragEndTimer)
    dragEndTimer = undefined
  }
}

function ensureExpanded(): Promise<boolean> {
  if (expandTask) return expandTask
  if (expanded.value) return Promise.resolve(true)
  expanded.value = true
  let task: Promise<boolean>
  task = call('main.EdgeService.SetExpanded', true)
    .then(() => true)
    .catch(() => {
      expanded.value = false
      return false
    })
    .finally(() => {
      if (expandTask === task) expandTask = null
    })
  expandTask = task
  return task
}

async function onEnter() {
  if (dragging.value) return
  cancelLeave()
  cancelDragEndCheck()
  loadConfig()
  loadReminder()
  if (expanded.value) return
  await ensureExpanded()
}

function scheduleCollapse() {
  if (dragging.value) return
  cancelLeave()
  leaveTimer = window.setTimeout(collapse, 380)
}

async function collapse() {
  cancelLeave()
  if (!expanded.value) return
  expanded.value = false
  try {
    await call('main.EdgeService.SetExpanded', false)
  } catch {
    /* 窗口可能正在关闭 */
  }
}

async function report() {
  if (busy.value) return
  busy.value = true
  expanded.value = false
  try {
    await call('main.EdgeService.Trigger')
  } catch {
    try {
      await call('main.CaptureService.Trigger')
    } catch {
      /* 捕获窗尚未就绪 */
    }
  } finally {
    window.setTimeout(() => { busy.value = false }, 500)
  }
}

async function openBoard() {
  cancelLeave()
  try {
    await call('main.EdgeService.OpenBoard')
  } catch {
    /* 主窗尚未就绪 */
  }
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    report()
  } else if (e.key === 'Escape') {
    collapse()
  }
}

function releaseCapture() {
  if (captureEl && pointerId !== null) {
    try {
      captureEl.releasePointerCapture(pointerId)
    } catch {
      /* 原生拖窗已接管鼠标时捕获可能已被系统释放 */
    }
  }
  captureEl = null
}

function endDragState() {
  releaseCapture()
  pointerId = null
  dragArmed = false
  dragging.value = false
}

function onPointerDown(e: PointerEvent) {
  if (e.button !== 0 || pointerId !== null) return
  cancelDragEndCheck()
  pointerId = e.pointerId
  captureEl = e.currentTarget as HTMLElement
  startX = e.screenX
  startY = e.screenY
  dragArmed = false
  try {
    captureEl.setPointerCapture(e.pointerId)
  } catch {
    /* 合成事件可能没有活跃指针，仍由元素直接接收后续事件 */
  }
}

// 超过阈值后把拖窗交给系统：窗口由 OS 直接搬运，避免逐帧同步 IPC 造成的卡顿。
async function beginNativeDrag() {
  const ok = await ensureExpanded()
  if (!ok || pointerId === null) {
    endDragState()
    return
  }
  try {
    await call('main.EdgeService.BeginDrag')
  } catch {
    endDragState()
    return
  }
  if (pointerId === null) {
    void call('main.EdgeService.EndDrag')
    endDragState()
    return
  }
  // 保留指针捕获：Windows 端 Wails 需要先 ReleaseCapture 才能进入系统拖窗循环。
  startNativeDrag()
}

function onPointerMove(e: PointerEvent) {
  if (pointerId === null || e.pointerId !== pointerId || dragArmed) return
  if (Math.hypot(e.screenX - startX, e.screenY - startY) < 4) return
  dragArmed = true
  dragging.value = true
  cancelLeave()
  void beginNativeDrag()
}

function finishPointer(e: PointerEvent) {
  if (pointerId === null || e.pointerId !== pointerId) return
  const wasArmed = dragArmed
  releaseCapture()
  pointerId = null
  dragArmed = false
  if (wasArmed) {
    dragging.value = false
    // Windows 收不到这里的 pointerup，会由 WindowEndMove 收尾；macOS 则依赖这次调用。
    void call('main.EdgeService.EndDrag')
    return
  }
  report()
}

function onPointerCancel(e: PointerEvent) {
  if (pointerId === null || e.pointerId !== pointerId) return
  const wasArmed = dragArmed
  releaseCapture()
  pointerId = null
  dragArmed = false
  dragging.value = false
  if (wasArmed) void call('main.EdgeService.EndDrag')
}
</script>

<template>
  <div
    ref="shellEl"
    class="edge-shell"
    :class="['dock-' + dock, { expanded, busy, dragging, pulsing, 'has-pending': hasPending }]"
    @mouseenter="onEnter"
    @mouseleave="scheduleCollapse"
    @pointerdown="onPointerDown"
    @pointermove="onPointerMove"
    @pointerup="finishPointer"
    @pointercancel="onPointerCancel"
  >
    <span class="sr-only" aria-live="polite">{{ announcement }}</span>
    <div v-if="expanded" class="pet-card">
      <div
        class="pet-record"
        role="button"
        tabindex="0"
        :aria-label="recordLabel"
        @keydown="onKey"
      >
        <div class="pet-bubble">
          <span class="bubble-kicker">{{ reminderKicker }}</span>
          <strong>{{ reminderTitle }}</strong>
          <span class="bubble-sub">
            {{ hasPending || hasOverdue ? reminderSub : '点一下，把刚遇到的事记下来' }}
          </span>
        </div>

      <div class="pet-stage" aria-hidden="true">
        <svg class="pet-svg" viewBox="0 0 170 136" fill="none">
          <defs>
            <linearGradient id="edge-body" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0" stop-color="#efb45d" />
              <stop offset="1" stop-color="#cf8129" />
            </linearGradient>
            <linearGradient id="edge-head" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0" stop-color="#ffdca5" />
              <stop offset="1" stop-color="#f0b968" />
            </linearGradient>
            <radialGradient id="edge-glow" cx="50%" cy="50%" r="50%">
              <stop offset="0" stop-color="#e8a34a" stop-opacity="0.2" />
              <stop offset="1" stop-color="#e8a34a" stop-opacity="0" />
            </radialGradient>
          </defs>

          <ellipse cx="85" cy="124" rx="48" ry="7" fill="url(#edge-glow)" />
          <ellipse cx="85" cy="125" rx="30" ry="3.5" fill="#7f4a0c" opacity="0.12" />

          <g class="pet-bug">
            <g class="antennae" stroke="#8f520d" stroke-width="2" stroke-linecap="round">
              <path d="M76 35c-7-9-12-13-19-16" />
              <path d="M94 35c7-9 12-13 19-16" />
            </g>
            <circle cx="55" cy="18" r="3" fill="#a86313" />
            <circle cx="115" cy="18" r="3" fill="#a86313" />

            <g stroke="#8f520d" stroke-width="2.2" stroke-linecap="round" opacity="0.7">
              <path d="M57 82 37 70M59 96 34 94M59 108 39 118" />
              <path d="M113 82 133 70M111 96 136 94M111 108 131 118" />
            </g>

            <ellipse cx="85" cy="96" rx="34" ry="36" fill="url(#edge-body)" stroke="#8f520d" stroke-width="2.4" />
            <path d="M85 69v53" stroke="#fff3dc" stroke-width="2" stroke-linecap="round" stroke-dasharray="4 7" opacity="0.7" />

            <circle cx="85" cy="52" r="26" fill="url(#edge-head)" stroke="#8f520d" stroke-width="2.4" />
            <g class="eyes">
              <circle cx="76" cy="50" r="5.2" fill="#fffdf8" />
              <circle cx="94" cy="50" r="5.2" fill="#fffdf8" />
              <circle cx="77" cy="51" r="2.5" fill="#3a2a18" />
              <circle cx="95" cy="51" r="2.5" fill="#3a2a18" />
              <circle cx="78" cy="50" r="0.8" fill="#fffdf8" />
              <circle cx="96" cy="50" r="0.8" fill="#fffdf8" />
            </g>
            <ellipse cx="66" cy="59" rx="4.5" ry="2.4" fill="#d96b4c" opacity="0.35" />
            <ellipse cx="104" cy="59" rx="4.5" ry="2.4" fill="#d96b4c" opacity="0.35" />
            <path d="M80 61c2 3 8 3 10 0" stroke="#6f3d0a" stroke-width="1.8" stroke-linecap="round" />

            <g transform="translate(123 88) rotate(-24) scale(2.1)" stroke="#a86313" stroke-width="1.65" stroke-linecap="round" stroke-linejoin="round">
              <path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z" />
            </g>
          </g>

          <g class="sparkles" stroke="#a86313" stroke-width="1.6" stroke-linecap="round" opacity="0.75">
            <path d="M137 30v9M132.5 34.5h9" />
            <path d="M34 44v6M31 47h6" />
            <path d="M145 112v6M142 115h6" />
          </g>
        </svg>
      </div>

        <div class="pet-cta">
          <span>{{ busy ? '正在打开…' : '点我记一条' }}</span>
          <kbd>{{ hotkey }}</kbd>
        </div>
        <span class="pet-hint">自动带上剪贴板截图 / 文本</span>
      </div>
      <button
        v-if="hasPending"
        type="button"
        class="bubble-link"
        title="打开看板，查看未完成事项"
        @pointerdown.stop
        @pointerup.stop
        @click.stop="openBoard"
      >
        看板 →
      </button>
    </div>

    <div
      v-else
      class="edge-tab"
      role="button"
      tabindex="0"
      :aria-label="recordLabel"
      title="点击记一条，按住可拖动位置"
      @keydown="onKey"
    >
      <span
        v-if="hasPending"
        class="edge-badge"
        :class="{ overdue: hasOverdue, today: hasDueToday && !hasOverdue }"
        aria-hidden="true"
      >{{ badgeLabel }}</span>
      <svg viewBox="0 0 20 26" fill="none" aria-hidden="true">
        <g stroke="#fff8ea" stroke-width="1.35" stroke-linecap="round">
          <path d="M7 6 4.5 3.5M13 6 15.5 3.5" />
          <path d="M4.2 12.5H1.8M4.2 18H1.6M15.8 12.5h2.4M15.8 18h2.6" />
          <circle cx="10" cy="8" r="3.2" />
          <ellipse cx="10" cy="17" rx="4.4" ry="5.8" />
        </g>
      </svg>
    </div>
  </div>
</template>

<style>
html, body, #app {
  background: transparent !important;
  overflow: hidden;
}
</style>

<style scoped>
.edge-shell {
  width: 100vw;
  height: 100vh;
  display: flex;
  -webkit-user-select: none;
  user-select: none;
  font-family: var(--rf-sans);
  cursor: pointer;
  outline: none;
}
.edge-shell.dock-right { align-items: center; justify-content: flex-end; }
.edge-shell.dock-left { align-items: center; justify-content: flex-start; }
.edge-shell.dock-top { align-items: flex-start; justify-content: center; }
.edge-shell.dock-bottom { align-items: flex-end; justify-content: center; }
.edge-shell.dragging { cursor: grabbing; }
.edge-shell.dragging * { cursor: grabbing; }
/* 系统拖窗时窗口内容通常不重绘，暂停装饰动画减少合成开销。 */
.edge-shell.dragging .pet-card { animation: none; }
.edge-shell.dragging .pet-bug,
.edge-shell.dragging .antennae,
.edge-shell.dragging .eyes,
.edge-shell.dragging .sparkles { animation-play-state: paused; }
.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  margin: -1px;
  padding: 0;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
  border: 0;
}
.edge-tab:focus-visible { outline: 1px solid var(--rf-accent); outline-offset: 2px; }

.edge-tab {
  position: relative;
  width: 18px;
  height: 64px;
  flex-shrink: 0;
  border-radius: 9px 0 0 9px;
  background: linear-gradient(180deg, #c98633 0%, #a86313 58%, #8f520d 100%);
  box-shadow: inset 1px 0 0 rgba(255, 253, 248, 0.35);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 2;
  transition: filter .16s ease;
}
.edge-shell.dock-left .edge-tab {
  border-radius: 0 9px 9px 0;
  box-shadow: inset -1px 0 0 rgba(255, 253, 248, 0.35);
}
.edge-shell.dock-top .edge-tab,
.edge-shell.dock-bottom .edge-tab {
  width: 64px;
  height: 18px;
}
.edge-shell.dock-top .edge-tab {
  border-radius: 0 0 9px 9px;
  box-shadow: inset 0 -1px 0 rgba(255, 253, 248, 0.35);
}
.edge-shell.dock-bottom .edge-tab {
  border-radius: 9px 9px 0 0;
  box-shadow: inset 0 1px 0 rgba(255, 253, 248, 0.35);
}
.edge-tab svg { width: 13px; height: 17px; filter: drop-shadow(0 1px 1px rgba(60, 37, 8, 0.25)); }
.edge-shell:hover .edge-tab { filter: brightness(1.08); }

/* 徽标只叠在 24px 收起标签内，不改变窗口尺寸。 */
.edge-badge {
  position: absolute;
  z-index: 3;
  min-width: 14px;
  height: 14px;
  padding: 0 2px;
  box-sizing: border-box;
  border-radius: 999px;
  border: 1px solid rgba(168, 99, 19, 0.42);
  background: #fff8ea;
  color: var(--rf-accent-strong);
  font-family: var(--rf-mono);
  font-size: 8.5px;
  font-weight: 700;
  line-height: 12px;
  text-align: center;
  font-variant-numeric: tabular-nums;
  box-shadow: 0 2px 7px rgba(76, 62, 43, 0.2);
  pointer-events: none;
}
.edge-badge.overdue {
  background: var(--rf-danger);
  color: #fff8ea;
  border-color: rgba(148, 52, 44, 0.75);
  box-shadow: 0 2px 8px rgba(180, 63, 54, 0.35);
}
.edge-badge.today {
  background: #fff1d2;
  color: #8f520d;
  border-color: rgba(168, 99, 19, 0.55);
}
.edge-shell.dock-right .edge-badge { top: 1px; left: 2px; }
.edge-shell.dock-left .edge-badge { top: 1px; right: 2px; }
.edge-shell.dock-top .edge-badge { top: 1px; right: 2px; }
.edge-shell.dock-bottom .edge-badge { bottom: 1px; right: 2px; }

.edge-tab::after {
  content: '';
  position: absolute;
  inset: -3px;
  border-radius: inherit;
  border: 1px solid rgba(255, 248, 234, 0.85);
  opacity: 0;
  pointer-events: none;
}
.edge-shell.pulsing .edge-tab::after { animation: tab-ping 2s ease-out 1; }
.edge-shell.pulsing .edge-badge { animation: badge-pop 2.2s cubic-bezier(.2, .8, .2, 1) 1; }

.pet-card {
  box-sizing: border-box;
  width: 184px;
  height: 232px;
  flex-shrink: 0;
  border-radius: 18px;
  border: 1px solid var(--rf-border);
  background: linear-gradient(180deg, #fffdf9 0%, #faf5ea 100%);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.9);
  overflow: hidden;
  position: relative;
  animation: card-in .18s cubic-bezier(.2, .8, .2, 1);
}
.pet-record {
  box-sizing: border-box;
  width: 100%;
  height: 100%;
  padding: 12px 12px 10px;
  border-radius: 17px;
  display: flex;
  flex-direction: column;
  align-items: center;
  cursor: pointer;
}
.pet-record:focus-visible { outline: 1px solid var(--rf-accent); outline-offset: -3px; }
.bubble-link {
  position: absolute;
  top: 8px;
  right: 9px;
  z-index: 3;
  padding: 2px 4px;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: var(--rf-accent);
  font-family: var(--rf-sans);
  font-size: 9px;
  font-weight: 700;
  line-height: 1.2;
  cursor: pointer;
}
.bubble-link:hover { background: rgba(168, 99, 19, 0.1); text-decoration: underline; text-underline-offset: 2px; }
.bubble-link:focus-visible { outline: 1px solid var(--rf-accent); outline-offset: 1px; }
.pet-card::before {
  content: '';
  position: absolute;
  top: 0;
  left: 20px;
  right: 20px;
  height: 2px;
  border-radius: 0 0 2px 2px;
  background: linear-gradient(90deg, transparent, rgba(168, 99, 19, 0.5), transparent);
}
.pet-bubble { text-align: center; line-height: 1.25; flex-shrink: 0; }
.bubble-kicker {
  display: block;
  font-family: var(--rf-mono);
  font-size: 7px;
  letter-spacing: 0.14em;
  color: var(--rf-accent);
  opacity: 0.8;
}
.pet-bubble strong { display: block; margin-top: 3px; font-size: 14px; letter-spacing: -0.01em; color: var(--rf-text); }
.bubble-sub { display: block; margin-top: 2px; font-size: 9.5px; color: var(--rf-text-faint); }
.pet-stage {
  flex: 1;
  min-height: 0;
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 2px 0 0;
}
.pet-svg { width: 148px; height: 112px; overflow: visible; }
.pet-bug { animation: pet-float 4.4s ease-in-out infinite; transform-box: fill-box; transform-origin: center; }
.antennae { animation: antenna-sway 3.6s ease-in-out infinite; transform-box: fill-box; transform-origin: center bottom; }
.eyes { animation: pet-blink 5.2s ease-in-out infinite; transform-box: fill-box; transform-origin: center; }
.sparkles { animation: sparkle 3s ease-in-out infinite; transform-box: fill-box; transform-origin: center; }

.pet-cta {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
  border: 1px solid rgba(168, 99, 19, 0.48);
  background: rgba(168, 99, 19, 0.06);
  color: var(--rf-accent);
  border-radius: 8px;
  padding: 5px 10px;
  font-size: 11.5px;
  font-weight: 700;
  transition: background .15s ease, border-color .15s ease;
}
.pet-card:hover .pet-cta { background: rgba(168, 99, 19, 0.11); border-color: var(--rf-accent); }
.pet-cta kbd { font-size: 8.5px; line-height: 14px; color: var(--rf-accent); border-color: rgba(168, 99, 19, 0.26); background: rgba(255, 253, 248, 0.7); }
.pet-hint { flex-shrink: 0; margin-top: 3px; font-size: 8.5px; color: var(--rf-text-faint); }

.edge-shell.dock-left .pet-card { animation-name: card-in-left; }
.edge-shell.dock-top .pet-card { animation-name: card-in-top; }
.edge-shell.dock-bottom .pet-card { animation-name: card-in-bottom; }

@keyframes card-in {
  from { opacity: 0; transform: translateX(14px) scale(.985); }
  to { opacity: 1; transform: translateX(0) scale(1); }
}
@keyframes card-in-left {
  from { opacity: 0; transform: translateX(-14px) scale(.985); }
  to { opacity: 1; transform: translateX(0) scale(1); }
}
@keyframes card-in-top {
  from { opacity: 0; transform: translateY(-14px) scale(.985); }
  to { opacity: 1; transform: translateY(0) scale(1); }
}
@keyframes card-in-bottom {
  from { opacity: 0; transform: translateY(14px) scale(.985); }
  to { opacity: 1; transform: translateY(0) scale(1); }
}
@keyframes pet-float {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-4px); }
}
@keyframes antenna-sway {
  0%, 100% { transform: rotate(0deg); }
  50% { transform: rotate(2.5deg); }
}
@keyframes pet-blink {
  0%, 92%, 100% { transform: scaleY(1); }
  95% { transform: scaleY(0.1); }
}
@keyframes sparkle {
  0%, 100% { opacity: .35; }
  50% { opacity: .9; }
}
@keyframes tab-ping {
  0% { opacity: .9; transform: scale(.8); }
  70% { opacity: 0; transform: scale(1.25); }
  100% { opacity: 0; transform: scale(1.25); }
}
@keyframes badge-pop {
  0% { transform: scale(1); }
  18% { transform: scale(1.28); }
  36% { transform: scale(.96); }
  54% { transform: scale(1.08); }
  100% { transform: scale(1); }
}
@media (prefers-reduced-motion: reduce) {
  .pet-card { animation: none; }
  .pet-bug, .antennae, .eyes, .sparkles { animation: none; }
  .edge-shell.pulsing .edge-tab::after { animation: none; opacity: .7; }
  .edge-shell.pulsing .edge-badge {
    animation: none;
    box-shadow: 0 0 0 3px rgba(255, 248, 234, .45), 0 2px 7px rgba(76, 62, 43, .2);
  }
}
</style>

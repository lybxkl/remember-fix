<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { CalendarDays, X } from 'lucide-vue-next'
import { dueMeta, duePreset, startOfLocalDay } from '../lib/constants'

const props = defineProps<{
  modelValue: number
  allDay: boolean
  compact?: boolean
}>()
const emit = defineEmits<{
  (e: 'update:modelValue', value: number): void
  (e: 'update:allDay', value: boolean): void
}>()

const dateStr = ref('')
const timeStr = ref('')
const clock = ref(Date.now())
let clockTimer: number | undefined

onMounted(() => {
  clockTimer = window.setInterval(() => {
    clock.value = Date.now()
  }, 60_000)
})
onBeforeUnmount(() => {
  if (clockTimer) window.clearInterval(clockTimer)
})

function toDateStr(ts: number): string {
  const d = new Date(ts * 1000)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

function toTimeStr(ts: number): string {
  const d = new Date(ts * 1000)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

function syncFromProps() {
  if (!props.modelValue) {
    dateStr.value = ''
    timeStr.value = ''
    return
  }
  dateStr.value = toDateStr(props.modelValue)
  timeStr.value = props.allDay ? '' : toTimeStr(props.modelValue)
}
watch(() => [props.modelValue, props.allDay], syncFromProps, { immediate: true })

const summary = computed(() => dueMeta(props.modelValue, props.allDay, false, clock.value))
const presetToday = computed(() => duePreset('today', new Date(clock.value)))
const presetTomorrow = computed(() => duePreset('tomorrow', new Date(clock.value)))
const presetWeekend = computed(() => duePreset('weekend', new Date(clock.value)))

const activePreset = computed(() => {
  if (!props.modelValue) return ''
  const due = startOfLocalDay(new Date(props.modelValue * 1000)).getTime()
  if (due === startOfLocalDay(new Date(presetToday.value * 1000)).getTime()) return 'today'
  if (due === startOfLocalDay(new Date(presetTomorrow.value * 1000)).getTime()) return 'tomorrow'
  if (due === startOfLocalDay(new Date(presetWeekend.value * 1000)).getTime()) return 'weekend'
  return ''
})

function emitDue(date: string, time: string) {
  if (!date) {
    emit('update:modelValue', 0)
    emit('update:allDay', true)
    return
  }
  const [y, m, d] = date.split('-').map(Number)
  if (!y || !m || !d) return
  if (time) {
    const [hh, mm] = time.split(':').map(Number)
    emit('update:modelValue', Math.floor(new Date(y, m - 1, d, hh || 0, mm || 0, 0).getTime() / 1000))
    emit('update:allDay', false)
  } else {
    emit('update:modelValue', Math.floor(new Date(y, m - 1, d, 0, 0, 0).getTime() / 1000))
    emit('update:allDay', true)
  }
}

function setPreset(kind: 'today' | 'tomorrow' | 'weekend') {
  const date = toDateStr(duePreset(kind))
  dateStr.value = date
  timeStr.value = ''
  emitDue(date, '')
}

function onDateChange(value: string | null) {
  dateStr.value = value ?? ''
  if (!value) {
    timeStr.value = ''
    emitDue('', '')
    return
  }
  emitDue(value, timeStr.value)
}

function onTimeChange(value: string | null) {
  timeStr.value = value ?? ''
  if (dateStr.value) emitDue(dateStr.value, timeStr.value)
}

function onNativeDate(e: Event) {
  onDateChange((e.target as HTMLInputElement).value)
}

function onNativeTime(e: Event) {
  onTimeChange((e.target as HTMLInputElement).value)
}

function clear() {
  dateStr.value = ''
  timeStr.value = ''
  emitDue('', '')
}
</script>

<template>
  <div class="due-field" :class="{ compact, 'has-value': !!modelValue }">
    <span class="due-label"><CalendarDays :size="compact ? 11 : 12" /> 到期</span>
    <div class="due-presets">
      <button
        type="button"
        class="due-preset"
        :class="{ active: activePreset === 'today' }"
        @click="setPreset('today')"
      >今天</button>
      <button
        type="button"
        class="due-preset"
        :class="{ active: activePreset === 'tomorrow' }"
        @click="setPreset('tomorrow')"
      >明天</button>
      <button
        type="button"
        class="due-preset"
        :class="{ active: activePreset === 'weekend' }"
        @click="setPreset('weekend')"
      >周末</button>
    </div>
    <input
      class="due-date"
      type="date"
      :value="dateStr"
      aria-label="到期日期"
      @change="onNativeDate"
    />
    <input
      class="due-time"
      type="time"
      :value="timeStr"
      :disabled="!dateStr"
      aria-label="到期时间，可选"
      @change="onNativeTime"
    />
    <button v-if="modelValue" type="button" class="due-clear" title="清除到期时间" @click="clear">
      <X :size="11" />
    </button>
    <span v-if="summary" class="due-summary" :class="summary.tone">{{ summary.text }}</span>
  </div>
</template>

<style scoped>
.due-field {
  display: flex;
  align-items: center;
  gap: 5px;
  flex-wrap: wrap;
  min-width: 0;
}
.due-label {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 10.5px;
  font-weight: 700;
  letter-spacing: .06em;
  color: var(--rf-text-faint);
  white-space: nowrap;
}
.due-presets { display: inline-flex; gap: 3px; }
.due-preset {
  border: 1px solid var(--rf-border);
  background: var(--rf-panel);
  color: var(--rf-text-faint);
  border-radius: 5px;
  font-size: 10.5px;
  line-height: 1;
  padding: 4px 6px;
  cursor: pointer;
  transition: color .14s ease, border-color .14s ease, background .14s ease;
}
.due-preset:hover { color: var(--rf-text-dim); border-color: var(--rf-border-strong); }
.due-preset.active {
  color: var(--rf-accent);
  border-color: rgba(168, 99, 19, .42);
  background: var(--rf-accent-soft);
}
.due-date, .due-time {
  height: 25px;
  border: 1px solid var(--rf-border);
  border-radius: 6px;
  background: var(--rf-panel);
  color: var(--rf-text-dim);
  font-family: var(--rf-sans);
  font-size: 10.5px;
  padding: 0 5px;
  outline: none;
  transition: border-color .14s ease, background .14s ease;
}
.due-date { width: 122px; }
.due-time { width: 94px; }
.due-date:hover, .due-time:hover { border-color: var(--rf-border-strong); }
.due-date:focus, .due-time:focus { border-color: var(--rf-accent); background: var(--rf-card); }
.due-time:disabled { opacity: .5; cursor: default; }
.due-date::-webkit-calendar-picker-indicator,
.due-time::-webkit-calendar-picker-indicator { opacity: .5; cursor: pointer; }
.due-date::-webkit-calendar-picker-indicator:hover,
.due-time::-webkit-calendar-picker-indicator:hover { opacity: .85; }
.due-clear {
  width: 19px;
  height: 19px;
  border: none;
  border-radius: 50%;
  background: transparent;
  color: var(--rf-text-faint);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
}
.due-clear:hover { color: var(--rf-danger); background: rgba(180, 63, 54, .07); }
.due-summary {
  margin-left: auto;
  font-size: 10px;
  color: var(--rf-text-faint);
  white-space: nowrap;
}
.due-summary.overdue { color: var(--rf-danger); font-weight: 700; }
.due-summary.today { color: var(--rf-accent); font-weight: 700; }
.due-summary.soon { color: var(--rf-text-dim); }
.compact .due-label { font-size: 10px; }
.compact .due-preset { padding: 3px 5px; font-size: 10px; }
.compact .due-date { width: 108px; }
.compact .due-time { width: 82px; }
</style>

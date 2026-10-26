// 看板列 / 类型 / 分类 / 严重度 / 到期时间的展示定义
import type { Bug, Kind } from './types'

export interface ColumnDef {
  key: string
  label: string
  code: string
  color: string
  statuses: string[]
  dropStatus: string
}

export const KIND_META: Record<Kind, { label: string; short: string; color: string; soft: string }> = {
  bug: { label: 'Bug', short: 'BUG', color: '#a86313', soft: 'rgba(168, 99, 19, 0.08)' },
  todo: { label: '待办', short: 'TODO', color: '#3b68ad', soft: 'rgba(59, 104, 173, 0.08)' },
}

export const BUG_STATUS_COLUMNS: ColumnDef[] = [
  { key: 'todo', label: '待处理', code: 'TODO', color: '#a86313', statuses: ['todo'], dropStatus: 'todo' },
  { key: 'doing', label: '进行中', code: 'DOING', color: '#3b68ad', statuses: ['doing'], dropStatus: 'doing' },
  { key: 'fixed', label: '已修复', code: 'FIXED', color: '#2f7d57', statuses: ['fixed'], dropStatus: 'fixed' },
  { key: 'verified', label: '已验证', code: 'VERIFIED', color: '#1f7f76', statuses: ['verified'], dropStatus: 'verified' },
  { key: 'wontfix', label: '不修', code: 'WONTFIX', color: '#8a8377', statuses: ['wontfix'], dropStatus: 'wontfix' },
]

export const TODO_STATUS_COLUMNS: ColumnDef[] = [
  { key: 'todo', label: '待办', code: 'TODO', color: '#a86313', statuses: ['todo'], dropStatus: 'todo' },
  { key: 'doing', label: '进行中', code: 'DOING', color: '#3b68ad', statuses: ['doing'], dropStatus: 'doing' },
  { key: 'fixed', label: '已完成', code: 'DONE', color: '#2f7d57', statuses: ['fixed'], dropStatus: 'fixed' },
  { key: 'wontfix', label: '已搁置', code: 'PAUSED', color: '#8a8377', statuses: ['wontfix'], dropStatus: 'wontfix' },
]

export const ALL_STATUS_COLUMNS: ColumnDef[] = [
  { key: 'todo', label: '待处理', code: 'TODO', color: '#a86313', statuses: ['todo'], dropStatus: 'todo' },
  { key: 'doing', label: '进行中', code: 'DOING', color: '#3b68ad', statuses: ['doing'], dropStatus: 'doing' },
  { key: 'fixed', label: '已完成', code: 'DONE', color: '#2f7d57', statuses: ['fixed', 'verified'], dropStatus: 'fixed' },
  { key: 'wontfix', label: '已搁置', code: 'PAUSED', color: '#8a8377', statuses: ['wontfix'], dropStatus: 'wontfix' },
]

export type BoardView = 'all' | 'bug' | 'todo'

export function boardColumns(view: BoardView): ColumnDef[] {
  if (view === 'bug') return BUG_STATUS_COLUMNS
  if (view === 'todo') return TODO_STATUS_COLUMNS
  return ALL_STATUS_COLUMNS
}

export const CATEGORIES = [
  { key: 'uncategorized', label: '未分类' },
  { key: 'ui', label: '界面 UI' },
  { key: 'logic', label: '逻辑' },
  { key: 'crash', label: '崩溃' },
  { key: 'performance', label: '性能' },
  { key: 'compat', label: '兼容性' },
  { key: 'data', label: '数据' },
  { key: 'security', label: '安全' },
  { key: 'other', label: '其他' },
] as const

export const SEVERITIES = ['P0', 'P1', 'P2', 'P3'] as const

export const SEVERITY_META: Record<string, { label: string; color: string }> = {
  P0: { label: 'P0 致命', color: '#b43f36' },
  P1: { label: 'P1 严重', color: '#b25e1d' },
  P2: { label: 'P2 一般', color: '#8a6a12' },
  P3: { label: 'P3 轻微', color: '#7a756a' },
}

export function kindLabel(kind: Kind | string): string {
  return KIND_META[kind as Kind]?.label ?? '事项'
}

export function kindSourceLabel(source: string): string {
  if (source === 'user') return '手动选择'
  if (source === 'ai') return 'AI 判断'
  return '自动判断'
}

export function categoryLabel(key: string): string {
  return CATEGORIES.find((c) => c.key === key)?.label ?? key
}

export function statusOptions(kind: Kind): { key: string; label: string }[] {
  const cols = kind === 'todo' ? TODO_STATUS_COLUMNS : BUG_STATUS_COLUMNS
  return cols.filter((c) => !(kind === 'todo' && c.key === 'verified')).map((c) => ({ key: c.key, label: c.label }))
}

export function statusLabel(key: string, kind: Kind = 'bug'): string {
  const col = statusOptions(kind).find((c) => c.key === key)
  return col?.label ?? key
}

export function isOpen(status: string): boolean {
  return status === 'todo' || status === 'doing'
}

export function isDoneStatus(status: string): boolean {
  return status === 'fixed' || status === 'verified'
}

export function formatTime(unix: number): string {
  if (!unix) return ''
  const d = new Date(unix * 1000)
  const now = new Date()
  const sameDay = d.toDateString() === now.toDateString()
  const hm = `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
  if (sameDay) return hm
  return `${d.getMonth() + 1}/${d.getDate()} ${hm}`
}

export function relativeTime(unix: number, nowMs = Date.now()): string {
  if (!unix) return ''
  const now = nowMs
  const diff = Math.max(0, now - unix * 1000)
  const min = Math.floor(diff / 60000)
  if (min < 1) return '刚刚'
  if (min < 60) return `${min} 分钟前`
  const hours = Math.floor(min / 60)
  if (hours < 24) return `${hours} 小时前`
  const days = Math.floor(hours / 24)
  if (days === 1) return '昨天'
  if (days < 7) return `${days} 天前`
  return formatTime(unix)
}

export function firstLine(text: string, max = 40): string {
  const line = text.split('\n').find((l) => l.trim()) ?? ''
  return line.length > max ? line.slice(0, max) + '…' : line
}

export type DueTone = 'overdue' | 'today' | 'soon' | 'normal' | 'done'

export interface DueMeta {
  text: string
  tone: DueTone
  title: string
}

export function startOfLocalDay(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate())
}

export function duePreset(kind: 'today' | 'tomorrow' | 'weekend', from = new Date()): number {
  const d = startOfLocalDay(from)
  if (kind === 'tomorrow') d.setDate(d.getDate() + 1)
  if (kind === 'weekend') {
    const day = d.getDay()
    if (day !== 0 && day !== 6) d.setDate(d.getDate() + (6 - day))
  }
  return Math.floor(d.getTime() / 1000)
}

export function dueMeta(dueAt: number, allDay: boolean, done = false, nowMs = Date.now()): DueMeta | null {
  if (!dueAt) return null
  const due = new Date(dueAt * 1000)
  const now = new Date(nowMs)
  const today = startOfLocalDay(now)
  const dueDay = startOfLocalDay(due)
  const dayDiff = Math.round((dueDay.getTime() - today.getTime()) / 86400000)
  const hm = `${String(due.getHours()).padStart(2, '0')}:${String(due.getMinutes()).padStart(2, '0')}`
  const shortDate = `${due.getMonth() + 1}/${due.getDate()}`
  const fullDate = `${due.getFullYear()}-${String(due.getMonth() + 1).padStart(2, '0')}-${String(due.getDate()).padStart(2, '0')}`
  const title = `${fullDate} ${allDay ? '全天' : hm}`
  if (done) {
    return { text: allDay ? shortDate : `${shortDate} ${hm}`, tone: 'done', title }
  }
  if (allDay) {
    if (dayDiff < 0) return { text: `逾期 ${Math.abs(dayDiff)} 天`, tone: 'overdue', title }
    if (dayDiff === 0) return { text: '今天到期', tone: 'today', title }
    if (dayDiff === 1) return { text: '明天到期', tone: 'soon', title }
    if (dayDiff <= 3) return { text: `${dayDiff} 天后到期`, tone: 'soon', title }
    return { text: `${shortDate} 到期`, tone: 'normal', title }
  }
  if (due.getTime() < now.getTime()) {
    const hours = Math.floor((now.getTime() - due.getTime()) / 3600000)
    if (dayDiff === 0 && hours < 1) return { text: '刚刚到期', tone: 'overdue', title }
    if (dayDiff === 0) return { text: `逾期 ${hours} 小时`, tone: 'overdue', title }
    return { text: `逾期 ${Math.abs(dayDiff)} 天`, tone: 'overdue', title }
  }
  if (dayDiff === 0) return { text: `今天 ${hm}`, tone: 'today', title }
  if (dayDiff === 1) return { text: `明天 ${hm}`, tone: 'soon', title }
  if (dayDiff <= 3) return { text: `${dayDiff} 天后 ${hm}`, tone: 'soon', title }
  return { text: `${shortDate} ${hm}`, tone: 'normal', title }
}

export function dueBucket(item: Bug, nowMs = Date.now()): 'overdue' | 'today' | '' {
  if (item.kind !== 'todo') return ''
  const meta = dueMeta(item.dueAt, item.dueAllDay, isDoneStatus(item.status), nowMs)
  if (!meta) return ''
  if (meta.tone === 'overdue') return 'overdue'
  if (meta.tone === 'today') return 'today'
  return ''
}

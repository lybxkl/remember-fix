// 看板列 / 分类 / 严重度的展示定义

export const STATUS_COLUMNS = [
  { key: 'todo', label: '待处理', code: 'TODO', color: '#a86313' },
  { key: 'doing', label: '进行中', code: 'DOING', color: '#3b68ad' },
  { key: 'fixed', label: '已修复', code: 'FIXED', color: '#2f7d57' },
  { key: 'verified', label: '已验证', code: 'VERIFIED', color: '#1f7f76' },
  { key: 'wontfix', label: '不修', code: 'WONTFIX', color: '#8a8377' },
] as const

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

export function categoryLabel(key: string): string {
  return CATEGORIES.find((c) => c.key === key)?.label ?? key
}

export function statusLabel(key: string): string {
  return STATUS_COLUMNS.find((c) => c.key === key)?.label ?? key
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

export function relativeTime(unix: number): string {
  if (!unix) return ''
  const now = Date.now()
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

// 与 Go 侧 internal/models JSON 结构对齐
export type Kind = 'bug' | 'todo'
export type KindSource = 'heuristic' | 'ai' | 'user'

export interface Attachment {
  type: 'image' | 'text'
  path: string
  text?: string
}

export interface AISuggestion {
  kind: Kind
  kindConfidence: number
  title: string
  category: string
  severity: string
  tags: string[]
  summary: string
  steps: string[]
  dueAt: number
  dueAllDay: boolean
  model: string
  at: number
}

export interface Bug {
  id: string
  kind: Kind
  kindSource: KindSource
  title: string
  description: string
  descriptionText: string
  status: string
  category: string
  severity: string
  dueAt: number
  dueAllDay: boolean
  tags: string[]
  attachments: Attachment[]
  sourceApp: string
  ai: AISuggestion | null
  aiStatus: 'none' | 'pending' | 'done' | 'failed'
  createdAt: number
  updatedAt: number
  resolvedAt: number | null
}

export interface IntentResult {
  kind: Kind
  confidence: number
  source: 'heuristic' | 'fallback'
  confident: boolean
  reason: string
}

export interface KindChange {
  id: string
  title: string
  from: Kind
  to: Kind
  fromStatus: string
  fromDueAt: number
  toDueAt: number
  confidence: number
}

export interface KindUndone {
  id: string
  kind: Kind
  status: string
  dueAt: number
  dueAllDay: boolean
}

export interface CaptureItem {
  id: string
  type: 'image' | 'text'
  path: string
  text: string
  seq: number
  at: number
  sourceApp: string
}

export interface Snapshot {
  selectedId: string
  items: CaptureItem[]
}

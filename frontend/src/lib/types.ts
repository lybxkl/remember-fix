// 与 Go 侧 internal/models JSON 结构对齐
export interface Attachment {
  type: 'image' | 'text'
  path: string
  text?: string
}

export interface AISuggestion {
  title: string
  category: string
  severity: string
  tags: string[]
  summary: string
  steps: string[]
  model: string
  at: number
}

export interface Bug {
  id: string
  title: string
  description: string
  descriptionText: string
  status: string
  category: string
  severity: string
  tags: string[]
  attachments: Attachment[]
  sourceApp: string
  ai: AISuggestion | null
  aiStatus: 'none' | 'pending' | 'done' | 'failed'
  createdAt: number
  updatedAt: number
  resolvedAt: number | null
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

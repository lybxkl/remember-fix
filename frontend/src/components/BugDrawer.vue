<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Copy, RefreshCw, Sparkles, Trash2, WandSparkles, X } from 'lucide-vue-next'
import RichEditor from './RichEditor.vue'
import { useBugs } from '../composables/useBugs'
import { call, onEvent } from '../lib/wails'
import type { Bug } from '../lib/types'
import { CATEGORIES, SEVERITIES, SEVERITY_META, STATUS_COLUMNS, categoryLabel, formatTime } from '../lib/constants'
import { renderMarkdown } from '../lib/markdown'

const props = defineProps<{ bugId: string }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'saved'): void }>()

const { save, remove, reanalyze, fixPrompt } = useBugs()

const form = ref<Bug | null>(null)
const fixPromptHtml = ref('')
const fixPromptRaw = ref('')
const fixPromptLoading = ref(false)
const editorRef = ref<InstanceType<typeof RichEditor>>()
let offBug: (() => void) | undefined
let offDeleted: (() => void) | undefined

watch(
  () => props.bugId,
  async (id) => {
    if (!id) return
    fixPromptHtml.value = ''
    fixPromptRaw.value = ''
    const b = await call<Bug>('main.BugService.Get', id)
    form.value = JSON.parse(JSON.stringify(b))
  },
  { immediate: true },
)

onMounted(() => {
  offBug = onEvent<Bug>('bug:changed', (b) => {
    const f = form.value
    if (!f || !b || b.id !== f.id) return
    const titleWasEmpty = !f.title.trim()
    f.ai = b.ai
    f.aiStatus = b.aiStatus
    if (b.aiStatus === 'done') {
      if (titleWasEmpty) f.title = b.title
      if (f.category === 'uncategorized') f.category = b.category
      if (!f.tags?.length) f.tags = [...(b.tags ?? [])]
      if (titleWasEmpty && f.severity === 'P2' && b.severity !== 'P2') f.severity = b.severity
    }
  })
  offDeleted = onEvent<{ id: string }>('bug:deleted', ({ id }) => {
    if (form.value?.id === id) emit('close')
  })
  window.addEventListener('keydown', onKeydown, true)
})

onBeforeUnmount(() => {
  offBug?.()
  offDeleted?.()
  window.removeEventListener('keydown', onKeydown, true)
})

const sevColor = computed(() => (s: string) => SEVERITY_META[s]?.color ?? '#7a756a')

function onKeydown(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
    e.preventDefault()
    persist()
    return
  }
  if (e.key === 'Escape' && !document.querySelector('.el-image-viewer__wrapper')) {
    emit('close')
  }
}

async function persist() {
  const f = form.value
  if (!f) return
  f.descriptionText = htmlToText(f.description)
  if (!f.title.trim() && !f.descriptionText.trim()) {
    ElMessage.warning('标题和描述至少填一个')
    return
  }
  await save(f)
  ElMessage.success('已保存')
  emit('saved')
  emit('close')
}

function htmlToText(html: string): string {
  if (!html) return ''
  return (new DOMParser().parseFromString(html, 'text/html').body.textContent ?? '').trim()
}

async function onDelete() {
  const f = form.value
  if (!f) return
  await ElMessageBox.confirm('确定删除这条记录吗？截图会在后续清理中一并移除。', '删除确认', {
    type: 'warning',
    confirmButtonText: '删除',
    cancelButtonText: '取消',
  })
  await remove(f.id)
  ElMessage.success('已删除')
  emit('close')
}

function applyAI() {
  const f = form.value
  const ai = f?.ai
  if (!f || !ai) return
  if (!f.title.trim()) f.title = ai.title
  f.category = ai.category
  f.severity = ai.severity
  f.tags = [...new Set([...(f.tags ?? []), ...ai.tags])]
  ElMessage.success('已应用 AI 建议')
}

function onRetryAI() {
  const f = form.value
  if (!f) return
  f.aiStatus = 'pending'
  reanalyze(f.id).catch((e) => {
    f.aiStatus = f.ai?.at ? 'done' : (f.aiStatus === 'pending' ? 'none' : f.aiStatus)
    ElMessage.error(e instanceof Error ? e.message : String(e))
  })
}

function insertShot() {
  const f = form.value
  const img = f?.attachments.find((a) => a.type === 'image' && a.path)
  if (!img) {
    ElMessage.info('这条记录没有截图附件')
    return
  }
  editorRef.value?.insertImage('/shots/' + img.path)
}

async function onFixPrompt() {
  const f = form.value
  if (!f) return
  fixPromptLoading.value = true
  try {
    fixPromptRaw.value = await fixPrompt(f.id)
    fixPromptHtml.value = renderMarkdown(fixPromptRaw.value)
  } catch (e) {
    ElMessage.error('生成失败：' + (e instanceof Error ? e.message : String(e)))
  } finally {
    fixPromptLoading.value = false
  }
}

async function copyFixPrompt() {
  const f = form.value
  if (!f) return
  try {
    if (!fixPromptRaw.value) {
      fixPromptLoading.value = true
      fixPromptRaw.value = await fixPrompt(f.id)
      fixPromptHtml.value = renderMarkdown(fixPromptRaw.value)
    }
    const ok = await call<boolean>('main.ConfigService.CopyText', fixPromptRaw.value)
    if (ok) ElMessage.success('修复指令已复制，可直接粘贴给 AI 编程工具')
    else ElMessage.error('复制失败')
  } catch (e) {
    ElMessage.error('生成失败：' + (e instanceof Error ? e.message : String(e)))
  } finally {
    fixPromptLoading.value = false
  }
}
</script>

<template>
  <el-drawer
    :model-value="!!bugId"
    size="620px"
    :with-header="false"
    :close-on-press-escape="false"
    destroy-on-close
    class="bug-drawer"
    @close="emit('close')"
  >
    <div v-if="form" class="drawer">
      <header class="drawer-head">
        <div class="drawer-id">
          <span class="eyebrow">BUG / {{ form.id.slice(0, 8).toUpperCase() }}</span>
          <span class="drawer-created">{{ formatTime(form.createdAt) }} 创建<span v-if="form.sourceApp"> · {{ form.sourceApp }}</span></span>
        </div>
        <button class="icon-btn" title="关闭" @click="emit('close')"><X :size="17" /></button>
      </header>

      <div class="drawer-scroll">
        <input v-model="form.title" class="title-input" placeholder="标题（留空则由 AI 生成）" />

        <div class="meta-row">
          <el-select v-model="form.status" style="width: 108px">
            <el-option v-for="c in STATUS_COLUMNS" :key="c.key" :value="c.key" :label="c.label" />
          </el-select>
          <el-select v-model="form.category" style="width: 120px">
            <el-option v-for="c in CATEGORIES" :key="c.key" :value="c.key" :label="c.label" />
          </el-select>
          <el-select v-model="form.severity" style="width: 116px">
            <el-option v-for="s in SEVERITIES" :key="s" :value="s">
              <span :style="{ color: sevColor(s), fontWeight: 700 }">{{ s }}</span>
              <span style="margin-left: 6px">{{ SEVERITY_META[s]?.label.slice(3) }}</span>
            </el-option>
          </el-select>
          <el-select
            v-model="form.tags"
            multiple filterable allow-create default-first-option
            collapse-tags collapse-tags-tooltip :max-collapse-tags="3"
            placeholder="标签" style="flex: 1; min-width: 130px"
          >
            <el-option v-for="t in form.tags" :key="t" :value="t" :label="t" />
          </el-select>
        </div>

        <div v-if="form.attachments.length" class="attachments">
          <template v-for="(a, i) in form.attachments" :key="i">
            <el-image
              v-if="a.type === 'image'"
              :src="'/shots/' + a.path"
              :preview-src-list="['/shots/' + a.path]"
              fit="cover"
              class="attach-img"
              preview-teleported
            />
            <div v-else class="attach-text">
              <span class="attach-tag">TXT</span>
              <span class="attach-preview">{{ a.text?.slice(0, 90) }}…</span>
            </div>
          </template>
        </div>

        <div class="section-head">
          <span class="section-label">描述</span>
          <span class="section-hint">Ctrl+S 保存</span>
        </div>
        <RichEditor
          ref="editorRef"
          v-model="form.description"
          placeholder="复现步骤、期望/实际结果…"
          :min-height="120"
          @insert-shot="insertShot"
        />

        <div class="section-head">
          <span class="section-label"><Sparkles :size="12" /> AI 草稿</span>
          <button class="text-btn" :disabled="form.aiStatus === 'pending'" @click="onRetryAI">
            <RefreshCw :size="12" :class="{ spin: form.aiStatus === 'pending' }" />
            {{ form.aiStatus === 'done' ? '重新分析' : '分析' }}
          </button>
          <span class="section-spacer" />
          <button v-if="form.aiStatus === 'done'" class="text-btn accent" @click="applyAI">
            <WandSparkles :size="12" /> 应用到表单
          </button>
        </div>

        <div class="ai-box">
          <template v-if="form.aiStatus === 'pending'">
            <div class="ai-muted"><span class="pulse-dot" /> AI 正在看截图和描述，完成后自动刷新…</div>
          </template>
          <template v-else-if="form.aiStatus === 'failed'">
            <div class="ai-error">分析失败。请检查设置中的 API Key、Base URL 与模型，或稍后重试。</div>
          </template>
          <template v-else-if="form.ai">
            <div class="ai-title">{{ form.ai.title }}</div>
            <div class="ai-meta">
              <span class="ai-sev" :style="{ color: SEVERITY_META[form.ai.severity]?.color }">{{ form.ai.severity }}</span>
              <span>{{ categoryLabel(form.ai.category) }}</span>
              <span v-for="t in form.ai.tags" :key="t" class="ai-tag">{{ t }}</span>
            </div>
            <p class="ai-summary">{{ form.ai.summary }}</p>
            <ol v-if="form.ai.steps?.length" class="ai-steps">
              <li v-for="(s, i) in form.ai.steps" :key="i">{{ s }}</li>
            </ol>
          </template>
          <div v-else class="ai-muted">还没有分析结果。保存后会自动分析，也可以点上方“分析”。</div>
        </div>

        <div class="section-head">
          <span class="section-label">修复指令</span>
          <button class="text-btn" :disabled="fixPromptLoading" @click="onFixPrompt">
            <RefreshCw :size="12" :class="{ spin: fixPromptLoading }" />
            {{ fixPromptHtml ? '重新生成' : '生成' }}
          </button>
          <span class="section-spacer" />
          <button class="text-btn" :disabled="fixPromptLoading" @click="copyFixPrompt">
            <Copy :size="12" /> 复制
          </button>
        </div>
        <div v-if="fixPromptHtml" class="fix-box rf-md" v-html="fixPromptHtml" />
        <div v-else class="fix-empty">
          生成可直接投喂 Claude Code / Cursor 的修复指令，包含定位建议、验收标准与回归风险。
        </div>
      </div>

      <footer class="drawer-foot">
        <button class="btn-danger-ghost" @click="onDelete"><Trash2 :size="14" /> 删除</button>
        <span class="spacer" />
        <button class="btn-ghost" @click="emit('close')">取消</button>
        <button class="btn-outline" @click="persist">保存</button>
      </footer>
    </div>
  </el-drawer>
</template>

<style scoped>
.drawer { height: 100%; display: flex; flex-direction: column; }
.drawer-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: 2px 2px 14px;
  flex-shrink: 0;
}
.drawer-id { display: flex; flex-direction: column; gap: 5px; }
.eyebrow {
  font-family: var(--rf-mono);
  font-size: 9.5px;
  letter-spacing: 0.14em;
  color: var(--rf-accent);
}
.drawer-created { font-size: 11px; color: var(--rf-text-faint); }
.icon-btn {
  width: 30px;
  height: 30px;
  border-radius: 8px;
  border: 1px solid transparent;
  background: transparent;
  color: var(--rf-text-dim);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
}
.icon-btn:hover { background: var(--rf-card); border-color: var(--rf-border); color: var(--rf-text); }
.drawer-scroll { flex: 1; min-height: 0; overflow-y: auto; display: flex; flex-direction: column; gap: 13px; padding-right: 4px; }
/* 抽屉是 flex 纵向容器，防止编辑器被压扁到不可见 */
.drawer-scroll > :deep(.rf-editor) { flex-shrink: 0; }
.title-input {
  width: 100%;
  background: transparent;
  border: none;
  border-bottom: 1px solid var(--rf-border);
  outline: none;
  color: var(--rf-text);
  font-size: 20px;
  font-weight: 700;
  letter-spacing: -0.01em;
  padding: 2px 0 12px;
}
.title-input:focus { border-bottom-color: var(--rf-accent); }
.title-input::placeholder { color: var(--rf-text-faint); font-weight: 500; }
.meta-row { display: flex; gap: 8px; flex-wrap: wrap; }
.attachments { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
.attach-img { width: 132px; height: 82px; border-radius: 9px; border: 1px solid var(--rf-border); }
.attach-text {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 11.5px;
  color: var(--rf-text-dim);
  background: var(--rf-panel);
  border: 1px solid var(--rf-border);
  border-radius: 8px;
  padding: 7px 10px;
  max-width: 100%;
}
.attach-tag { font-family: var(--rf-mono); font-size: 8.5px; color: var(--rf-accent); border: 1px solid rgba(168,99,19,.3); border-radius: 3px; padding: 1px 4px; }
.attach-preview { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.section-head { display: flex; align-items: center; gap: 7px; margin-top: 3px; }
.section-label {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 10.5px;
  font-weight: 700;
  letter-spacing: 0.08em;
  color: var(--rf-text-faint);
  text-transform: uppercase;
}
.section-hint { font-size: 10px; color: var(--rf-text-faint); }
.section-spacer { flex: 1; }
.text-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: none;
  background: transparent;
  color: var(--rf-text-dim);
  font-size: 11px;
  cursor: pointer;
  padding: 3px 6px;
  border-radius: 5px;
}
.text-btn:hover:not(:disabled) { color: var(--rf-text); background: var(--rf-card); }
.text-btn.accent { color: var(--rf-accent); }
.text-btn:disabled { color: var(--rf-text-faint); cursor: default; }
.spin { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
.ai-box {
  background: linear-gradient(180deg, rgba(168, 99, 19, .055), rgba(168, 99, 19, .02));
  border: 1px solid rgba(168, 99, 19, .2);
  border-radius: 11px;
  padding: 12px 14px;
  font-size: 12.5px;
}
.ai-muted { color: var(--rf-text-dim); display: flex; align-items: center; gap: 7px; font-size: 12px; line-height: 1.6; }
.pulse-dot { width: 6px; height: 6px; border-radius: 50%; background: var(--rf-accent); animation: pulse 1.2s ease-in-out infinite; }
@keyframes pulse { 50% { opacity: .25; } }
.ai-error { color: var(--rf-danger); font-size: 12px; line-height: 1.6; }
.ai-title { font-weight: 700; font-size: 13.5px; margin-bottom: 5px; }
.ai-meta { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; font-size: 10.5px; color: var(--rf-text-dim); margin-bottom: 7px; }
.ai-sev { font-weight: 800; font-family: var(--rf-mono); }
.ai-tag { color: var(--rf-accent); background: var(--rf-accent-soft); border-radius: 3px; padding: 1px 5px; }
.ai-summary { margin: 0; line-height: 1.65; color: var(--rf-text); }
.ai-steps { margin: 8px 0 0; padding-left: 18px; line-height: 1.75; color: var(--rf-text-dim); font-size: 12px; }
.fix-box {
  background: var(--rf-well);
  border: 1px solid var(--rf-border);
  border-radius: 10px;
  padding: 12px 14px;
  max-height: 300px;
  overflow: auto;
}
.fix-empty { font-size: 11.5px; line-height: 1.7; color: var(--rf-text-faint); border: 1px dashed var(--rf-border); border-radius: 9px; padding: 11px 13px; }
.drawer-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-top: 13px;
  border-top: 1px solid var(--rf-border);
  flex-shrink: 0;
}
.spacer { flex: 1; }
.btn-ghost, .btn-outline, .btn-danger-ghost {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  border-radius: 8px;
  font-size: 12.5px;
  font-weight: 600;
  padding: 7px 14px;
  cursor: pointer;
  transition: background .14s ease, border-color .14s ease, color .14s ease;
}
.btn-ghost { color: var(--rf-text-dim); background: transparent; border: 1px solid transparent; }
.btn-ghost:hover { color: var(--rf-text); background: var(--rf-card); }
.btn-outline { color: var(--rf-accent); background: transparent; border: 1px solid rgba(168, 99, 19, .48); }
.btn-outline:hover { background: var(--rf-accent-soft); border-color: var(--rf-accent); }
.btn-danger-ghost { color: var(--rf-text-faint); background: transparent; border: 1px solid transparent; }
.btn-danger-ghost:hover { color: var(--rf-danger); background: rgba(180, 63, 54, .07); }
</style>

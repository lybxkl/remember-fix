<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { ClipboardCopy, ClipboardPaste, Sparkles, Wrench, X } from 'lucide-vue-next'
import RichEditor from '../components/RichEditor.vue'
import KindSwitch from '../components/KindSwitch.vue'
import DueField from '../components/DueField.vue'
import { useCapture } from '../composables/useCapture'
import { call } from '../lib/wails'
import { duePreset, formatTime } from '../lib/constants'
import type { IntentResult, Kind, KindSource } from '../lib/types'
import { altName, defaultHotkey, modSymbol, screenshotKeys } from '../lib/platform'

const { items, selected, selectedId, readyTick, select, sync, save, cancel } = useCapture()

const title = ref('')
const description = ref('')
const descriptionText = ref('')
const saving = ref(false)
const hotkey = ref(defaultHotkey)
const titleInput = ref<HTMLInputElement>()
const editorRef = ref<InstanceType<typeof RichEditor>>()

const kind = ref<Kind>('bug')
const kindSource = ref<KindSource>('heuristic')
const intent = ref<IntentResult | null>(null)
const manualKind = ref(false)
const dueAt = ref(0)
const dueAllDay = ref(true)
let detectTimer: number | undefined
let detectSeq = 0

const isImage = computed(() => selected.value?.type === 'image')
const shotURL = computed(() => (selected.value?.path ? '/shots/' + selected.value.path : ''))
const selectedIndex = computed(() => items.value.findIndex((x) => x.id === selectedId.value))
const hasSomething = computed(
  () => !!selected.value || !!title.value.trim() || !!descriptionText.value.trim(),
)
const isTodo = computed(() => kind.value === 'todo')
const kindHint = computed(() => {
  if (manualKind.value) return '手动选择'
  if (!intent.value) return '正在判断…'
  return intent.value.source === 'fallback' ? '沿用上次类型' : `自动判断 ${Math.round(intent.value.confidence * 100)}%`
})
const titlePlaceholder = computed(() =>
  isTodo.value ? '要做什么、什么时候（留空由 AI 代写）' : '哪里、出了什么问题（留空由 AI 代写）',
)
const editorPlaceholder = computed(() =>
  isTodo.value ? '要办什么、有没有时间限制、需要带什么…' : '做了什么、期望什么、实际怎样…',
)
const aiHint = computed(() =>
  isTodo.value
    ? '保存后 AI 会补全标题、优先级和到期时间（仅在你明确提及时）'
    : '保存后 AI 会自动补全标题、分类与标签',
)

onMounted(async () => {
  await sync()
  await loadHotkey()
  focusPrimary()
  runDetect()
  window.addEventListener('keydown', onKeydown)
})
onBeforeUnmount(() => {
  detectSeq++
  if (detectTimer) window.clearTimeout(detectTimer)
  window.removeEventListener('keydown', onKeydown)
})

watch(readyTick, async () => {
  detectSeq++
  if (detectTimer) {
    window.clearTimeout(detectTimer)
    detectTimer = undefined
  }
  title.value = ''
  description.value = ''
  descriptionText.value = ''
  dueAt.value = 0
  dueAllDay.value = true
  manualKind.value = false
  kindSource.value = 'heuristic'
  intent.value = null
  await loadHotkey()
  await nextTick()
  focusPrimary()
  runDetect()
})

watch(description, (v) => {
  descriptionText.value = htmlToText(v)
})

watch([title, descriptionText, () => selected.value?.id], () => scheduleDetect())

function focusPrimary() {
  setTimeout(() => titleInput.value?.focus(), 90)
}

async function loadHotkey() {
  try {
    const cfg = await call<Record<string, string>>('main.ConfigService.GetAll')
    if (cfg?.hotkey) hotkey.value = cfg.hotkey
    // 首次输入前先沿用上次类型；后续会由本地判断覆盖。
    if (!manualKind.value && !intent.value && cfg?.['capture.lastKind'] === 'todo') {
      kind.value = 'todo'
    }
  } catch {
    /* 保持默认显示 */
  }
}

function scheduleDetect() {
  if (manualKind.value) return
  if (detectTimer) window.clearTimeout(detectTimer)
  detectTimer = window.setTimeout(runDetect, 220)
}

async function runDetect(): Promise<IntentResult | null> {
  if (manualKind.value) return null
  const seq = ++detectSeq
  try {
    const res = await call<IntentResult>('main.BugService.DetectIntent', {
      title: title.value.trim(),
      body: descriptionText.value.trim(),
      sourceApp: selected.value?.sourceApp || '',
      hasImage: selected.value?.type === 'image',
    })
    if (seq !== detectSeq || manualKind.value || !res) return null
    intent.value = res
    kind.value = res.kind
    kindSource.value = 'heuristic'
    return res
  } catch {
    /* 判断失败时保持当前类型 */
    return null
  }
}

function setKind(value: Kind) {
  manualKind.value = true
  kind.value = value
  kindSource.value = 'user'
}

function setDuePreset(preset: 'today' | 'tomorrow' | 'weekend') {
  if (!isTodo.value) return
  dueAt.value = duePreset(preset)
  dueAllDay.value = true
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
    e.preventDefault()
    onSave()
  } else if (e.key === 'ArrowLeft' && e.altKey) {
    e.preventDefault()
    step(-1)
  } else if (e.key === 'ArrowRight' && e.altKey) {
    e.preventDefault()
    step(1)
  } else if (e.altKey && e.key === '1') {
    e.preventDefault()
    setKind('bug')
  } else if (e.altKey && e.key === '2') {
    e.preventDefault()
    setKind('todo')
  } else if (e.altKey && (e.key === 't' || e.key === 'T')) {
    e.preventDefault()
    setDuePreset('today')
  } else if (e.altKey && (e.key === 'n' || e.key === 'N')) {
    e.preventDefault()
    setDuePreset('tomorrow')
  }
}

function step(delta: number) {
  if (items.value.length < 2) return
  const next = (selectedIndex.value + delta + items.value.length) % items.value.length
  select(items.value[next].id)
}

function insertShot() {
  if (shotURL.value) editorRef.value?.insertImage(shotURL.value)
}

async function copyText() {
  if (!selected.value?.text) return
  const ok = await call<boolean>('main.ConfigService.CopyText', selected.value.text)
  if (ok) ElMessage.success('已复制')
  else ElMessage.error('复制失败')
}

async function onSave() {
  if (saving.value) return
  if (!hasSomething.value) {
    ElMessage.warning('剪贴板为空，写一句话或直接描述也可以记录')
    return
  }
  if (!selected.value && !title.value.trim() && !descriptionText.value.trim()) {
    ElMessage.warning('写一句描述吧（标题留空会由 AI 生成）')
    return
  }
  saving.value = true
  try {
    // 保证最后一段输入也参与类型判断；手动选择的类型不会被覆盖。
    if (!manualKind.value) await runDetect()
    await save({
      title: title.value.trim(),
      description: description.value,
      descriptionText: descriptionText.value,
      kind: kind.value,
      kindSource: manualKind.value ? 'user' : kindSource.value,
      dueAt: dueAt.value,
      dueAllDay: dueAllDay.value,
    })
    title.value = ''
    description.value = ''
    descriptionText.value = ''
  } catch (e) {
    ElMessage.error('保存失败：' + (e instanceof Error ? e.message : String(e)))
  } finally {
    saving.value = false
  }
}

function htmlToText(html: string): string {
  if (!html) return ''
  // DOMParser 文档是惰性的：不执行脚本、不加载资源，只取纯文本。
  return (new DOMParser().parseFromString(html, 'text/html').body.textContent ?? '').trim()
}
</script>

<template>
  <div class="rf-capture">
    <header class="cap-head">
      <div class="cap-brand">
        <span class="brand-mark"><Wrench :size="15" :stroke-width="2.2" /></span>
        <div class="cap-brand-text">
          <span class="cap-eyebrow">REMEMBER / EVERYTHING</span>
          <span class="cap-title">记一条</span>
        </div>
      </div>
      <span class="cap-hotkey" :title="`全局热键 ${hotkey}`">{{ hotkey }}</span>
      <button class="icon-btn" title="关闭 Esc" @click="cancel">
        <X :size="16" />
      </button>
    </header>

    <div class="cap-body">
      <section class="cap-source">
        <div class="source-line">
          <span class="source-dot" :class="{ live: !!selected }" />
          <span v-if="selected" class="source-label">
            {{ isImage ? '截图已就位' : '剪贴板文本已就位' }}
          </span>
          <span v-else class="source-label">剪贴板为空</span>
          <span v-if="selected?.sourceApp" class="source-app" :title="selected.sourceApp">
            {{ selected.sourceApp }}
          </span>
          <span class="spacer" />
          <span v-if="items.length" class="source-index">{{ selectedIndex + 1 }} / {{ items.length }}</span>
        </div>

        <div v-if="selected" class="cap-preview">
          <el-image
            v-if="isImage"
            :src="shotURL"
            :preview-src-list="[shotURL]"
            fit="contain"
            class="cap-shot"
            preview-teleported
          />
          <div v-else class="cap-text">
            <div class="cap-text-body">{{ selected.text }}</div>
            <button class="mini-action" @click.stop="copyText">
              <ClipboardCopy :size="12" /> 复制
            </button>
          </div>
          <button v-if="isImage" class="preview-insert" title="把截图插入描述" @click="insertShot">
            + 插入描述
          </button>
          <span class="preview-time">{{ formatTime(selected.at) }}</span>
        </div>
        <div v-else class="cap-empty">
          <ClipboardPaste :size="18" />
          <div>
            <div class="empty-title">没有抓到剪贴板内容</div>
            <div class="empty-sub">
              先 <template v-for="(key, i) in screenshotKeys" :key="key"><span v-if="i > 0">+</span><kbd>{{ key }}</kbd></template> 截屏到剪贴板，或复制一段文字；也可以直接在下面记录
            </div>
          </div>
        </div>

        <div v-if="items.length > 1" class="cap-history">
          <button
            v-for="(it, i) in items"
            :key="it.id"
            class="hist-item"
            :class="{ active: it.id === selectedId }"
            :title="`${it.type === 'image' ? '截图' : '文本'} · ${formatTime(it.at)}`"
            @click="select(it.id)"
          >
            <img v-if="it.type === 'image'" :src="'/shots/' + it.path" alt="" />
            <span v-else class="hist-text">TXT</span>
            <span class="hist-index">{{ i + 1 }}</span>
          </button>
          <span class="hist-hint">{{ altName }} ←/→ 切换</span>
        </div>
      </section>

      <section class="cap-form">
        <div class="cap-kind-row">
          <span class="field-label">类型</span>
          <KindSwitch
            :model-value="kind"
            compact
            :hint="kindHint"
            :hint-title="intent?.reason"
            @update:model-value="setKind"
          />
        </div>
        <div v-if="isTodo" class="cap-due-row">
          <DueField v-model="dueAt" v-model:all-day="dueAllDay" compact />
        </div>

        <label class="field-label" for="cap-title">一句话</label>
        <input
          id="cap-title"
          ref="titleInput"
          v-model="title"
          class="cap-title-input"
          :placeholder="titlePlaceholder"
        />

        <div class="field-row">
          <label class="field-label">细节</label>
          <span class="ai-hint"><Sparkles :size="11" /> {{ aiHint }}</span>
        </div>
        <div class="cap-editor">
          <RichEditor
            ref="editorRef"
            v-model="description"
            :placeholder="editorPlaceholder"
            :min-height="92"
            @insert-shot="insertShot"
          />
        </div>
      </section>
    </div>

    <footer class="cap-foot">
      <span class="cap-shortcuts"><kbd>Esc</kbd> 关闭 <span class="dot-sep">·</span> <kbd>{{ modSymbol }}</kbd><kbd>↵</kbd> 保存 <span class="dot-sep">·</span> <kbd>Alt</kbd>+<kbd>1/2</kbd> 切类型</span>
      <span class="spacer" />
      <button class="btn-ghost" @click="cancel">放弃</button>
      <button class="btn-commit" :disabled="saving || !hasSomething" @click="onSave">
        {{ saving ? '保存中…' : '记下来' }}
      </button>
    </footer>
  </div>
</template>

<style scoped>
.rf-capture {
  height: 100vh;
  display: flex;
  flex-direction: column;
  background:
    radial-gradient(120% 70% at 50% -10%, rgba(168, 99, 19, 0.06), transparent 55%),
    var(--rf-bg);
  overflow: hidden;
}
.cap-head {
  --wails-draggable: drag;
  display: flex;
  align-items: center;
  gap: 10px;
  height: 54px;
  padding: 0 14px;
  border-bottom: 1px solid var(--rf-border);
  flex-shrink: 0;
}
.cap-head button { --wails-draggable: no-drag; }
.cap-brand { display: flex; align-items: center; gap: 9px; }
.brand-mark {
  width: 28px;
  height: 28px;
  border-radius: 8px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--rf-accent);
  background: var(--rf-accent-soft);
  border: 1px solid rgba(168, 99, 19, 0.28);
}
.cap-brand-text { display: flex; flex-direction: column; line-height: 1.1; }
.cap-eyebrow {
  font-family: var(--rf-mono);
  font-size: 8.5px;
  letter-spacing: 0.14em;
  color: var(--rf-text-faint);
}
.cap-title { font-size: 14px; font-weight: 700; letter-spacing: 0.02em; }
.cap-hotkey {
  margin-left: auto;
  font-family: var(--rf-mono);
  font-size: 10.5px;
  color: var(--rf-text-faint);
  border: 1px solid var(--rf-border);
  border-radius: 5px;
  padding: 2px 6px;
}
.icon-btn {
  width: 28px;
  height: 28px;
  border-radius: 7px;
  border: 1px solid transparent;
  background: transparent;
  color: var(--rf-text-dim);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
}
.icon-btn:hover { color: var(--rf-text); background: var(--rf-card); border-color: var(--rf-border); }

.cap-body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 12px 14px 0;
}
.cap-source { display: flex; flex-direction: column; gap: 8px; flex-shrink: 0; }
.source-line { display: flex; align-items: center; gap: 7px; font-size: 11.5px; }
.source-dot { width: 6px; height: 6px; border-radius: 50%; background: var(--rf-text-faint); }
.source-dot.live { background: var(--rf-ok); box-shadow: 0 0 0 3px rgba(45, 122, 82, 0.1); }
.source-label { color: var(--rf-text-dim); font-weight: 600; letter-spacing: 0.02em; }
.source-app {
  font-size: 10.5px;
  color: var(--rf-text-faint);
  background: var(--rf-panel);
  border: 1px solid var(--rf-border);
  padding: 1px 6px;
  border-radius: 4px;
  max-width: 210px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.source-index { font-family: var(--rf-mono); font-size: 10px; color: var(--rf-text-faint); }
.spacer { flex: 1; }

.cap-preview {
  position: relative;
  height: 178px;
  border: 1px solid var(--rf-border);
  border-radius: 11px;
  background:
    linear-gradient(45deg, rgba(38,34,27,0.022) 25%, transparent 25%, transparent 75%, rgba(38,34,27,0.022) 75%),
    var(--rf-well);
  background-size: 14px 14px;
  overflow: hidden;
}
.cap-shot { width: 100%; height: 100%; }
.cap-text { height: 100%; display: flex; flex-direction: column; padding: 28px 13px 12px; gap: 8px; }
.cap-text-body {
  flex: 1;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: var(--rf-mono);
  font-size: 12px;
  line-height: 1.65;
  color: var(--rf-text-dim);
}
.mini-action {
  align-self: flex-start;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  color: var(--rf-text-dim);
  background: var(--rf-card);
  border: 1px solid var(--rf-border);
  border-radius: 6px;
  padding: 3px 8px;
  cursor: pointer;
}
.mini-action:hover { color: var(--rf-accent); border-color: rgba(168, 99, 19, 0.34); }
.preview-insert {
  position: absolute;
  right: 9px;
  bottom: 9px;
  font-size: 10.5px;
  color: var(--rf-text);
  background: rgba(255, 253, 248, 0.9);
  border: 1px solid var(--rf-border-strong);
  border-radius: 6px;
  padding: 3px 8px;
  cursor: pointer;
  -webkit-backdrop-filter: blur(6px);
  backdrop-filter: blur(6px);
}
.preview-insert:hover { border-color: var(--rf-accent); color: var(--rf-accent); }
.preview-time {
  position: absolute;
  left: 9px;
  top: 8px;
  font-family: var(--rf-mono);
  font-size: 9.5px;
  color: var(--rf-text-faint);
  background: rgba(255, 253, 248, 0.86);
  border-radius: 4px;
  padding: 1px 5px;
}
.cap-empty {
  height: 92px;
  border: 1px dashed var(--rf-border-strong);
  border-radius: 11px;
  background: var(--rf-panel);
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 16px;
  color: var(--rf-text-faint);
  flex-shrink: 0;
}
.empty-title { font-size: 12.5px; color: var(--rf-text-dim); font-weight: 600; margin-bottom: 3px; }
.empty-sub { font-size: 11px; line-height: 1.7; }

.cap-history { display: flex; align-items: center; gap: 6px; overflow-x: auto; padding-bottom: 1px; }
.hist-item {
  position: relative;
  width: 46px;
  height: 31px;
  flex-shrink: 0;
  padding: 0;
  border-radius: 6px;
  border: 1px solid var(--rf-border);
  background: var(--rf-well);
  overflow: hidden;
  cursor: pointer;
  color: var(--rf-text-faint);
}
.hist-item.active { border-color: var(--rf-accent); box-shadow: 0 0 0 1px rgba(168, 99, 19, 0.2); }
.hist-item img { width: 100%; height: 100%; object-fit: cover; display: block; }
.hist-text { font-family: var(--rf-mono); font-size: 8.5px; letter-spacing: 0.1em; }
.hist-index {
  position: absolute;
  right: 2px;
  bottom: 0;
  font-size: 8px;
  font-family: var(--rf-mono);
  background: rgba(255, 253, 248, 0.88);
  color: var(--rf-text);
  border-radius: 3px 0 0 0;
  padding: 0 3px;
}
.hist-hint { font-size: 10px; color: var(--rf-text-faint); white-space: nowrap; margin-left: 2px; }

.cap-form { flex: 1; min-height: 0; display: flex; flex-direction: column; gap: 7px; }
.cap-kind-row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 26px;
  flex-shrink: 0;
}
.cap-kind-row .field-label { flex-shrink: 0; }
.cap-due-row { flex-shrink: 0; margin: -1px 0 1px; }
.field-row { display: flex; align-items: center; gap: 8px; }
.field-label {
  font-size: 10.5px;
  font-weight: 700;
  letter-spacing: 0.09em;
  color: var(--rf-text-faint);
  text-transform: uppercase;
}
.ai-hint {
  margin-left: auto;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 10px;
  color: var(--rf-text-faint);
}
.cap-title-input {
  width: 100%;
  flex-shrink: 0;
  background: var(--rf-panel);
  border: 1px solid var(--rf-border);
  border-radius: 9px;
  outline: none;
  color: var(--rf-text);
  font-size: 14.5px;
  font-weight: 600;
  padding: 9px 12px;
  transition: border-color .15s ease, background .15s ease;
}
.cap-title-input::placeholder { color: var(--rf-text-faint); font-weight: 400; }
.cap-title-input:focus { border-color: var(--rf-accent); background: var(--rf-card); }
.cap-editor { flex: 1; min-height: 0; display: flex; }
.cap-editor > :deep(.rf-editor) { flex: 1; }

.cap-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 11px 14px 13px;
  flex-shrink: 0;
}
.cap-shortcuts { font-size: 10px; color: var(--rf-text-faint); display: flex; align-items: center; gap: 3px; }
.dot-sep { margin: 0 3px; opacity: .6; }
kbd {
  display: inline-block;
  background: var(--rf-panel);
  border: 1px solid var(--rf-border);
  border-bottom-color: var(--rf-border-strong);
  border-radius: 4px;
  padding: 0 4px;
  font-family: var(--rf-mono);
  font-size: 9.5px;
  line-height: 16px;
  color: var(--rf-text-dim);
}
.btn-ghost, .btn-commit {
  border-radius: 8px;
  font-size: 12.5px;
  font-weight: 600;
  padding: 7px 14px;
  cursor: pointer;
  transition: background .14s ease, border-color .14s ease, color .14s ease;
}
.btn-ghost { color: var(--rf-text-dim); background: transparent; border: 1px solid transparent; }
.btn-ghost:hover { color: var(--rf-text); background: var(--rf-card); }
.btn-commit { color: var(--rf-accent); background: transparent; border: 1px solid rgba(168, 99, 19, 0.48); }
.btn-commit:hover:not(:disabled) { background: var(--rf-accent-soft); border-color: var(--rf-accent); }
.btn-commit:disabled { color: var(--rf-text-faint); border-color: var(--rf-border); cursor: default; }
</style>

<script setup lang="ts">
import { computed } from 'vue'
import { AlertCircle, Image as ImageIcon, Loader2, Sparkles } from 'lucide-vue-next'
import type { Bug as BugType } from '../lib/types'
import { categoryLabel, firstLine, relativeTime, SEVERITY_META } from '../lib/constants'

const props = defineProps<{ bug: BugType }>()
const emit = defineEmits<{ (e: 'open', id: string): void }>()

const sev = computed(() => SEVERITY_META[props.bug.severity] ?? SEVERITY_META.P2)
const thumb = computed(() => props.bug.attachments.find((a) => a.type === 'image' && a.path)?.path ?? '')
const imageCount = computed(() => props.bug.attachments.filter((a) => a.type === 'image').length)
const titleText = computed(
  () => props.bug.title || firstLine(props.bug.descriptionText, 48) || '未命名 Bug',
)
const descPreview = computed(() => {
  const desc = firstLine(props.bug.descriptionText, 70)
  return desc && desc !== titleText.value ? desc : ''
})
const aiTitle = computed(
  () => ({
    pending: 'AI 正在分析',
    done: 'AI 已分析',
    failed: 'AI 分析失败，打开详情可重试',
    none: '',
  })[props.bug.aiStatus] ?? '',
)

function open() {
  emit('open', props.bug.id)
}

function onDragStart(e: DragEvent) {
  e.dataTransfer?.setData('text/rf-bug-id', props.bug.id)
  e.dataTransfer?.setData('text/plain', props.bug.id)
  if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move'
}
</script>

<template>
  <article
    class="bug-card"
    role="button"
    tabindex="0"
    draggable="true"
    :style="{ '--sev': sev.color }"
    :aria-label="`打开 ${titleText}`"
    @click="open"
    @keydown.enter.prevent="open"
    @keydown.space.prevent="open"
    @dragstart="onDragStart"
  >
    <div class="card-top">
      <span class="sev">{{ bug.severity }}</span>
      <span class="cat">{{ categoryLabel(bug.category) }}</span>
      <span class="spacer" />
      <span v-if="bug.aiStatus === 'pending'" class="ai pending" :title="aiTitle">
        <Loader2 :size="12" />
      </span>
      <span v-else-if="bug.aiStatus === 'done'" class="ai done" :title="aiTitle">
        <Sparkles :size="12" />
      </span>
      <span v-else-if="bug.aiStatus === 'failed'" class="ai failed" :title="aiTitle">
        <AlertCircle :size="12" />
      </span>
    </div>

    <h3 class="card-title">{{ titleText }}</h3>
    <p v-if="descPreview" class="card-desc">{{ descPreview }}</p>

    <div v-if="thumb" class="card-thumb">
      <img :src="'/shots/' + thumb" loading="lazy" alt="bug 截图" />
      <span v-if="imageCount > 1" class="thumb-count"><ImageIcon :size="10" />{{ imageCount }}</span>
    </div>

    <div v-if="bug.tags?.length" class="card-tags">
      <span v-for="t in bug.tags.slice(0, 4)" :key="t" class="tag">{{ t }}</span>
      <span v-if="bug.tags.length > 4" class="tag more">+{{ bug.tags.length - 4 }}</span>
    </div>

    <footer class="card-foot">
      <span class="time">{{ relativeTime(bug.createdAt) }}</span>
      <span v-if="bug.sourceApp" class="source" :title="bug.sourceApp">{{ bug.sourceApp }}</span>
    </footer>
  </article>
</template>

<style scoped>
.bug-card {
  position: relative;
  background: var(--rf-card);
  border: 1px solid var(--rf-border);
  border-radius: 10px;
  padding: 10px 12px 9px 14px;
  cursor: grab;
  display: flex;
  flex-direction: column;
  gap: 6px;
  user-select: none;
  transition: border-color .15s ease, background .15s ease, transform .12s ease, box-shadow .15s ease;
  animation: card-in .24s ease both;
}
.bug-card::before {
  content: '';
  position: absolute;
  left: 5px;
  top: 12px;
  bottom: 12px;
  width: 2px;
  border-radius: 2px;
  background: var(--sev);
  opacity: .55;
}
.bug-card:hover {
  border-color: var(--rf-border-strong);
  background: var(--rf-card-hover);
  transform: translateY(-1px);
  box-shadow: 0 6px 18px rgba(76, 62, 43, 0.1);
}
.bug-card:active { cursor: grabbing; }
.bug-card:focus-visible { border-color: var(--rf-accent); outline: none; }
@keyframes card-in {
  from { opacity: 0; transform: translateY(4px); }
  to { opacity: 1; transform: translateY(0); }
}
.card-top { display: flex; align-items: center; gap: 7px; min-height: 16px; }
.sev {
  font-family: var(--rf-mono);
  font-size: 9.5px;
  font-weight: 700;
  color: var(--sev);
  border: 1px solid color-mix(in srgb, var(--sev) 45%, transparent);
  border-radius: 4px;
  padding: 0 4px;
  line-height: 15px;
}
.cat { font-size: 10px; color: var(--rf-text-faint); }
.spacer { flex: 1; }
.ai { display: inline-flex; align-items: center; }
.ai.pending { color: var(--rf-accent); }
.ai.pending svg { animation: spin 1s linear infinite; }
.ai.done { color: var(--rf-accent); opacity: .8; }
.ai.failed { color: var(--rf-danger); }
@keyframes spin { to { transform: rotate(360deg); } }
.card-title {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
  line-height: 1.42;
  letter-spacing: -0.005em;
  color: var(--rf-text);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.card-desc {
  margin: 0;
  font-size: 11.5px;
  line-height: 1.5;
  color: var(--rf-text-dim);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.card-thumb {
  position: relative;
  margin-top: 1px;
  border-radius: 6px;
  overflow: hidden;
  border: 1px solid var(--rf-border);
  background: var(--rf-well);
}
.card-thumb img {
  display: block;
  width: 100%;
  height: 92px;
  object-fit: cover;
  transition: transform .25s ease;
}
.bug-card:hover .card-thumb img { transform: scale(1.015); }
.thumb-count {
  position: absolute;
  right: 5px;
  bottom: 5px;
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-family: var(--rf-mono);
  font-size: 9px;
  color: #fffdf8;
  background: rgba(58, 49, 38, .66);
  border-radius: 4px;
  padding: 1px 5px;
  backdrop-filter: blur(4px);
}
.card-tags { display: flex; gap: 4px; flex-wrap: wrap; }
.tag {
  font-size: 9.5px;
  color: var(--rf-accent);
  background: var(--rf-accent-soft);
  border: 1px solid rgba(168, 99, 19, 0.16);
  border-radius: 3px;
  padding: 0 5px;
  line-height: 16px;
}
.tag.more { color: var(--rf-text-faint); background: transparent; border-color: var(--rf-border); }
.card-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 1px;
  font-size: 9.5px;
  color: var(--rf-text-faint);
}
.time { flex-shrink: 0; }
.source {
  margin-left: auto;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 130px;
  opacity: .8;
}
</style>

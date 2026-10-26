<script setup lang="ts">
import { computed, ref } from 'vue'
import BugCard from './BugCard.vue'
import type { Bug } from '../lib/types'

const props = defineProps<{
  status: string
  label: string
  code: string
  color: string
  bugs: Bug[]
}>()

const emit = defineEmits<{
  (e: 'open', id: string): void
  (e: 'drop', bugId: string, status: string): void
}>()

const dragDepth = ref(0)
const count = computed(() => props.bugs.length)
const over = computed(() => dragDepth.value > 0)

function hasBug(e: DragEvent) {
  return !!e.dataTransfer && Array.from(e.dataTransfer.types).includes('text/rf-bug-id')
}

function onDragEnter(e: DragEvent) {
  if (!hasBug(e)) return
  e.preventDefault()
  dragDepth.value++
}

function onDragOver(e: DragEvent) {
  if (!hasBug(e)) return
  e.preventDefault()
  e.dataTransfer!.dropEffect = 'move'
}

function onDragLeave() {
  dragDepth.value = Math.max(0, dragDepth.value - 1)
}

function onDrop(e: DragEvent) {
  dragDepth.value = 0
  const id = e.dataTransfer?.getData('text/rf-bug-id') || e.dataTransfer?.getData('text/plain')
  if (id) emit('drop', id, props.status)
}
</script>

<template>
  <section
    class="column"
    :class="{ over }"
    :style="{ '--col': color }"
    @dragenter="onDragEnter"
    @dragover="onDragOver"
    @dragleave="onDragLeave"
    @drop="onDrop"
  >
    <header class="col-head">
      <span class="col-dot" />
      <span class="col-label">{{ label }}</span>
      <span class="col-code">{{ code }}</span>
      <span class="col-count">{{ count }}</span>
    </header>
    <div class="col-body">
      <BugCard v-for="b in bugs" :key="b.id" :bug="b" @open="emit('open', $event)" />
      <div v-if="!bugs.length" class="col-empty">
        <span class="empty-line" />
        <span>拖到这里</span>
        <span class="empty-line" />
      </div>
    </div>
  </section>
</template>

<style scoped>
.column {
  flex: 1 0 218px;
  min-width: 218px;
  display: flex;
  flex-direction: column;
  border-radius: 12px;
  background: var(--rf-bg-soft);
  border: 1px solid transparent;
  overflow: hidden;
  transition: border-color .15s ease, background .15s ease;
  position: relative;
}
.column::before {
  content: '';
  position: absolute;
  top: 0;
  left: 14px;
  right: 14px;
  height: 2px;
  border-radius: 0 0 2px 2px;
  background: var(--col);
  opacity: .7;
}
.column.over {
  border-color: color-mix(in srgb, var(--col) 55%, transparent);
  background: color-mix(in srgb, var(--col) 5%, var(--rf-bg-soft));
}
.col-head {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 13px 13px 9px;
  flex-shrink: 0;
}
.col-dot { width: 6px; height: 6px; border-radius: 50%; background: var(--col); }
.col-label { font-size: 12px; font-weight: 700; letter-spacing: .01em; }
.col-code {
  font-family: var(--rf-mono);
  font-size: 8.5px;
  letter-spacing: .1em;
  color: var(--rf-text-faint);
  margin-top: 1px;
}
.col-count {
  margin-left: auto;
  font-family: var(--rf-mono);
  font-size: 10px;
  color: var(--rf-text-dim);
  background: var(--rf-panel);
  border: 1px solid var(--rf-border);
  border-radius: 10px;
  padding: 0 7px;
  line-height: 18px;
  min-width: 24px;
  text-align: center;
}
.col-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 1px 9px 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.col-empty {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 16px 6px;
  color: var(--rf-text-faint);
  font-size: 10.5px;
  opacity: .7;
}
.empty-line { flex: 1; height: 1px; background: var(--rf-border); }
</style>

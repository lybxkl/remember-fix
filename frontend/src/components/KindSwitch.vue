<script setup lang="ts">
import { Bug, ListTodo } from 'lucide-vue-next'
import type { Kind } from '../lib/types'
import { KIND_META } from '../lib/constants'

const props = defineProps<{
  modelValue: Kind
  hint?: string
  hintTitle?: string
  compact?: boolean
}>()
const emit = defineEmits<{ (e: 'update:modelValue', value: Kind): void }>()

const options: Kind[] = ['bug', 'todo']

function choose(kind: Kind) {
  if (kind !== props.modelValue) emit('update:modelValue', kind)
}
</script>

<template>
  <div class="kind-switch" :class="{ compact }" role="group" aria-label="事项类型">
    <button
      v-for="kind in options"
      :key="kind"
      type="button"
      class="kind-option"
      :class="{ active: modelValue === kind }"
      :style="{ '--kind': KIND_META[kind].color, '--kind-soft': KIND_META[kind].soft }"
      :aria-pressed="modelValue === kind"
      @click="choose(kind)"
    >
      <Bug v-if="kind === 'bug'" :size="compact ? 11 : 13" :stroke-width="2.2" />
      <ListTodo v-else :size="compact ? 11 : 13" :stroke-width="2.2" />
      <span>{{ KIND_META[kind].label }}</span>
      <kbd v-if="!compact" class="kind-key">Alt+{{ kind === 'bug' ? 1 : 2 }}</kbd>
    </button>
    <span v-if="hint" class="kind-hint" :title="hintTitle">{{ hint }}</span>
  </div>
</template>

<style scoped>
.kind-switch {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  min-width: 0;
}
.kind-option {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  border: 1px solid var(--rf-border);
  background: var(--rf-panel);
  color: var(--rf-text-faint);
  border-radius: 7px;
  padding: 5px 9px;
  font-size: 11.5px;
  font-weight: 700;
  line-height: 1;
  cursor: pointer;
  transition: color .14s ease, border-color .14s ease, background .14s ease;
}
.kind-option:hover { color: var(--rf-text-dim); border-color: var(--rf-border-strong); }
.kind-option.active {
  color: var(--kind);
  border-color: color-mix(in srgb, var(--kind) 55%, transparent);
  background: var(--kind-soft);
}
.kind-option.compact,
.kind-switch.compact .kind-option {
  padding: 4px 8px;
  font-size: 11px;
  border-radius: 6px;
}
.kind-key {
  margin-left: 2px;
  opacity: .55;
  font-size: 8px;
  line-height: 12px;
  padding: 0 3px;
  background: transparent;
  border-color: currentColor;
  color: inherit;
}
.kind-hint {
  margin-left: 3px;
  font-size: 10px;
  color: var(--rf-text-faint);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>

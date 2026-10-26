<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { Editor, EditorContent } from '@tiptap/vue-3'
import StarterKit from '@tiptap/starter-kit'
import Image from '@tiptap/extension-image'
import { Table, TableRow, TableHeader, TableCell } from '@tiptap/extension-table'
import TextAlign from '@tiptap/extension-text-align'
import Subscript from '@tiptap/extension-subscript'
import Superscript from '@tiptap/extension-superscript'
import {
  Bold, Italic, Strikethrough, Code, Heading2, List, ListOrdered,
  CodeXml, ImagePlus, Table2, Minus, Undo2, Redo2,
} from 'lucide-vue-next'

const props = defineProps<{
  modelValue: string
  placeholder?: string
  minHeight?: number
}>()

const emit = defineEmits<{ (e: 'update:modelValue', v: string): void; (e: 'insert-shot'): void }>()

const editor = shallowRef<Editor | null>(null)
const empty = ref(true)

onMounted(() => {
  editor.value = new Editor({
    content: props.modelValue || '',
    extensions: [
      StarterKit,
      Image.configure({ inline: false, allowBase64: true }),
      Table.configure({ resizable: true }),
      TableRow, TableHeader, TableCell,
      TextAlign.configure({ types: ['heading', 'paragraph'] }),
      Subscript, Superscript,
    ],
    editorProps: {
      attributes: { spellcheck: 'false' },
    },
    onUpdate: ({ editor: ed }) => {
      empty.value = ed.isEmpty
      emit('update:modelValue', ed.isEmpty ? '' : ed.getHTML())
    },
  })
  empty.value = editor.value.isEmpty
})

watch(
  () => props.modelValue,
  (v) => {
    const ed = editor.value
    if (!ed) return
    const current = ed.isEmpty ? '' : ed.getHTML()
    if (v !== current) {
      ed.commands.setContent(v || '', { emitUpdate: false })
      empty.value = ed.isEmpty
    }
  },
)

onBeforeUnmount(() => editor.value?.destroy())

function insertImage(src: string) {
  editor.value?.chain().focus().setImage({ src }).run()
}

function focus() {
  editor.value?.commands.focus('end')
}

defineExpose({ insertImage, focus })
</script>

<template>
  <div class="rf-editor">
    <div class="rf-toolbar">
      <button title="加粗 Ctrl+B" @click.prevent="editor?.chain().focus().toggleBold().run()"><Bold :size="14" /></button>
      <button title="斜体 Ctrl+I" @click.prevent="editor?.chain().focus().toggleItalic().run()"><Italic :size="14" /></button>
      <button title="删除线" @click.prevent="editor?.chain().focus().toggleStrike().run()"><Strikethrough :size="14" /></button>
      <button title="行内代码" @click.prevent="editor?.chain().focus().toggleCode().run()"><Code :size="14" /></button>
      <span class="rf-sep" />
      <button title="标题" @click.prevent="editor?.chain().focus().toggleHeading({ level: 2 }).run()"><Heading2 :size="14" /></button>
      <button title="无序列表" @click.prevent="editor?.chain().focus().toggleBulletList().run()"><List :size="14" /></button>
      <button title="有序列表" @click.prevent="editor?.chain().focus().toggleOrderedList().run()"><ListOrdered :size="14" /></button>
      <button title="代码块" @click.prevent="editor?.chain().focus().toggleCodeBlock().run()"><CodeXml :size="14" /></button>
      <span class="rf-sep" />
      <button title="表格" @click.prevent="editor?.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run()"><Table2 :size="14" /></button>
      <button title="分隔线" @click.prevent="editor?.chain().focus().setHorizontalRule().run()"><Minus :size="14" /></button>
      <span class="rf-sep" />
      <button title="插入当前截图" @click.prevent="$emit('insert-shot')"><ImagePlus :size="14" /></button>
      <span class="rf-flex" />
      <button title="撤销 Ctrl+Z" @click.prevent="editor?.chain().focus().undo().run()"><Undo2 :size="14" /></button>
      <button title="重做 Ctrl+Shift+Z" @click.prevent="editor?.chain().focus().redo().run()"><Redo2 :size="14" /></button>
    </div>
    <div class="rf-editor-body" :style="{ minHeight: (minHeight ?? 90) + 'px' }">
      <div v-if="placeholder && empty" class="rf-placeholder">{{ placeholder }}</div>
      <EditorContent :editor="editor ?? undefined" />
    </div>
  </div>
</template>

<style scoped>
.rf-editor {
  border: 1px solid var(--rf-border);
  border-radius: 10px;
  background: var(--rf-well);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  transition: border-color .15s ease;
}
.rf-editor:focus-within { border-color: var(--rf-border-strong); }
.rf-toolbar {
  display: flex;
  align-items: center;
  gap: 1px;
  padding: 4px 6px;
  border-bottom: 1px solid var(--rf-border);
  background: var(--rf-panel);
  flex-wrap: wrap;
}
.rf-toolbar button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 25px;
  height: 25px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--rf-text-faint);
  cursor: pointer;
  padding: 0;
  transition: background .12s ease, color .12s ease;
}
.rf-toolbar button:hover { background: var(--rf-card-hover); color: var(--rf-text); }
.rf-sep { width: 1px; height: 14px; background: var(--rf-border); margin: 0 5px; }
.rf-flex { flex: 1; }
.rf-editor-body { position: relative; padding: 9px 12px; overflow: auto; flex: 1; }
.rf-placeholder {
  position: absolute;
  top: 9px;
  left: 12px;
  color: var(--rf-text-faint);
  pointer-events: none;
  font-size: 13px;
  line-height: 1.7;
}
</style>

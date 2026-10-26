<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import {
  Activity, Copy, Database, FolderOpen, Keyboard, PlugZap, Rocket, X,
} from 'lucide-vue-next'
import { useSettings } from '../composables/useSettings'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ (e: 'close'): void }>()

const {
  settings, load, save, testAI, setHotkey, setAutostart,
  openDataDir, exportMarkdown, analyzeAll, copyText,
} = useSettings()

const form = reactive({
  baseURL: '',
  apiKey: '',
  model: '',
  hotkey: '',
})
const aiEnabled = ref(true)
const autoStart = ref(false)
const testing = ref(false)
const saving = ref(false)
const analyzing = ref(false)
const exporting = ref(false)
const lastExport = ref('')

watch(
  () => props.open,
  async (v) => {
    if (!v) return
    await load()
    form.baseURL = settings.value['ai.baseURL'] ?? ''
    form.apiKey = settings.value['ai.apiKey'] ?? ''
    form.model = settings.value['ai.model'] ?? ''
    form.hotkey = settings.value.hotkey ?? 'Ctrl+Alt+R'
    aiEnabled.value = (settings.value['ai.enabled'] ?? 'true') === 'true'
    autoStart.value = settings.value.autostart === 'true'
  },
)

function aiPayload() {
  return {
    'ai.enabled': aiEnabled.value ? 'true' : 'false',
    'ai.baseURL': form.baseURL.trim(),
    'ai.apiKey': form.apiKey.trim(),
    'ai.model': form.model.trim(),
  }
}

async function onSave() {
  if (saving.value) return
  if (!form.hotkey.trim()) {
    ElMessage.warning('全局热键不能为空')
    return
  }
  saving.value = true
  try {
    await save(aiPayload())
    if (form.hotkey.trim() !== (settings.value.hotkey ?? '')) {
      await setHotkey(form.hotkey.trim())
    }
    if (autoStart.value !== (settings.value.autostart === 'true')) {
      await setAutostart(autoStart.value)
    }
    ElMessage.success('设置已保存')
    emit('close')
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  } finally {
    saving.value = false
  }
}

async function onTest() {
  testing.value = true
  try {
    await save(aiPayload())
    const reply = await testAI()
    ElMessage.success('连接成功：' + reply)
  } catch (e) {
    ElMessage.error('连接失败：' + (e instanceof Error ? e.message : String(e)))
  } finally {
    testing.value = false
  }
}

async function onAnalyzeAll() {
  analyzing.value = true
  try {
    await save(aiPayload())
    const n = await analyzeAll()
    if (n > 0) ElMessage.success(`已排队分析 ${n} 条记录，看板上会陆续更新`)
    else ElMessage.info('没有待分析的记录')
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  } finally {
    analyzing.value = false
  }
}

async function onExport() {
  exporting.value = true
  try {
    lastExport.value = await exportMarkdown()
    ElMessage.success('已导出，可打开数据目录查看')
  } catch (e) {
    ElMessage.error('导出失败：' + (e instanceof Error ? e.message : String(e)))
  } finally {
    exporting.value = false
  }
}

async function copyPath() {
  if (!lastExport.value) return
  const ok = await copyText(lastExport.value)
  if (ok) ElMessage.success('路径已复制')
}

async function onOpenDataDir() {
  try {
    await openDataDir()
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  }
}
</script>

<template>
  <el-dialog
    :model-value="open"
    width="640px"
    :show-close="false"
    class="settings-dialog"
    @close="emit('close')"
  >
    <template #header>
      <div class="settings-head">
        <div>
          <div class="eyebrow">SETTINGS</div>
          <div class="settings-title">设置</div>
        </div>
        <button class="icon-btn" @click="emit('close')"><X :size="17" /></button>
      </div>
    </template>

    <div class="settings-body">
      <section class="group">
        <div class="group-head">
          <span class="group-icon"><PlugZap :size="14" /></span>
          <div>
            <div class="group-title">AI 分析</div>
            <div class="group-sub">Anthropic Messages 协议，任意兼容端点都可接入</div>
          </div>
          <span class="spacer" />
          <el-switch v-model="aiEnabled" />
        </div>

        <div class="field">
          <label>Base URL</label>
          <el-input v-model="form.baseURL" placeholder="https://.../apps/anthropic" />
        </div>
        <div class="field">
          <label>API Key</label>
          <el-input v-model="form.apiKey" type="password" show-password placeholder="sk-..." />
        </div>
        <div class="field">
          <label>模型</label>
          <el-input v-model="form.model" placeholder="qwen3.8-max" />
          <div class="field-hint">当前端点实测可用：<code>qwen3.8-max</code> / <code>qwen3.7-plus</code>（支持看图）</div>
        </div>
        <div class="row-actions">
          <button class="text-btn" :disabled="testing" @click="onTest">
            <Activity :size="13" /> {{ testing ? '测试中…' : '测试 AI 连接' }}
          </button>
          <button class="text-btn" :disabled="analyzing" @click="onAnalyzeAll">
            <Rocket :size="13" /> {{ analyzing ? '排队中…' : '分析所有未处理记录' }}
          </button>
        </div>
      </section>

      <section class="group">
        <div class="group-head">
          <span class="group-icon"><Keyboard :size="14" /></span>
          <div>
            <div class="group-title">捕获热键</div>
            <div class="group-sub">全局生效，保存后立即重新注册</div>
          </div>
        </div>
        <div class="field inline">
          <label>热键</label>
          <el-input v-model="form.hotkey" placeholder="Ctrl+Alt+R" style="width: 220px" />
          <span class="field-hint inline">例如 Ctrl+Alt+R / Ctrl+Shift+Space</span>
        </div>
      </section>

      <section class="group">
        <div class="group-head">
          <span class="group-icon"><Database :size="14" /></span>
          <div>
            <div class="group-title">数据</div>
            <div class="group-sub">SQLite 与截图均保存在本机数据目录</div>
          </div>
        </div>
        <div class="row-actions wrap">
          <button class="outline-btn" @click="onOpenDataDir"><FolderOpen :size="14" /> 打开数据目录</button>
          <button class="outline-btn" :disabled="exporting" @click="onExport">
            <Database :size="14" /> {{ exporting ? '导出中…' : '导出全部为 Markdown' }}
          </button>
        </div>
        <div v-if="lastExport" class="export-path">
          <span class="export-label">EXPORTED</span>
          <span class="export-file" :title="lastExport">{{ lastExport }}</span>
          <button class="mini-icon" title="复制路径" @click="copyPath"><Copy :size="12" /></button>
        </div>
      </section>

      <section class="group">
        <div class="group-head">
          <span class="group-icon"><Rocket :size="14" /></span>
          <div>
            <div class="group-title">启动</div>
            <div class="group-sub">登录 Windows 后自动在托盘待命</div>
          </div>
          <span class="spacer" />
          <el-switch v-model="autoStart" />
        </div>
      </section>

      <p class="privacy-note">
        API Key 仅保存在本机数据目录的 SQLite 中，只会发送给你配置的 AI 端点；不会上传到其他服务。
      </p>
    </div>

    <template #footer>
      <div class="settings-footer">
        <button class="btn-ghost" @click="emit('close')">取消</button>
        <button class="btn-outline" :disabled="saving" @click="onSave">
          {{ saving ? '保存中…' : '保存设置' }}
        </button>
      </div>
    </template>
  </el-dialog>
</template>

<style scoped>
.settings-head { display: flex; align-items: flex-start; justify-content: space-between; }
.eyebrow { font-family: var(--rf-mono); font-size: 9.5px; letter-spacing: .15em; color: var(--rf-accent); }
.settings-title { font-size: 20px; font-weight: 700; margin-top: 3px; }
.icon-btn {
  width: 30px; height: 30px; border-radius: 8px; border: 1px solid transparent;
  background: transparent; color: var(--rf-text-dim); display: inline-flex;
  align-items: center; justify-content: center; cursor: pointer;
}
.icon-btn:hover { background: var(--rf-card); border-color: var(--rf-border); color: var(--rf-text); }
.settings-body { display: flex; flex-direction: column; gap: 14px; max-height: 62vh; overflow-y: auto; padding-right: 4px; }
.group {
  background: var(--rf-panel);
  border: 1px solid var(--rf-border);
  border-radius: 12px;
  padding: 14px 16px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.group-head { display: flex; align-items: center; gap: 9px; }
.group-icon {
  width: 26px; height: 26px; border-radius: 7px; display: inline-flex;
  align-items: center; justify-content: center; color: var(--rf-accent);
  background: var(--rf-accent-soft); border: 1px solid rgba(168,99,19,.2);
}
.group-title { font-size: 13px; font-weight: 700; }
.group-sub { font-size: 10.5px; color: var(--rf-text-faint); margin-top: 2px; }
.spacer { flex: 1; }
.field { display: flex; flex-direction: column; gap: 5px; }
.field.inline { flex-direction: row; align-items: center; gap: 10px; }
.field label { font-size: 10.5px; font-weight: 700; letter-spacing: .07em; color: var(--rf-text-faint); text-transform: uppercase; }
.field-hint { font-size: 10.5px; color: var(--rf-text-faint); line-height: 1.6; }
.field-hint.inline { margin-left: 2px; }
.field-hint code { font-family: var(--rf-mono); font-size: 10px; color: var(--rf-accent); background: var(--rf-accent-soft); border-radius: 3px; padding: 1px 4px; }
.row-actions { display: flex; gap: 8px; align-items: center; }
.row-actions.wrap { flex-wrap: wrap; }
.text-btn {
  display: inline-flex; align-items: center; gap: 5px; padding: 5px 8px;
  border: 1px solid transparent; border-radius: 6px; background: transparent;
  color: var(--rf-text-dim); font-size: 11.5px; cursor: pointer;
}
.text-btn:hover:not(:disabled) { color: var(--rf-accent); background: var(--rf-accent-soft); }
.text-btn:disabled { color: var(--rf-text-faint); cursor: default; }
.outline-btn {
  display: inline-flex; align-items: center; gap: 6px;
  border: 1px solid var(--rf-border-strong); border-radius: 7px;
  background: transparent; color: var(--rf-text); font-size: 11.5px;
  padding: 6px 11px; cursor: pointer;
}
.outline-btn:hover:not(:disabled) { border-color: var(--rf-accent); color: var(--rf-accent); background: var(--rf-accent-soft); }
.outline-btn:disabled { color: var(--rf-text-faint); cursor: default; }
.export-path {
  display: flex; align-items: center; gap: 8px; border-top: 1px dashed var(--rf-border);
  padding-top: 9px; font-size: 10.5px;
}
.export-label { font-family: var(--rf-mono); font-size: 8.5px; color: var(--rf-accent); letter-spacing: .1em; }
.export-file { color: var(--rf-text-dim); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1; }
.mini-icon { border: none; background: transparent; color: var(--rf-text-faint); cursor: pointer; padding: 2px; }
.mini-icon:hover { color: var(--rf-text); }
.privacy-note { font-size: 10.5px; line-height: 1.7; color: var(--rf-text-faint); margin: 0 4px; }
.settings-footer { display: flex; justify-content: flex-end; gap: 8px; }
.btn-ghost, .btn-outline {
  border-radius: 8px; font-size: 12.5px; font-weight: 600; padding: 7px 15px; cursor: pointer;
  transition: background .14s ease, border-color .14s ease, color .14s ease;
}
.btn-ghost { color: var(--rf-text-dim); background: transparent; border: 1px solid transparent; }
.btn-ghost:hover { color: var(--rf-text); background: var(--rf-card); }
.btn-outline { color: var(--rf-accent); background: transparent; border: 1px solid rgba(168,99,19,.48); }
.btn-outline:hover:not(:disabled) { background: var(--rf-accent-soft); border-color: var(--rf-accent); }
.btn-outline:disabled { color: var(--rf-text-faint); border-color: var(--rf-border); cursor: default; }
</style>

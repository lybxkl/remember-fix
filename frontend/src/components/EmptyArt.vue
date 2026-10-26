<script setup lang="ts">
import { defaultHotkey } from '../lib/platform'

defineProps<{ hotkey?: string }>()
defineEmits<{ (e: 'record'): void }>()
</script>

<template>
  <div class="empty">
    <div class="art" aria-hidden="true">
      <svg viewBox="0 0 280 178" fill="none">
        <defs>
          <radialGradient id="rf-empty-glow" cx="50%" cy="50%" r="50%">
            <stop offset="0%" stop-color="var(--rf-accent)" stop-opacity="0.13" />
            <stop offset="100%" stop-color="var(--rf-accent)" stop-opacity="0" />
          </radialGradient>
        </defs>
        <ellipse class="glow" cx="124" cy="86" rx="86" ry="58" fill="url(#rf-empty-glow)" />

        <path class="desk" d="M28 146h224" />
        <path class="desk-tick" d="M46 146v5M234 146v5" />

        <g class="bug">
          <g class="antennae" stroke="var(--rf-accent)" stroke-width="1.4" stroke-linecap="round">
            <path d="M113 42c-6-8-9-11-14-14" />
            <path d="M127 42c6-8 9-11 14-14" />
          </g>
          <circle cx="96" cy="26" r="1.7" fill="var(--rf-accent)" />
          <circle cx="144" cy="26" r="1.7" fill="var(--rf-accent)" />

          <g stroke="var(--rf-accent)" stroke-width="1.5" stroke-linecap="round">
            <path d="M103 66 76 52M101 84 71 81M102 101 76 111" opacity=".62" />
            <path d="M137 66 164 52M139 84 169 81M138 101 164 111" opacity=".62" />
          </g>

          <circle cx="120" cy="48" r="11.5" fill="rgba(168,99,19,.07)" stroke="var(--rf-accent)" stroke-width="1.6" />
          <ellipse cx="120" cy="88" rx="27" ry="33" fill="rgba(168,99,19,.04)" stroke="var(--rf-accent)" stroke-width="1.6" />
          <path d="M120 66v44" stroke="var(--rf-accent)" stroke-width="1.2" stroke-dasharray="3 6" opacity=".55" />
          <circle cx="112" cy="46" r="1.2" fill="var(--rf-accent)" opacity=".8" />
          <circle cx="128" cy="46" r="1.2" fill="var(--rf-accent)" opacity=".8" />
        </g>

        <g class="wrench" transform="translate(166 58) rotate(-7)" stroke="var(--rf-text-dim)" stroke-width="1.25" stroke-linecap="round" stroke-linejoin="round" opacity=".8">
          <path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z" transform="scale(2.15)" />
        </g>

        <g stroke="var(--rf-text-faint)" stroke-width="1.2" stroke-linecap="round" opacity=".65">
          <path d="M68 40v8M64 44h8" />
          <path d="M196 120v6M193 123h6" />
        </g>
      </svg>
    </div>

    <div class="title">桌面很干净，像刚下班</div>
    <div class="sub">
      按 <kbd>{{ hotkey || defaultHotkey }}</kbd> 或点托盘 / 菜单栏图标，把刚截的图记成一条 Bug
    </div>
    <button class="record" @click="$emit('record')">记第一条</button>
    <div class="mini">也可以先复制一段报错日志</div>
  </div>
</template>

<style scoped>
.empty {
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 7px;
  padding-bottom: 24px;
}
.art { width: 300px; height: 190px; margin-bottom: -6px; }
.art svg { width: 100%; height: 100%; overflow: visible; }
.glow { animation: glow 5.5s ease-in-out infinite; transform-origin: 124px 86px; }
.bug { animation: float 5s ease-in-out infinite; transform-origin: 120px 88px; }
.antennae { animation: sway 4s ease-in-out infinite; transform-origin: 120px 42px; }
.desk, .desk-tick { stroke: var(--rf-border-strong); stroke-width: 1; }
@keyframes float {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-4px); }
}
@keyframes sway {
  0%, 100% { transform: rotate(0deg); }
  50% { transform: rotate(2.5deg); }
}
@keyframes glow {
  0%, 100% { opacity: .75; transform: scale(1); }
  50% { opacity: 1; transform: scale(1.05); }
}
.title { font-size: 17px; font-weight: 700; color: var(--rf-text); letter-spacing: -0.01em; }
.sub { font-size: 12.5px; color: var(--rf-text-dim); display: flex; align-items: center; gap: 5px; }
.sub kbd {
  background: var(--rf-panel);
  border: 1px solid var(--rf-border);
  border-bottom-color: var(--rf-border-strong);
  border-radius: 4px;
  padding: 0 5px;
  font-family: var(--rf-mono);
  font-size: 10px;
  line-height: 18px;
  color: var(--rf-text);
}
.record {
  margin-top: 6px;
  border: 1px solid rgba(168, 99, 19, 0.48);
  background: transparent;
  color: var(--rf-accent);
  font-size: 12px;
  font-weight: 600;
  padding: 6px 16px;
  border-radius: 8px;
  cursor: pointer;
  transition: background .15s ease, border-color .15s ease;
}
.record:hover { background: var(--rf-accent-soft); border-color: var(--rf-accent); }
.mini { font-size: 10.5px; color: var(--rf-text-faint); margin-top: 1px; }
</style>

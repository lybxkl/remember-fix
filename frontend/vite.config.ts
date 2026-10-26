import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const r = (p: string) => resolve(dirname(fileURLToPath(import.meta.url)), p)

export default defineConfig({
  plugins: [vue()],
  clearScreen: false,
  server: { port: 5178, strictPort: true },
  build: {
    // macOS 12 的 WKWebView 基线是 Safari 15，不能只按 Chrome 版本降级。
    target: 'safari15',
    chunkSizeWarningLimit: 1600,
    rollupOptions: {
      input: {
        main: r('index.html'),
        capture: r('capture.html'),
        edge: r('edge.html'),
      },
    },
  },
})

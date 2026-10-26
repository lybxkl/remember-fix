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
    target: 'chrome110',
    chunkSizeWarningLimit: 1600,
    rollupOptions: {
      input: {
        main: r('index.html'),
        capture: r('capture.html'),
      },
    },
  },
})

// 构建产物体积预算卡关脚本：JS 总量超过预算即失败。
import { readdirSync, statSync } from 'node:fs'
import { join } from 'node:path'

const ASSETS = new URL('../dist/assets', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1')

const TOTAL_BUDGET = 3.5 * 1024 * 1024 // JS 总预算 3.5MB（min 后）

let files
try {
  files = readdirSync(ASSETS).filter((f) => f.endsWith('.js'))
} catch {
  console.error('[budget] 未找到 dist/assets，请先执行 vite build')
  process.exit(1)
}

let total = 0
const rows = []
for (const f of files) {
  const size = statSync(join(ASSETS, f)).size
  total += size
  rows.push({ chunk: f, KB: Math.round(size / 1024) })
}
rows.push({ chunk: '== TOTAL ==', KB: Math.round(total / 1024) })
console.table(rows)

if (total > TOTAL_BUDGET) {
  console.error(`[budget] JS 总体积 ${(total / 1048576).toFixed(2)}MB 超出预算 ${(TOTAL_BUDGET / 1048576).toFixed(2)}MB`)
  process.exit(1)
}
console.log(`[budget] JS 总体积 ${(total / 1048576).toFixed(2)}MB，未超预算 ✓`)

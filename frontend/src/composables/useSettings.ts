import { ref } from 'vue'
import { call } from '../lib/wails'

const settings = ref<Record<string, string>>({})

export function useSettings() {
  async function load() {
    settings.value = (await call<Record<string, string>>('main.ConfigService.GetAll')) ?? {}
  }

  async function save(kv: Record<string, string>) {
    await call('main.ConfigService.SetMany', kv)
    await load()
  }

  async function testAI(): Promise<string> {
    return call<string>('main.ConfigService.TestAI')
  }

  async function setHotkey(accel: string) {
    await call('main.ConfigService.SetHotkey', accel)
    await load()
  }

  async function setAutostart(enabled: boolean) {
    await call('main.ConfigService.SetAutostart', enabled)
    await load()
  }

  async function setEdgeEnabled(enabled: boolean) {
    await call('main.ConfigService.SetEdgeEnabled', enabled)
    await load()
  }

  async function openDataDir() {
    await call('main.ConfigService.OpenDataDir')
  }

  async function exportMarkdown(): Promise<string> {
    return call<string>('main.BugService.ExportMarkdown')
  }

  async function analyzeAll(): Promise<number> {
    return call<number>('main.BugService.AnalyzeUnanalyzed')
  }

  async function copyText(text: string): Promise<boolean> {
    return call<boolean>('main.ConfigService.CopyText', text)
  }

  return {
    settings, load, save, testAI, setHotkey, setAutostart, setEdgeEnabled,
    openDataDir, exportMarkdown, analyzeAll, copyText,
  }
}

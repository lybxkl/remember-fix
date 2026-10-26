// 平台相关的快捷键展示；行为判断仍以后端 Wails 为准。
export const isMac =
  typeof navigator !== 'undefined' && /mac/i.test(`${navigator.platform} ${navigator.userAgent}`)

export const defaultHotkey = isMac ? 'Cmd+Alt+R' : 'Ctrl+Alt+R'
export const hotkeyExample = isMac
  ? 'Cmd+Alt+R / Cmd+Shift+Space'
  : 'Ctrl+Alt+R / Ctrl+Shift+Space'
export const modName = isMac ? 'Cmd' : 'Ctrl'
export const modSymbol = isMac ? '⌘' : 'Ctrl'
export const altName = isMac ? 'Option' : 'Alt'
export const screenshotKeys = isMac ? ['Cmd', 'Ctrl', 'Shift', '4'] : ['Win', 'Shift', 'S']

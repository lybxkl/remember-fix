import { Call, Events } from '@wailsio/runtime'

// 调用 Go 绑定服务：main 包服务的完全限定名
export function call<T>(name: string, ...args: unknown[]): Promise<T> {
  return Call.ByName(name, ...args) as Promise<T>
}

// 订阅 Go 事件。Wails 的 emit data 可能是数组包裹，统一取第一个。
export function onEvent<T = unknown>(name: string, cb: (data: T) => void): () => void {
  const off = Events.On(name, (ev: unknown) => {
    const d = ev as { data?: unknown }
    let payload = d?.data
    if (Array.isArray(payload)) payload = payload[0]
    cb(payload as T)
  })
  return off as unknown as () => void
}

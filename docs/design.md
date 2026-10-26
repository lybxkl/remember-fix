# 「记得修」Remember-Fix — 技术设计文档

> 一个 Windows / macOS 桌面 Bug 快速记录与看板管理助手。
> 截屏 → 点一下 → 自动抓剪贴板 → 记一笔 → AI 分类 → 看板管理。

- 框架：Wails v3 (`github.com/wailsapp/wails/v3` `v3.0.0-beta.34`)
- 前端：Vue 3 + TypeScript + Vite + Element Plus + TipTap
- 平台：**Windows 10/11（WebView2）与 macOS 12+（Cocoa / CGO）**，共用同一套业务与前端代码

---

## 1. 产品核心场景

| 场景 | 流程 |
|---|---|
| 顺手记 bug | 任意界面截屏到剪贴板（Windows `Win+Shift+S` / macOS `⌘+Ctrl+Shift+4`）→ 按默认热键（Windows `Ctrl+Alt+R` / macOS `⌘+Option+R`）或点托盘/菜单栏图标 → 弹出小窗，截图已自动就位 → 写一句描述 → `Ctrl/⌘+Enter` 保存（2 秒内完成） |
| 复制文字报错 | 复制错误日志/文本 → 触发捕获窗 → 自动粘贴文本内容 |
| 回顾管理 | 打开主窗口看板：待处理 / 进行中 / 已修复 / 已验证 / 不修，拖拽流转 |
| AI 辅助 | 保存后自动调大模型：生成规范标题、分类、严重程度、标签、疑似复现步骤 |
| 排查重复 | 保存时/看板中提示"可能与 bug#xx 相似"（二期） |

## 2. 总体架构

```
┌────────────────────────────────────────────────────────────┐
│                      Wails3 App (Go)                       │
│                                                            │
│  入口层    SystemTray.OnClick   GlobalShortcut.Register     │
│            Window.Manager(主窗口 / 捕获窗 / 贴边窗)          │
│                                                            │
│  服务层    CaptureService   抓剪贴板快照、历史 ring buffer   │
│  (绑定到   BugService       CRUD、状态流转、筛选             │
│   前端)    AIService        LLM 调用（Anthropic Messages）    │
│            ConfigService    设置读写、API Key 管理           │
│            EdgeService      贴边窗展开 / 收起 / 触发捕获     │
│                                                            │
│  基础层    clipboard/  Win32 / NSPasteboard 剪贴板（序列号、DIB/TIFF→PNG）│
│            store/      SQLite (modernc.org/sqlite 纯 Go)    │
│            autostart/  HKCU Run / LaunchAgent 开机自启        │
│            ai/         HTTP 客户端、prompt、JSON 解析        │
│            fsx/        数据目录、截图文件落盘                 │
│                                                            │
│  事件      app.Event.Emit  capture:ready / bug:saved /      │
│            ai:updated / clipboard:changed                  │
└────────────────────────┬───────────────────────────────────┘
                         │ bindings (生成 TS) + events
┌────────────────────────▼───────────────────────────────────┐
│              Vue3 前端（一个 Vite 工程，三个窗口入口）        │
│   #/board   主窗口：看板视图 / 列表视图 / 详情抽屉 / 设置      │
│   #/capture 捕获窗：预览+历史切换+TipTap+快速保存             │
│   #/edge    贴边窗：四边可吸附的小虫，悬停展开、点击记录      │
└────────────────────────────────────────────────────────────┘
```

## 3. 关键技术决策（已查证 beta.34 文档）

### 3.1 剪贴板图片 —— 最重要的风险点，按平台自封装

**结论：Wails3 内置 `app.Clipboard` 只支持纯文本**（官方文档明确：Images not yet supported / No clipboard events / Must poll）。

**Windows（纯 Go、无 CGO）**：`internal/clipboard/windows.go` 用 `golang.org/x/sys/windows` 直接调 Win32：

```go
// 读取顺序（Win+Shift+S 截图后剪贴板同时存在多种格式）
1. RegisterClipboardFormat("PNG")   // Win10/11 截图工具会放原始 PNG，保 alpha、无损
2. CF_DIBV5                          // 含 ICC/alpha 信息，BITMAPV5HEADER 解析
3. CF_DIB                            // 兜底，BITMAPINFOHEADER，注意 bottom-up 行序
// OpenClipboard 失败重试（截图工具占用剪贴板是常态，重试 3 次 x 100ms）
// DIB → PNG：处理 32bpp BGRA、行 padding、自下而上翻转；用 image.RGBA + png.Encode
```

**macOS（CGO + Cocoa）**：`internal/clipboard/clipboard_darwin.go` 通过 cgo 调用 `NSPasteboard`，Objective-C 放在 `clipboard_darwin.m`：

```objc
1. changeCount                          // 序列号，用于轮询去重
2. NSPasteboardTypeString               // 文本
3. NSPasteboardTypePNG                  // 优先原始 PNG
4. NSPasteboardTypeTIFF → NSBitmapImageRep → PNG  // 截图工具常只写 TIFF
5. [[NSWorkspace sharedWorkspace] frontmostApplication].localizedName  // 来源应用
```

`darwin && !cgo` 时退化为空实现，便于在无 macOS 工具链的机器上对内部包做交叉编译检查；正式 macOS 构建使用 `CGO_ENABLED=1`。`New()` 由 `clipboard_new_windows.go` / `clipboard_new_darwin.go` 按平台返回实现。

**变化检测不用轮询内容**：后台 goroutine 每 400ms 调序列号（Windows `GetClipboardSequenceNumber` / macOS `changeCount`，零开销，不打开剪贴板），序列号变了才真正读取，维护**最近 10 条历史 ring buffer**（图片存临时文件，文本截断 4KB）。收益：用户连截 3 张图再打开助手，捕获窗里能左右切换挑选。

**时序兜底**：截图落剪贴板有延迟，捕获窗触发时若序列号与缓存相同，延迟 350ms 再抓一次。

### 3.2 窗口策略

| 窗口 | 参数 | 生命周期 |
|---|---|---|
| 主窗口（看板） | 普通有框窗口 1200×720（最小 980×600） | 关闭 = 隐藏到托盘（不清进程）；Windows 使用原生亮色标题栏，macOS 使用 Aqua 外观 |
| 捕获窗 | `Frameless + AlwaysOnTop + Width:500 Height:680`，无任务栏图标 | 每次触发 Show+Focus，Esc/保存后 Hide，单例复用；macOS 加入所有 Space 并覆盖全屏应用 |
| 贴边窗 | 透明 + `Frameless + AlwaysOnTop + HiddenOnTaskbar + DisableResize`，收起 24×64 / 64×24、展开 184×232 | 默认常驻屏幕右缘；可拖到屏幕任意一边并吸附，位置持久化；设置可关；点击触发捕获后自动收起 |

- 触发方式（四选一都做）：托盘/菜单栏左键单击 / 全局热键（Windows 默认 `Ctrl+Alt+R`，macOS 默认 `Cmd+Alt+R`，可配置）/ 主窗口"记录"按钮 / 屏幕边缘贴边宠物
- Frameless 窗口自带 CSS 拖动区：`style="--wails-draggable: drag"`
- 前端**单工程多入口**：`index.html` / `capture.html` / `edge.html` 共用同一 assets 与 Vite 构建，各自挂载独立根组件
- 贴边窗在 `WindowRuntimeReady` 后再 Show（过早 Show 会被 Windows 忽略）；每次布局先 `SetSize`，再读回实际尺寸重新贴边，兼容 WebView2 对极窄窗口的最小尺寸限制
- 贴边窗必须把 `DisableFramelessWindowDecorations` 设为 true，否则 Windows 11 会给无边框透明窗补一层白色圆角底板；展开窗尺寸精确等于卡片尺寸，避免卡片四周出现空白
- 拖动使用系统原生搬窗：指针移动超过阈值后调用 `wails:drag`，由操作系统直接移动窗口，避免每个 `pointermove` 都走一次同步 IPC；Windows 监听 `WindowEndMove/WindowEndResize`、macOS 用 pointerup 加移动静默期兜底，松手后按窗口中心吸附最近屏幕边缘，并把 `edge.dock` / `edge.offset` 写入设置；重启后按保存的方向与沿边位置恢复
- 贴边窗必须是非激活窗口：Windows 使用 `WS_EX_NOACTIVATE`（同时保留透明 / 置顶 / 工具窗标志），macOS 使用 `MacWindowClassPanel + NonActivating`。否则窗口未激活时第一次鼠标按下会被系统用于激活并吞掉，表现为“按下拖不动、也点不开”
- macOS 贴边窗使用 `MacBackdropTransparent + Square 圆角 + DisableShadow + CanJoinAllSpaces | FullScreenAuxiliary | IgnoresCycle`，避免在 Space 切换与全屏应用下消失；托盘图标通过 `SetTemplateIcon` 适配菜单栏明暗。Wails 的透明 WebView 依赖 `private_mac_apis` 构建标签，Darwin Taskfile 已默认带上（未上架 App Store，不影响分发）
- 默认热键由 `internal/store/defaults_{windows,darwin}.go` 提供；macOS 的 `Cmd` 由 Wails accelerator 映射为 Command，全局热键使用 Carbon，无需辅助功能权限

### 3.3 截图如何给前端显示

- **推荐**：注册 Asset Handler，拦截 `/shots/**` 路由，`http.ServeFile` 只从 `<dataDir>/shots/` 读图片（限制图片扩展名并校验路径不越界，避免数据库/日志被该路由暴露），前端 `<img src="/shots/2026/10/xxx.png">`（beta.34 的 `AssetOptions.Handler`，实现时以该版本文档为准）
- **保底**：binding 返回 `data:image/png;base64,...`（截图一般 0.2–2MB，本地通信可接受，实现最简单）

### 3.4 数据存储 —— SQLite（纯 Go 驱动）

`modernc.org/sqlite` + 原生 `database/sql`。库与截图都在 `os.UserConfigDir()/remember-fix/`：Windows 为 `%APPDATA%\remember-fix\`，macOS 为 `~/Library/Application Support/remember-fix/`。macOS 开机自启写入 `~/Library/LaunchAgents/com.rememberfix.desktop.plist`（Windows 为 HKCU `Run`）。

```sql
CREATE TABLE bugs (
  id            TEXT PRIMARY KEY,          -- uuid v7（可排序）
  title         TEXT NOT NULL,             -- 默认取描述首行，AI 可改写
  description   TEXT,                      -- TipTap JSON
  status        TEXT NOT NULL DEFAULT 'todo',  -- todo|doing|fixed|verified|wontfix
  category      TEXT NOT NULL DEFAULT 'uncategorized',
                -- ui|logic|crash|performance|compat|data|security|other|uncategorized
  severity      TEXT NOT NULL DEFAULT 'P2',   -- P0|P1|P2|P3
  tags          TEXT,                      -- JSON 数组
  attachments   TEXT,                      -- JSON: [{path,type:'image'|'text',text?}]
  source_app    TEXT,                      -- 可选：捕获时前台窗口名
  ai            TEXT,                      -- JSON: {summary, steps[], raw, model, at}
  ai_status     TEXT DEFAULT 'none',       -- none|pending|done|failed
  created_at    INTEGER, updated_at INTEGER, resolved_at INTEGER
);
CREATE INDEX idx_bugs_status ON bugs(status);
CREATE INDEX idx_bugs_category ON bugs(category);
CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT);
```

### 3.5 大模型接入 —— Anthropic Messages 协议

设置里配 `ai.baseURL + ai.apiKey + ai.model`（`ai.enabled` 总开关）。客户端按 Anthropic Messages 协议请求 `POST /v1/messages`，携带 `x-api-key` 与 `anthropic-version: 2023-06-01`；默认指向阿里云百炼 MaaS 的 Anthropic 兼容端点 `https://token-plan.cn-beijing.maas.aliyuncs.com/apps/anthropic`，默认模型 `qwen3.8-max`（原生支持图片输入，同一模型同时处理截图与文本）。

**能力与触发点：**

| 能力 | 触发 | 输入 | 输出 |
|---|---|---|---|
| 智能归类 | 保存新 bug 后异步执行 | 截图 + 描述 + 剪贴板文本附件 | 严格 JSON：`{title, category, severity, tags[], summary, steps[]}` |
| 批量补全 | 设置页按钮"分析未分析记录" | 同上 | 批量任务，逐条更新，状态事件推送 |
| Markdown 导出 | 设置页/按状态导出 | 本地记录 + 截图 | `exports/remember-fix-<时间>/bugs.md` + `assets/` |
| 转 AI 编程指令 | bug 详情按钮"生成修复 prompt" | 记录 + AI 结论 | 可一键复制给 Claude Code/Cursor |

**工程要点：**
- 输出约束为裸 JSON，兼容 ```json 围栏与首尾花括号提取，不依赖模型 JSON mode
- 单次分析 `max_tokens=4096`，HTTP 超时 120s、分析总超时 150s；温度沿用服务端默认
- **AI 失败绝不阻塞保存**：`ai_status=failed`，保留上一次成功结果，可手动重试或批量补全
- AI 只回填空字段：用户已填写的标题/分类/标签/严重度不会被覆盖
- 并发分析用 `sync.Map` 去重，避免重复请求
- API Key 存本机 settings 表（二期增强：Windows DPAPI / macOS Keychain 加密）；不写入源码或日志

### 3.6 前端结构（严格使用既定依赖，不新增）

```
frontend/src/
├── main.ts               # hash 分发：#/capture → CaptureApp，其余 → BoardApp
├── apps/
│   ├── BoardApp.vue      # 看板：5 列、卡片 HTML5 拖拽、搜索/筛选
│   └── CaptureApp.vue    # 捕获窗：预览+历史+编辑+保存
├── components/
│   ├── BugCard.vue  BugDrawer.vue(详情)  StatusColumn.vue
│   ├── CapturePreview.vue  HistorySwitcher.vue
│   ├── RichEditor.vue   # TipTap（starter-kit + image + table + sub/sup + text-align）
│   └── SettingsDialog.vue
├── composables/
│   ├── bugs.ts           # 调 bindings，缓存 + 事件驱动刷新
│   ├── capture.ts        # 快照加载、剪贴板历史切换
│   └── events.ts         # 封装 @wailsio/runtime Events
└── bindings/             # wails3 generate bindings 产物
```

- **看板拖拽：原生 HTML5 DnD 自实现**（dragstart 记 bugId → dragover 高亮列 → drop 调 `BugService.UpdateStatus`），不引 vuedraggable
- 状态管理用 composables（reactive 单例 + Events 订阅刷新），不引 pinia——两张窗口数据独立加载，靠 Go 事件保持一致
- Element Plus 按需自动导入；空状态使用内联 SVG 动画，不引入 three.js，守住 bundle 预算

## 4. 关键流程时序

**捕获（核心路径）**
```
热键/托盘点击
 → Go: CaptureService.Snapshot()
    读序列号 → 读 PNG/DIBV5/DIB（Windows）或 PNG/TIFF（macOS）或文本
    → 落盘 %ConfigDir%/remember-fix/shots/
    → app.Event.Emit("capture:ready", {snapshotId, attachments})
 → 捕获窗 Show + Focus（前端已在后台驻留，首开 <100ms）
 → 用户：切换历史(可选) / TipTap 写描述 / 微调 AI 预填字段
 → Ctrl/⌘+Enter → BugService.Save() → 入库 → 发 "bug:saved"
 → 捕获窗 Hide；若主窗口开着 → 看板收到新卡片（动画滑入）
 → AIService.Analyze(bugId) 异步 → 完成后 Emit "ai:updated" → 卡片徽标刷新
```

**剪贴板历史监听（常驻，开销极低）**
```
goroutine: 每 400ms 读剪贴板序列号（Win32 / NSPasteboard changeCount）
  变化 → 读内容(图片存临时文件) → 入 ring buffer(10)
  → 若捕获窗正开着 → Emit "clipboard:changed" → 前端历史切换器刷新
```

## 5. 目录结构

```
remember-fix/
├── main.go                  # 组装：主窗 / 捕获窗 / 贴边窗、托盘、热键、单实例、assets handler
├── services.go              # Wails 绑定服务：Capture / Bug / Config / Edge / AppShell
├── internal/
│   ├── clipboard/           # windows.go(Win32) / clipboard_darwin.go+.m(NSPasteboard) + 单测
│   ├── store/               # store.go(打开/迁移) + bugs.go + defaults_*.go + 单测
│   ├── ai/                  # anthropic.go(Messages 协议) + analyze.go(prompt/解析)
│   ├── capture/             # 剪贴板快照、序列号去重、晚到截图检查
│   ├── autostart/           # Windows HKCU Run / macOS LaunchAgent
│   ├── exporter/            # 按状态导出 Markdown + 截图
│   └── fsx/                 # 数据目录、截图落盘、路径校验
├── frontend/                # Vue3 + Vite，三入口（index / capture / edge）
├── build/                   # appicon、托盘图标、Windows 配置、darwin .app 与打包脚本
├── Taskfile.yml
└── docs/design.md
```

## 6. 风险与对策

| 风险 | 对策 |
|---|---|
| beta.34 API 与文档漂移（beta 期变更多） | go.mod 锁死 beta.34；bindings 用生成器而非手写；多窗口/事件先写 spike 验证 |
| 剪贴板 DIB 格式坑（DPI 缩放、alpha、多显示器拼接图） | 优先读截图工具放的原始 "PNG" 格式；DIB 解析覆盖 24/32bpp + bottom-up/top-down；单测用真实截图样本 |
| 截图工具占用剪贴板导致 OpenClipboard 失败 | 重试 3×100ms；仍失败 UI 提示"再点一次" |
| WebView2 环境缺失（老 Win10） | 打包勾选 WebView2 Evergreen 引导安装 |
| macOS 未签名 / 首次构建环境缺失 | ad-hoc codesign + `.app` 打包；README 说明 Gatekeeper 右键打开与 Xcode CLT 依赖；macOS 路径需真机回归 |
| 剪贴板实现只在 Windows 实机验证 | macOS 侧使用独立 cgo 文件与 `darwin && !cgo` 退化实现，交叉编译可验证非 cgo 部分；正式发布前在 Intel / Apple Silicon 各跑一轮捕获回归 |
| AI 输出不稳定 | 枚举约束 + 围栏/花括号兜底提取；失败保留旧结果且不影响主流程 |
| bundle 超预算（tiptap/element 都不小） | element 按需；空状态改用内联 SVG；已有 check-bundle-budget 脚本卡关 |

## 7. 里程碑

| 阶段 | 内容 | 验收 |
|---|---|---|
| M1 骨架 | wails3 工程、多窗口+托盘+热键、SQLite 迁移、看板手动 CRUD + 拖拽 | 能手动建 bug 并在看板流转 |
| M2 捕获（核心） | Win32 剪贴板图片/文本、序列号监听+历史、捕获窗全流程 | `Win+Shift+S` → 热键 → 2 秒内完成一条带图记录 |
| M3 AI | Anthropic Messages 客户端、保存后异步分析、设置页、重试/降级 | 保存带截图或纯文本 bug，自动补全 AI 标题/分类/标签 |
| M4 打磨 | 剪贴板历史切换、批量分析、生成修复 prompt、导出 markdown、NSIS 安装包、开机自启 | 发布 v0.1.0 |

## 8. 二期候选

多项目空间（bugs 加 project_id）、相似 bug 检测（embedding）、全局剪贴板历史面板、截图标注（画框/箭头）、浏览器插件抓 URL、DPAPI / Keychain 加密 API Key、托盘气泡提醒"你有 12 个 bug 超过一周没修"、macOS 公证（notarization）。

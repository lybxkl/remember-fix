<p align="center">
  <img src="build/appicon.png" width="96" alt="记得修图标" />
</p>

<h1 align="center">记得修 · Remember-Fix</h1>

<p align="center">截屏即记录，AI 自动补全，看板跟进到修完。</p>

<p align="center">
  <img alt="Windows" src="https://img.shields.io/badge/platform-Windows%2010%2F11-2f6f4e" />
  <img alt="Go" src="https://img.shields.io/badge/Go-1.27+-00ADD8" />
  <img alt="Wails" src="https://img.shields.io/badge/Wails-v3.0.0--beta.34-DF0000" />
  <img alt="Vue" src="https://img.shields.io/badge/Vue-3-42b883" />
  <img alt="License" src="https://img.shields.io/badge/license-Apache--2.0-8a6a12" />
</p>

![记得修看板](docs/screenshot-board.png)

## 这是什么

记得修是一款面向 Windows 的本地优先 Bug 捕获与管理工具。按下全局热键，它会读取剪贴板里刚截的图或复制的报错文本，打开一个轻量捕获窗；保存后 AI 自动补全标题、分类、严重度、标签、摘要和复现步骤，再用五列看板跟踪修复状态。

适合个人开发者、QA 和需要随手记录问题的小团队。

## 功能

- **一键捕获**：全局热键或托盘唤起，自动识别剪贴板截图 / 文本；保留最近剪贴板历史，支持 `Alt + ←/→` 切换、文本复制、截图插入描述。
- **AI 自动补全**：保存新记录后异步分析截图和文本，生成标题、分类、严重度、标签、摘要与复现步骤；只回填空白字段，不覆盖手填内容。
- **AI 可控**：可关闭 AI、测试连接、手动重新分析、批量分析未处理记录；失败时保留上一次成功结果，不影响正常保存。
- **五列看板**：待处理 / 进行中 / 已修复 / 已验证 / 不修，原生拖拽流转，自动统计待修与本周清掉数量。
- **检索与筛选**：`/` 快速聚焦搜索，按标题、描述、标签、来源搜索，支持分类 + 严重度组合筛选。
- **详情编辑**：富文本描述（加粗、列表、表格、代码块、截图）、状态 / 分类 / 严重度 / 标签编辑、附件预览、`Ctrl+S` 保存。
- **修复指令**：根据 Bug 与 AI 结论生成可直接投喂 Claude Code / Cursor 的 Markdown 修复提示，包含定位建议、验收标准和回归风险。
- **Markdown 导出**：按状态导出全部记录，自动复制截图到 `assets/`，保留 AI 摘要、复现步骤和文本日志。
- **桌面体验**：热键可改、关闭收托盘、单实例唤醒、开机自启、窗口位置与原生亮色标题栏。
- **贴边宠物**：屏幕边缘常驻一枚小虫标签，悬停展开成宠物卡片，点一下直接带入剪贴板截图 / 文本；按住卡片可拖到屏幕任意一边，位置会记住，也可在设置中随时关闭。
- **本地优先**：SQLite + 本地截图目录，无账号、无遥测、无云端同步。

## 快速开始

### 环境要求

- Windows 10 / 11（需要 WebView2 Runtime）
- Go 1.27+
- Node.js 20+
- Wails 3 CLI（`v3.0.0-beta.34`）

### 开发运行

```bash
git clone https://github.com/lybxkl/remember-fix.git
cd remember-fix

go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.34

cd frontend
npm install
cd ..

wails3 dev
```

### 生产构建

```bash
wails3 build
```

可执行文件输出到 `bin/remember-fix.exe`。

### 测试

```bash
go test ./...
cd frontend && npm run build
```

前端构建会执行 TypeScript 类型检查、Vite 打包和 bundle 体积检查。

## 使用方式

1. 用 `Win + Shift + S` 截取问题画面，或复制一段报错日志。
2. 按 `Ctrl + Alt + R`（默认热键，可在设置中修改），或点击托盘图标；也可以把鼠标移到屏幕边缘的小虫标签，点开贴边宠物。
3. 在捕获窗补一句话 / 描述；标题留空时由 AI 生成。
4. 点击“记下来”或按 `Ctrl + Enter`。AI 在后台补全信息，卡片会实时刷新。
5. 在详情抽屉里完善记录、生成修复指令，或把卡片拖到对应状态。

### 贴边宠物

鼠标移到屏幕边缘的琥珀色小虫标签上，它会展开成一张宠物卡片；点卡片或按提示热键即可记录当前剪贴板内容，捕获窗打开后宠物自动收起。按住卡片拖动，松手会自动吸附到最近的屏幕边缘，下次启动仍在原位置。设置 → 桌面贴边 可以关闭或重新开启。

![贴边宠物](docs/screenshot-edge-pet.png)

## AI 配置

在“设置 → AI 分析”中填写端点、API Key 和模型。

- 默认端点：`https://token-plan.cn-beijing.maas.aliyuncs.com/apps/anthropic`
- 默认模型：`qwen3.8-max`（支持看图）
- 协议：Anthropic Messages（`POST /v1/messages`，`x-api-key` + `anthropic-version: 2023-06-01`）
- 该端点实测可用：`qwen3.8-max` / `qwen3.7-plus`（均支持图片输入）
- 也可以填写其他兼容 Anthropic Messages 的服务端点

API Key 仅保存在本机数据库中，并只会发送到你配置的 AI 端点，不会进入源码、构建产物或本仓库。

> 当前会把 API Key 以明文形式保存在本机 SQLite 中，后续计划接入 Windows DPAPI 加密。

## 数据与隐私

所有数据默认保存在：

```text
%APPDATA%\remember-fix\
├── remember-fix.db      # SQLite 数据库（记录、设置、API Key）
├── shots\               # 截图原图
├── exports\             # Markdown 导出
└── remember-fix.log     # 运行日志
```

- 没有账号体系，没有遥测，没有第三方统计。
- 只有保存新记录或手动触发时，才会把该记录的文本 / 截图发送到你配置的 AI 端点。
- 关闭 AI 后应用可完全离线使用。
- 删除数据目录即可清空全部本地数据。

## 技术栈

| 层 | 技术 |
|---|---|
| 桌面外壳 | Wails v3 beta.34（多窗口、托盘、全局热键、单实例） |
| 后端 | Go 1.27、`database/sql`、`modernc.org/sqlite`（无 CGO） |
| 剪贴板 | Win32 `GetClipboardSequenceNumber`、PNG / DIBV5 / DIB 解析 |
| AI | Anthropic Messages 兼容客户端、图片 + 文本输入、严格 JSON 提取 |
| 前端 | Vue 3、TypeScript、Vite、Element Plus、TipTap 3、lucide-vue-next、markdown-it |
| 存储 | SQLite（WAL） |
| 导出 | Markdown + 本地截图副本 |

## 项目结构

```text
remember-fix/
├── main.go                    # 应用组装：窗口、托盘、热键、单实例、资源路由
├── services.go                # Wails 绑定：Capture / Bug / Config / Edge / AppShell
├── internal/
│   ├── ai/                    # Anthropic 客户端、分析 prompt、JSON 解析
│   ├── autostart/             # HKCU Run 开机自启
│   ├── capture/               # 剪贴板快照、序列号去重、历史
│   ├── clipboard/             # Win32 剪贴板与 DIB → PNG
│   ├── exporter/              # Markdown 导出与截图复制
│   ├── fsx/                   # 数据目录、路径校验、截图落盘
│   ├── models/                # 领域模型
│   └── store/                 # SQLite、设置、CRUD、AI 状态
├── frontend/
│   ├── index.html             # 主窗口（看板）
│   ├── capture.html           # 捕获窗
│   ├── edge.html              # 贴边宠物窗
│   └── src/
│       ├── apps/              # BoardApp / CaptureApp / EdgePetApp
│       ├── components/        # 看板、抽屉、设置、编辑器、空状态
│       ├── composables/       # 数据加载、设置、捕获
│       └── lib/               # 类型、常量、Wails 事件、Markdown
├── build/                     # 图标、Windows manifest、打包配置
├── docs/                      # 设计说明、截图
└── Taskfile.yml
```

详细设计见 [docs/design.md](docs/design.md)。

## 实现要点

- 剪贴板图片优先读取截图工具写入的原始 `PNG`，再回退 `CF_DIBV5` / `CF_DIB`，兼容 24/32bpp、bottom-up、alpha 全零等情况。
- 通过剪贴板序列号去重，同一张截图不会重复落盘。
- AI 分析异步执行并按记录去重；失败只更新状态，不覆盖上一次成功结果。
- Markdown 导出按状态分组，并把截图复制成相对路径，导出目录可整体移动。
- 贴边宠物是一块透明、置顶、不进任务栏的窄窗口；悬停时按 DPI 读取实际窗口尺寸并重新贴住屏幕边缘，点击后复用统一的捕获入口；拖动松手后吸附最近边缘，并把贴边方向与位置写入设置。
- 应用数据只在本地；仓库不包含运行时数据库、日志、截图、导出或可执行文件。

## 已知说明

- 当前仅面向 Windows。
- AI 分析与修复指令生成依赖网络；使用 `qwen3.8-max` 时，修复指令生成通常需要 1–2 分钟。
- `go vet` 对 Win32 剪贴板指针助手有一个已知 `unsafe.Pointer` 误报，不影响构建和运行。

## License

[Apache-2.0](LICENSE)

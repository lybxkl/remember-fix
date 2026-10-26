package main

import (
	"embed"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/lybxkl/remember-fix/internal/capture"
	"github.com/lybxkl/remember-fix/internal/clipboard"
	"github.com/lybxkl/remember-fix/internal/fsx"
	"github.com/lybxkl/remember-fix/internal/store"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

//go:embed build/tray.png
var trayIcon []byte

func main() {
	dataDir, err := fsx.DataDir()
	if err != nil {
		log.Fatal(err)
	}
	logger, logFile := setupLogging(dataDir)
	if logFile != nil {
		defer logFile.Close()
	}

	st, err := store.Open(dataDir)
	if err != nil {
		log.Fatal(err)
	}

	shell := &AppShell{dataDir: dataDir, edgeStore: st}
	shell.initEdgePosition(st)
	capturer := capture.New(dataDir, clipboard.New(), func(snap capture.Snapshot) {
		emit("capture:update", snap)
	})
	capSvc := &CaptureService{capturer: capturer, shell: shell}
	shell.trigger = capSvc.Trigger
	bugSvc := &BugService{store: st, dataDir: dataDir}
	cfgSvc := &ConfigService{store: st, shell: shell}
	edgeSvc := &EdgeService{shell: shell, store: st}

	var browserArgs []string
	if os.Getenv("REMEMBER_FIX_DEBUG") == "1" {
		browserArgs = append(browserArgs, "--remote-debugging-port=9222")
	}

	app := application.New(application.Options{
		Name:        "记得修",
		Description: "截屏记录 Bug，看板管理，AI 自动分类",
		Icon:        appIcon,
		Logger:      logger,
		LogLevel:    slog.LevelInfo,
		Services: []application.Service{
			application.NewService(capSvc),
			application.NewService(bugSvc),
			application.NewService(cfgSvc),
			application.NewService(edgeSvc),
		},
		Assets: application.AssetOptions{
			Handler: assetHandler(assets, dataDir),
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
			AdditionalBrowserArgs:         browserArgs,
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.rememberfix.desktop",
			ExitCode: 0,
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				shell.ShowMain()
			},
		},
		OnShutdown: func() {
			capturer.Stop()
			_ = st.Close()
		},
	})
	shell.app = app

	setupWindows(app, shell)
	setupTray(app, capSvc, shell)
	if runtime.GOOS == "darwin" {
		setupMacMenu(app, capSvc, shell)
		// 点击 Dock 图标时把主窗重新叫出来（macOS 的 reopen 事件）。
		// Wails 默认会显示所有隐藏窗口，连捕获窗和已关闭的贴边窗也会一起弹出；
		// 取消默认行为，只恢复主窗，捕获窗本身就开着时则聚焦它。
		app.Event.RegisterApplicationEventHook(events.Mac.ApplicationShouldHandleReopen, func(e *application.ApplicationEvent) {
			e.Cancel()
			shell.ShowMain()
			if shell.captureWin != nil && shell.captureWin.IsVisible() {
				shell.captureWin.Focus()
			}
		})
	}

	// 等贴边窗口的 WebView 运行时就绪后再显示；过早 Show 会被 Windows 忽略。
	shell.edgeWin.RegisterHook(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		if st.GetSetting("edge.enabled") != "false" {
			shell.SetEdgeEnabled(true)
		}
	})

	hotkey := st.GetSetting("hotkey")
	if hotkey == "" {
		hotkey = store.DefaultHotkey
	}
	if err := shell.registerHotkey(hotkey); err != nil {
		slog.Warn("全局热键注册失败，可稍后在设置里修改", "hotkey", hotkey, "err", err)
	}

	capturer.Start()
	go func() {
		st.CleanupOrphanShots(dataDir)
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			st.CleanupOrphanShots(dataDir)
		}
	}()

	slog.Info("记得修已启动", "dataDir", dataDir)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func setupWindows(app *application.App, shell *AppShell) {
	mainWin := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            "记得修 · Bug 看板",
		Width:            1200,
		Height:           720,
		MinWidth:         980,
		MinHeight:        600,
		URL:              "/",
		BackgroundColour: application.NewRGB(246, 243, 236),
		Windows: application.WindowsWindow{
			Theme: application.Light,
			CustomTheme: application.ThemeSettings{
				LightModeActive: &application.WindowTheme{
					TitleBarColour:  application.NewRGBPtr(246, 243, 236),
					TitleTextColour: application.NewRGBPtr(35, 32, 26),
					BorderColour:    application.NewRGBPtr(221, 212, 197),
				},
				LightModeInactive: &application.WindowTheme{
					TitleBarColour:  application.NewRGBPtr(246, 243, 236),
					TitleTextColour: application.NewRGBPtr(138, 131, 119),
					BorderColour:    application.NewRGBPtr(221, 212, 197),
				},
			},
		},
		Mac: application.MacWindow{
			Appearance: application.NSAppearanceNameAqua,
		},
	})
	shell.mainWin = mainWin
	mainWin.Center()
	// 关闭主窗口 = 隐藏到托盘，退出走托盘菜单。
	mainWin.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if shell.quitting.Load() {
			return
		}
		mainWin.Hide()
		e.Cancel()
	})

	captureWin := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "capture",
		Title:            "记一条 Bug",
		Width:            500,
		Height:           680,
		MinWidth:         460,
		MinHeight:        600,
		URL:              "/capture.html",
		Frameless:        true,
		AlwaysOnTop:      true,
		Hidden:           true,
		HideOnEscape:     true,
		BackgroundColour: application.NewRGB(246, 243, 236),
		Windows: application.WindowsWindow{
			HiddenOnTaskbar: true,
			Theme:           application.Light,
			CustomTheme: application.ThemeSettings{
				LightModeActive: &application.WindowTheme{
					TitleBarColour:  application.NewRGBPtr(246, 243, 236),
					TitleTextColour: application.NewRGBPtr(35, 32, 26),
					BorderColour:    application.NewRGBPtr(221, 212, 197),
				},
			},
		},
		Mac: application.MacWindow{
			Appearance: application.NSAppearanceNameAqua,
			CollectionBehavior: application.MacWindowCollectionBehaviorCanJoinAllSpaces |
				application.MacWindowCollectionBehaviorFullScreenAuxiliary,
		},
	})
	shell.captureWin = captureWin
	// Alt+F4 也不销毁捕获窗，保证下一次热键仍可用。
	captureWin.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if shell.quitting.Load() {
			return
		}
		captureWin.Hide()
		e.Cancel()
	})

	edgeWidth, edgeHeight := edgeWindowSize(shell.edgeDock, false)
	edgeWin := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "edge",
		Title:            "记得修 · 贴边",
		Width:            edgeWidth,
		Height:           edgeHeight,
		Frameless:        true,
		AlwaysOnTop:      true,
		Hidden:           true,
		DisableResize:    true,
		BackgroundType:   application.BackgroundTypeTransparent,
		BackgroundColour: application.NewRGBA(0, 0, 0, 0),
		URL:              "/edge.html",
		Windows: application.WindowsWindow{
			HiddenOnTaskbar: true,
			Theme:           application.Light,
			// 贴边窗不抢焦点，否则窗口未激活时第一次点击会被系统用于激活并吞掉，
			// 导致按下后既不移动也不打开捕获窗。这里保留 Wails 默认的透明/置顶/工具窗标志。
			ExStyle:                           0x08210188, // CONTROLPARENT|NOREDIRECTIONBITMAP|WINDOWEDGE|TOOLWINDOW|TOPMOST|NOACTIVATE
			DisableFramelessWindowDecorations: true,
		},
		Mac: application.MacWindow{
			Backdrop:      application.MacBackdropTransparent,
			Appearance:    application.NSAppearanceNameAqua,
			CornerType:    application.MacWindowCornerTypeSquare,
			DisableShadow: true,
			TabbingMode:   application.MacWindowTabbingModeDisallowed,
			// 非激活面板：不抢当前应用焦点，首次按下也不会被用于激活窗口而吞掉。
			WindowClass: application.MacWindowClassPanel,
			PanelPreferences: application.MacPanelPreferences{
				NonActivating:          true,
				FloatingPanel:          true,
				BecomesKeyOnlyIfNeeded: true,
			},
			CollectionBehavior: application.MacWindowCollectionBehaviorCanJoinAllSpaces |
				application.MacWindowCollectionBehaviorFullScreenAuxiliary |
				application.MacWindowCollectionBehaviorIgnoresCycle,
		},
	})
	shell.edgeWin = edgeWin
	edgeWin.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if shell.quitting.Load() {
			return
		}
		edgeWin.Hide()
		e.Cancel()
	})
	// 原生拖窗结束后吸附。Windows 有明确的结束事件；macOS 由前端 mouseup 触发，
	// 这里再用移动静默期兜底（见 noteEdgeMoved）。
	edgeWin.RegisterHook(events.Common.WindowDidMove, func(*application.WindowEvent) {
		shell.noteEdgeMoved()
	})
	if runtime.GOOS == "windows" {
		onEndMove := func(*application.WindowEvent) {
			shell.finishEdgeDrag()
		}
		edgeWin.RegisterHook(events.Windows.WindowStartMove, func(*application.WindowEvent) {
			shell.beginEdgeDrag()
		})
		edgeWin.RegisterHook(events.Windows.WindowEndMove, onEndMove)
		edgeWin.RegisterHook(events.Windows.WindowEndResize, onEndMove)
	}
}

func setupTray(app *application.App, capSvc *CaptureService, shell *AppShell) {
	tray := app.SystemTray.New()
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(trayIcon)
	} else {
		tray.SetIcon(trayIcon)
	}
	tray.SetTooltip("记得修 · 截屏记 Bug")
	tray.OnClick(func() {
		capSvc.Trigger()
	})
	menu := app.NewMenu()
	menu.Add("记一条 Bug").OnClick(func(*application.Context) {
		capSvc.Trigger()
	})
	menu.Add("打开看板").OnClick(func(*application.Context) {
		shell.ShowMain()
	})
	menu.Add("设置").OnClick(func(*application.Context) {
		shell.ShowMain()
		emit("ui:open-settings", nil)
	})
	menu.AddSeparator()
	menu.Add("退出").OnClick(func(*application.Context) {
		shell.Quit()
	})
	tray.SetMenu(menu)
}

// setupMacMenu 提供 macOS 标准的 App / 文件 / 编辑 / 显示 / 窗口菜单。
// 没有 Edit 菜单时，WebView 里的 Cmd+C / Cmd+V 等标准编辑快捷键不可用。
func setupMacMenu(app *application.App, capSvc *CaptureService, shell *AppShell) {
	menu := app.NewMenu()
	menu.AddRole(application.AppMenu)
	menu.AddRole(application.FileMenu)
	menu.AddRole(application.EditMenu)
	menu.AddRole(application.ViewMenu)
	menu.AddRole(application.WindowMenu)

	record := menu.AddSubmenu("记录")
	record.Add("记一条 Bug").OnClick(func(*application.Context) {
		capSvc.Trigger()
	})
	record.Add("打开看板").OnClick(func(*application.Context) {
		shell.ShowMain()
	})
	record.Add("设置…").OnClick(func(*application.Context) {
		shell.ShowMain()
		emit("ui:open-settings", nil)
	})
	app.Menu.Set(menu)
}

func emit(name string, data any) {
	if a := application.Get(); a != nil {
		a.Event.Emit(name, data)
	}
}

// assetHandler: /shots/** 只暴露截图目录中的图片，其余走打包的前端静态资源。
// 不能把 dataDir 当作根目录，否则数据库（含 API Key）和日志也会被该路由读到。
func assetHandler(assets embed.FS, dataDir string) http.Handler {
	static := application.BundledAssetFileServer(assets)
	shotsDir := filepath.Join(dataDir, "shots")
	mux := http.NewServeMux()
	mux.Handle("/shots/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(r.URL.Path, "/shots/")
		if rel == "" || strings.Contains(rel, "..") || strings.ContainsRune(rel, '\\') {
			http.NotFound(w, r)
			return
		}
		// 数据库里保存的是 shots/年/月/xxx.png，前端会在前面再加一次 /shots/。
		rel = strings.TrimPrefix(rel, "shots/")
		if rel == "" {
			http.NotFound(w, r)
			return
		}
		switch strings.ToLower(path.Ext(rel)) {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		default:
			http.NotFound(w, r)
			return
		}
		abs := fsx.AbsPath(shotsDir, rel)
		if abs == "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "max-age=86400")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeFile(w, r, abs)
	}))
	mux.Handle("/", static)
	return mux
}

func setupLogging(dataDir string) (*slog.Logger, *os.File) {
	w := os.Stderr
	f, err := os.OpenFile(filepath.Join(dataDir, "remember-fix.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		// GUI 进程的 stderr 可能不可用，只写日志文件，避免 MultiWriter 提前中断。
		w = f
	}
	logger := slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	return logger, f
}

package main

import (
	"embed"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
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

	shell := &AppShell{dataDir: dataDir}
	capturer := capture.New(dataDir, clipboard.NewWindowsClipboard(), func(snap capture.Snapshot) {
		emit("capture:update", snap)
	})
	capSvc := &CaptureService{capturer: capturer, shell: shell}
	shell.trigger = capSvc.Trigger
	bugSvc := &BugService{store: st, dataDir: dataDir}
	cfgSvc := &ConfigService{store: st, shell: shell}

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
}

func setupTray(app *application.App, capSvc *CaptureService, shell *AppShell) {
	tray := app.SystemTray.New()
	tray.SetIcon(trayIcon)
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

func emit(name string, data any) {
	if a := application.Get(); a != nil {
		a.Event.Emit(name, data)
	}
}

// assetHandler: /shots/** 路由到数据目录截图，其余走打包的前端静态资源。
func assetHandler(assets embed.FS, dataDir string) http.Handler {
	static := application.BundledAssetFileServer(assets)
	mux := http.NewServeMux()
	mux.Handle("/shots/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(r.URL.Path, "/shots/")
		if rel == "" || strings.Contains(rel, "..") || strings.ContainsRune(rel, '\\') {
			http.NotFound(w, r)
			return
		}
		abs := fsx.AbsPath(dataDir, rel)
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

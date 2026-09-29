//go:build windows

package main

// Windows 桌面 GUI：WebView2 原生窗口加载内嵌控制台页面。
// 页面与后端只通过 127.0.0.1 随机端口的回环 HTTP 通信——
// 手机/局域网无法访问 GUI 接口，配置能力物理隔离在 PC 本机。

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	webview2 "github.com/jchv/go-webview2"

	"hestia/internal/config"
)

// runGUI 在主线程运行 WebView2 窗口（阻塞直至窗口关闭）。
func runGUI(a *app) {
	runtime.LockOSThread()
	if exe, err := os.Executable(); err == nil {
		if message, err := os.ReadFile(filepath.Join(updateDir(exe), "last-error.txt")); err == nil {
			guiUpdater.fail(fmt.Errorf("上次更新失败：%s", strings.TrimSpace(string(message))))
		}
		go cleanupUpdateStages(exe)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		a.logger.Printf("GUI 初始化失败: %v", err)
		a.shutdown()
		return
	}
	var w webview2.WebView
	srv := &http.Server{Handler: buildGUIMux(a, func() {
		if w != nil {
			w.Dispatch(func() { w.Terminate() })
		}
	}), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		shutdownCtx := make(chan struct{})
		go func() { _ = srv.Close(); close(shutdownCtx) }()
		select {
		case <-shutdownCtx:
		case <-time.After(2 * time.Second):
		}
	}()

	// WebView2 的用户数据也固定在 exe 同目录，不在系统盘留任何文件
	w = webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		DataPath:  filepath.Join(a.dataDir, "webview"),
	})
	defer w.Destroy()
	w.SetTitle("Hestia 控制台")
	w.SetSize(600, 760, webview2.HintNone)
	setupWindowChrome(syscall.Handle(w.Window()))
	// 防白屏：窗口先隐藏，页面渲染完成（guiReady）后再显示
	hideMainWindow()
	defer removeTray()
	w.Bind("guiReady", func() { showGuiWindow() })
	// 兜底：页面异常时 5 秒后强制显示，避免永远无窗
	go func() {
		time.Sleep(5 * time.Second)
		w.Dispatch(func() { showGuiWindow() })
	}()
	w.Navigate("http://" + ln.Addr().String() + "/")
	a.logger.Print("GUI 窗口已就绪")
	w.Run()
	a.shutdown()
}

func buildGUIMux(a *app, quit func()) *http.ServeMux {
	mux := http.NewServeMux()

	// 内嵌控制台页面（web/gui）
	if guiFS, err := fs.Sub(webFiles, "web/gui"); err == nil {
		mux.Handle("/", http.FileServerFS(guiFS))
	}

	jwt := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(v)
	}
	jerr := func(w http.ResponseWriter, msg string) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
	}
	body := func(w http.ResponseWriter, r *http.Request, dst any) bool {
		if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
			jerr(w, "请求格式错误")
			return false
		}
		return true
	}

	mux.HandleFunc("/gui/api/state", func(w http.ResponseWriter, r *http.Request) {
		jwt(w, guiState(a))
	})
	// Update operations stay on the desktop-only listener and require a same-origin POST.
	updateRequest := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodPost || r.Header.Get("Origin") != "http://"+r.Host {
			http.Error(w, "forbidden", http.StatusForbidden)
			return false
		}
		return true
	}
	mux.HandleFunc("/gui/api/update/check", func(w http.ResponseWriter, r *http.Request) {
		if !updateRequest(w, r) {
			return
		}
		if _, err := guiUpdater.check(r.Context()); err != nil {
			jerr(w, err.Error())
			return
		}
		jwt(w, guiUpdater.snapshot())
	})
	mux.HandleFunc("/gui/api/update/install", func(w http.ResponseWriter, r *http.Request) {
		if !updateRequest(w, r) {
			return
		}
		if err := guiUpdater.start(quit); err != nil {
			jerr(w, err.Error())
			return
		}
		jwt(w, guiUpdater.snapshot())
	})
	mux.HandleFunc("/gui/api/toggle-service", func(w http.ResponseWriter, r *http.Request) {
		if a.srv.Running() {
			a.srv.StopListener()
			a.logger.Print("GUI：服务已停止")
		} else {
			if err := a.srv.StartListener(); err != nil {
				jerr(w, err.Error())
				return
			}
			a.logger.Print("GUI：服务已启动")
		}
		jwt(w, guiState(a))
	})
	mux.HandleFunc("/gui/api/add", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Path string `json:"path"`
		}
		if !body(w, r, &b) || b.Path == "" {
			jerr(w, "请填写路径")
			return
		}
		abs := config.AbsPath(b.Path)
		fi, err := os.Stat(abs)
		if err != nil || !fi.IsDir() {
			jerr(w, "路径不存在或不是文件夹："+abs)
			return
		}
		err = a.cfgM.Update(func(c *config.Config) {
			for _, l := range c.Libraries {
				if equalFoldPath(config.AbsPath(l.Path), abs) {
					return
				}
			}
			c.Libraries = append(c.Libraries, config.Library{Path: abs, Label: fi.Name(), Enabled: true})
		})
		if err != nil {
			jerr(w, err.Error())
			return
		}
		jwt(w, guiState(a))
	})
	mux.HandleFunc("/gui/api/remove", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			ID string `json:"id"`
		}
		if !body(w, r, &b) {
			return
		}
		path := libPathByID(a, b.ID)
		if path == "" {
			jerr(w, "媒体库不存在")
			return
		}
		_ = a.cfgM.Update(func(c *config.Config) {
			out := c.Libraries[:0]
			for _, l := range c.Libraries {
				if config.AbsPath(l.Path) != path {
					out = append(out, l)
				}
			}
			c.Libraries = out
		})
		jwt(w, guiState(a))
	})
	mux.HandleFunc("/gui/api/toggle", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			ID      string `json:"id"`
			Enabled bool   `json:"enabled"`
		}
		if !body(w, r, &b) {
			return
		}
		path := libPathByID(a, b.ID)
		if path == "" {
			jerr(w, "媒体库不存在")
			return
		}
		_ = a.cfgM.Update(func(c *config.Config) {
			for i := range c.Libraries {
				if config.AbsPath(c.Libraries[i].Path) == path {
					c.Libraries[i].Enabled = b.Enabled
				}
			}
		})
		jwt(w, guiState(a))
	})
	mux.HandleFunc("/gui/api/rescan", func(w http.ResponseWriter, r *http.Request) {
		go a.store.RescanAll()
		jwt(w, guiState(a))
	})
	mux.HandleFunc("/gui/api/port", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Port int `json:"port"`
		}
		if !body(w, r, &b) || b.Port < 1 || b.Port > 65535 {
			jerr(w, "端口取值 1-65535")
			return
		}
		_ = a.cfgM.Update(func(c *config.Config) { c.Port = b.Port })
		if a.srv.Running() {
			if err := a.srv.StartListener(); err != nil {
				jerr(w, err.Error())
				return
			}
		}
		jwt(w, guiState(a))
	})
	mux.HandleFunc("/gui/api/autostart", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Enabled bool `json:"enabled"`
		}
		if !body(w, r, &b) {
			return
		}
		_ = a.cfgM.Update(func(c *config.Config) { c.AutoStart = b.Enabled })
		jwt(w, guiState(a))
	})
	mux.HandleFunc("/gui/api/pickdir", func(w http.ResponseWriter, r *http.Request) {
		jwt(w, map[string]any{"path": pickFolderDialog("选择媒体库文件夹")})
	})
	mux.HandleFunc("/gui/api/open", func(w http.ResponseWriter, r *http.Request) {
		if a.srv.Running() {
			openBrowser(a.srv.URLs()[0])
		}
		jwt(w, guiState(a))
	})
	mux.HandleFunc("/gui/api/openlog", func(w http.ResponseWriter, r *http.Request) {
		logPath := filepath.Join(a.dataDir, "logs", "hestia.log")
		if _, err := os.Stat(logPath); err != nil { // 还没有日志文件时先落一条，保证能打开
			a.logger.Print("查看日志")
		}
		shellOpen(logPath)
		jwt(w, guiState(a))
	})
	return mux
}

func guiState(a *app) map[string]any {
	c := a.cfgM.Get()
	libs := a.store.Libraries()
	folders, media := a.store.Counts()
	libJSON := make([]map[string]any, 0, len(libs))
	for _, l := range libs {
		libJSON = append(libJSON, map[string]any{
			"id": l.ID, "label": l.Label, "path": l.Path,
			"enabled": l.Enabled, "scanning": l.Scanning,
			"files": l.Files, "dirs": l.Dirs, "lastErr": l.LastErr,
		})
	}
	urls := []string{}
	if a.srv.Running() {
		urls = a.srv.URLs()
	}
	return map[string]any{
		"running": a.srv.Running(), "urls": urls,
		"update": guiUpdater.snapshot(),
		"port":   c.Port, "autoStart": c.AutoStart,
		"ffmpeg": a.tc.FFmpegOK(), "ffmpegPath": a.tc.FFmpeg,
		"scanning": a.store.AnyScanning(), "folders": folders, "media": media,
		"libs": libJSON,
	}
}

func libPathByID(a *app, id string) string {
	for _, l := range a.store.Libraries() {
		if l.ID == id {
			return config.AbsPath(l.Path)
		}
	}
	return ""
}

// equalFoldPath 路径比较（Windows 盘符大小写不敏感）。
func equalFoldPath(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

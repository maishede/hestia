package main

import (
	"embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/getlantern/systray"

	"hestia/internal/config"
	"hestia/internal/index"
	"hestia/internal/progress"
	"hestia/internal/server"
)

//go:embed all:web
var webFiles embed.FS

// mustWebFS 把 embed 根下的 web/ 剥成站点根（index.html、assets/…）。
func mustWebFS() fs.FS {
	sub, err := fs.Sub(webFiles, "web")
	if err != nil {
		fatal(log.New(os.Stderr, "", 0), "内嵌前端缺失: %v", err)
	}
	return sub
}

type app struct {
	cfgM   *config.Manager
	store  *index.Store
	tc     server.Toolchain
	tr     *server.Transcoder
	prog   *progress.Store
	srv    *server.Server
	logger *log.Logger
	home   string

	appliedEnabled  map[string]bool
	appliedPort     int
	appliedListen   string
	appliedAuto     *bool
	first           bool
	periodicStopped chan struct{}
}

func main() {
	hide := flag.Bool("hide", false, "启动时隐藏控制台窗口（配合托盘，仅 Windows）")
	flag.Parse()

	home := os.Getenv("HESTIA_HOME")
	if home == "" {
		if exe, err := os.Executable(); err == nil {
			home = filepath.Dir(exe)
		} else {
			home = "."
		}
	}
	if runtime.GOOS == "windows" { // 控制台切 UTF-8，避免中文乱码
		_ = exec.Command("cmd", "/c", "chcp", "65001", ">NUL").Run()
	}

	dataDir := filepath.Join(home, "data")
	logger := newRotatingLogger(dataDir)

	cfgM, err := config.Load(filepath.Join(home, "config.json"))
	if err != nil {
		fatal(logger, "加载配置失败: %v", err)
	}
	store, err := index.New(dataDir)
	if err != nil {
		fatal(logger, "初始化索引失败: %v", err)
	}
	prog, err := progress.New(dataDir)
	if err != nil {
		fatal(logger, "初始化进度存储失败: %v", err)
	}
	tc := server.DetectToolchain()
	tr := server.NewTranscoder(tc.FFmpeg, dataDir, func(mediaID string) (string, error) {
		abs, _, err := store.ResolveMedia(mediaID)
		return abs, err
	})
	srv, err := server.New(cfgM, store, tc, tr, prog, dataDir, mustWebFS(), logger)
	if err != nil {
		fatal(logger, "初始化服务失败: %v", err)
	}

	a := &app{
		cfgM: cfgM, store: store, tc: tc, tr: tr, prog: prog, srv: srv,
		logger: logger, home: home,
		appliedEnabled:  map[string]bool{},
		appliedPort:     -1,
		appliedListen:   "",
		periodicStopped: make(chan struct{}),
	}

	useTray := cfgM.Get().UseTray() && runtime.GOOS == "windows"
	if useTray && *hide {
		hideConsole(true)
	}

	if err := a.serve(); err != nil {
		fatal(logger, "%v", err)
	}

	if useTray {
		go a.waitSignals(systray.Quit)
		systray.Run(a.onTrayReady, a.onTrayExit) // 阻塞至托盘退出
		return
	}
	a.waitSignals(nil)
	a.shutdown()
}

func fatal(l *log.Logger, format string, v ...any) {
	l.Printf("致命错误: "+format, v...)
	os.Exit(1)
}

// serve 启动监听、应用配置、周期重扫并打印横幅。
func (a *app) serve() error {
	if err := a.srv.ListenAndServe(); err != nil {
		return err
	}
	// 端口被占时自动 +1，回写避免每次漂移
	if actual := a.srv.Port(); actual != a.cfgM.Get().Port {
		_ = a.cfgM.Update(func(c *config.Config) { c.Port = actual })
	}
	a.apply(a.cfgM.Get())
	a.cfgM.OnChange(a.apply)

	go func() {
		t := time.NewTicker(15 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-a.periodicStopped:
				return
			case <-t.C:
				if !a.store.AnyScanning() {
					a.store.RescanAll()
				}
			}
		}
	}()

	a.banner()
	if a.cfgM.Get().OpenBrowser {
		go func() {
			time.Sleep(600 * time.Millisecond)
			openBrowser(a.srv.URLs()[0])
		}()
	}
	return nil
}

// waitSignals 等待退出信号；onQuit 用于托盘模式下优雅触发退出。
func (a *app) waitSignals(onQuit func()) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	fmt.Println("\n正在退出…")
	if onQuit != nil {
		onQuit() // systray.Quit → onTrayExit → shutdown
		return
	}
	a.shutdown()
}

func (a *app) shutdown() {
	close(a.periodicStopped)
	a.tr.Close()
	a.store.Close()
	a.prog.Close()
	a.cfgM.Close()
	a.logger.Print("已退出")
}

// ---- 配置统一应用器：管理页 API 与手改 config.json 走同一生效路径 ----
func (a *app) apply(c config.Config) {
	want := map[string]config.Library{}
	for _, l := range c.Libraries {
		if l.Path == "" {
			continue
		}
		abs := config.AbsPath(l.Path)
		want[a.store.LibID(abs)] = config.Library{Path: abs, Label: l.Label, Enabled: l.Enabled}
	}
	for _, cur := range a.store.Libraries() {
		if _, ok := want[cur.ID]; !ok {
			a.store.RemoveLibrary(cur.ID)
			delete(a.appliedEnabled, cur.ID)
			a.logger.Printf("已移除媒体库: %s", cur.Path)
		}
	}
	for id, w := range want {
		_, exists := a.store.GetLibrary(id)
		if !exists {
			if _, err := a.store.AddLibrary(w.Path, w.Label); err != nil {
				a.logger.Printf("添加媒体库失败 %s: %v", w.Path, err)
				continue
			}
			a.logger.Printf("新增媒体库: %s（后台扫描中…）", w.Path)
			go a.store.RescanLibrary(id)
			a.appliedEnabled[id] = w.Enabled
			continue
		}
		a.store.SetLibraryLabel(id, w.Label)
		if prev, ok := a.appliedEnabled[id]; ok && prev != w.Enabled {
			a.store.SetLibraryEnabled(id, w.Enabled)
			if w.Enabled {
				a.logger.Printf("媒体库已启用: %s（重新扫描）", w.Path)
				go a.store.RescanLibrary(id)
			} else {
				a.logger.Printf("媒体库已停用: %s", w.Path)
			}
		}
		a.appliedEnabled[id] = w.Enabled
	}
	if !a.first && (a.appliedPort != c.Port || a.appliedListen != c.Listen) {
		if err := a.srv.RestartListener(c.Listen, c.Port); err != nil {
			a.logger.Printf("端口切换失败，继续使用 %d: %v", a.srv.Port(), err)
		} else {
			a.logger.Printf("监听已切换: %s:%d", c.Listen, c.Port)
		}
	}
	a.appliedPort, a.appliedListen = c.Port, c.Listen
	// 开机自启（仅 Windows 注册表）
	if a.appliedAuto == nil || *a.appliedAuto != c.AutoStart {
		if err := setAutoStart(c.AutoStart); err != nil {
			a.logger.Printf("开机自启设置失败: %v", err)
		} else if a.appliedAuto != nil {
			a.logger.Printf("开机自启: %v", c.AutoStart)
		}
		auto := c.AutoStart
		a.appliedAuto = &auto
	}
	a.first = false
}

func (a *app) banner() {
	ff := "未检测到（HEVC/MKV 等格式将无法转码兜底，建议放到程序同目录或加入 PATH）"
	if a.tc.FFmpegOK() {
		ff = "已就绪 " + a.tc.FFmpeg
	}
	var b strings.Builder
	b.WriteString("\n┌─────────────────────────────────────────────┐\n")
	b.WriteString("│  Hestia 家庭影视服务器 v" + server.Version + strings.Repeat(" ", max(0, 26-len(server.Version))) + "│\n")
	for _, u := range a.srv.URLs() {
		pad := max(0, 43-len(u))
		b.WriteString("│  " + u + strings.Repeat(" ", 1+pad) + "│\n")
	}
	b.WriteString("│  ffmpeg: " + ff + "\n")
	b.WriteString("│  配置: " + filepath.Join(a.home, "config.json") + "\n")
	b.WriteString("│  手机浏览器输入上方地址即可观看              │\n")
	b.WriteString("└─────────────────────────────────────────────┘")
	a.logger.Print(b.String())
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

// ---- 滚动日志 ----

func newRotatingLogger(dataDir string) *log.Logger {
	return log.New(io.MultiWriter(os.Stdout, &rotatingWriter{
		dir: filepath.Join(dataDir, "logs"), name: "hestia.log",
		maxBytes: 5 << 20, keep: 3,
	}), "", log.Ltime)
}

type rotatingWriter struct {
	dir      string
	name     string
	maxBytes int64
	keep     int

	mu   sync.Mutex
	f    *os.File
	size int64
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		w.open()
	}
	if w.f == nil {
		return len(p), nil // 日志文件不可用时仅输出控制台
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	if w.size > w.maxBytes {
		w.rotate()
	}
	return n, err
}

func (w *rotatingWriter) path() string { return filepath.Join(w.dir, w.name) }

func (w *rotatingWriter) open() {
	_ = os.MkdirAll(w.dir, 0o755)
	f, err := os.OpenFile(w.path(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	w.f = f
	if fi, err := f.Stat(); err == nil {
		w.size = fi.Size()
	}
}

func (w *rotatingWriter) rotate() {
	if w.f != nil {
		_ = w.f.Close()
		w.f = nil
	}
	for i := w.keep - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", w.path(), i), fmt.Sprintf("%s.%d", w.path(), i+1))
	}
	_ = os.Rename(w.path(), w.path()+".1")
	w.open()
}

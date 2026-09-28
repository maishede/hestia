// Package server 提供 HTTP 服务：REST API、直链流媒体、转码、内嵌前端静态资源。
package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"hestia/internal/config"
	"hestia/internal/index"
	"hestia/internal/progress"
)

const Version = "0.3.2"

type Server struct {
	cfg     *config.Manager
	store   *index.Store
	tc      Toolchain
	tr      *Transcoder
	prog    *progress.Store
	dataDir string
	logger  *log.Logger

	mux   *http.ServeMux
	index []byte

	mu     sync.Mutex
	srv    *http.Server
	port   int
	listen string

	subMu sync.Mutex // 串行化 ffmpeg 字幕提取/封面抽帧（家用带宽足够，避免磁盘抖动）
}

func New(cfg *config.Manager, store *index.Store, tc Toolchain, tr *Transcoder, prog *progress.Store, dataDir string, webFS fs.FS, logger *log.Logger) (*Server, error) {
	s := &Server{
		cfg: cfg, store: store, tc: tc, tr: tr, prog: prog, dataDir: dataDir, logger: logger,
		mux: http.NewServeMux(),
	}
	assets, err := fs.Sub(webFS, "assets")
	if err != nil {
		return nil, err
	}
	s.index, err = fs.ReadFile(webFS, "index.html")
	if err != nil {
		return nil, err
	}

	// 用户端 API
	s.mux.HandleFunc("GET /api/server/info", s.handleServerInfo)
	s.mux.HandleFunc("GET /api/folders", s.handleRoots)
	s.mux.HandleFunc("GET /api/folders/{id}/children", s.handleChildren)
	s.mux.HandleFunc("GET /api/search", s.handleSearch)
	s.mux.HandleFunc("GET /api/media/{id}", s.handleMediaDetail)
	s.mux.HandleFunc("GET /api/media/{id}/stream", s.handleStream)
	s.mux.HandleFunc("GET /api/media/{id}/image", s.handleImage)
	s.mux.HandleFunc("POST /api/media/{id}/transcode", s.handleTranscodeStart)
	s.mux.HandleFunc("GET /api/transcode/{sid}/{file}", s.handleTranscodeFile)
	s.mux.HandleFunc("DELETE /api/transcode/{sid}", s.handleTranscodeStop)
	// 字幕与封面
	s.mux.HandleFunc("GET /api/subtitle/{id}", s.handleSubtitle)
	s.mux.HandleFunc("GET /api/media/{id}/embeddedsub/{index}", s.handleEmbeddedSub)
	s.mux.HandleFunc("GET /api/folders/{id}/cover", s.handleFolderCover)
	// 播放进度（多设备同步）
	s.mux.HandleFunc("POST /api/progress", s.handleProgressSet)
	s.mux.HandleFunc("GET /api/progress/recent", s.handleProgressRecent)
	s.mux.HandleFunc("GET /api/progress/{id}", s.handleProgressGet)
	s.mux.HandleFunc("DELETE /api/progress/{id}", s.handleProgressDelete)
	s.mux.HandleFunc("DELETE /api/progress", s.handleProgressClear)
	// 内嵌前端
	s.mux.Handle("GET /assets/", cacheStatic(http.StripPrefix("/assets/", http.FileServerFS(assets))))
	s.mux.HandleFunc("GET /{$}", s.serveIndex)
	return s, nil
}

func cacheStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 内嵌资源无 Last-Modified/ETag 校验器，改为 no-cache 确保升级后浏览器取到新版本
		w.Header().Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(s.index)
}

// ListenAndServe 启动监听；端口被占时自动 +1 重试（最多 20 次）。
func (s *Server) ListenAndServe() error {
	cfg := s.cfg.Get()
	port := cfg.Port
	var ln net.Listener
	var err error
	for i := 0; i < 21; i++ {
		ln, err = net.Listen("tcp", fmt.Sprintf("%s:%d", cfg.Listen, port))
		if err == nil {
			port += i
			break
		}
	}
	if err != nil {
		return fmt.Errorf("无法监听 %s:%d: %w", cfg.Listen, cfg.Port, err)
	}
	s.mu.Lock()
	s.port, s.listen = port, cfg.Listen
	s.srv = &http.Server{Handler: s.mux, ReadHeaderTimeout: 10 * time.Second}
	s.mu.Unlock()
	go func() {
		if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Printf("http 服务异常退出: %v", err)
		}
	}()
	return nil
}

// RestartListener 热切换监听地址/端口：先抢新端口（旧监听不动，失败保持现状），
// 成功后再排空旧连接；目标与当前一致时为无操作。
func (s *Server) RestartListener(listen string, port int) error {
	s.mu.Lock()
	old, curPort, curListen := s.srv, s.port, s.listen
	s.mu.Unlock()
	if old != nil && curPort == port && curListen == listen {
		return nil
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", listen, port))
	if err != nil {
		return fmt.Errorf("端口 %d 不可用: %w", port, err)
	}
	if old != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = old.Shutdown(ctx)
		cancel()
	}
	s.serveOn(ln, listen, port)
	return nil
}

// StartListener 按当前配置启动监听。
func (s *Server) StartListener() error {
	c := s.cfg.Get()
	return s.RestartListener(c.Listen, c.Port)
}

// StopListener 停止监听（进程不退出）。
func (s *Server) StopListener() {
	s.mu.Lock()
	old := s.srv
	s.srv = nil
	s.mu.Unlock()
	if old != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = old.Shutdown(ctx)
		cancel()
	}
}

// Running 报告服务是否在监听。
func (s *Server) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.srv != nil
}

func (s *Server) serveOn(ln net.Listener, listen string, port int) {
	srv := &http.Server{Handler: s.mux, ReadHeaderTimeout: 10 * time.Second}
	s.mu.Lock()
	s.srv, s.port, s.listen = srv, port, listen
	s.mu.Unlock()
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Printf("http 服务异常退出: %v", err)
		}
	}()
}

func (s *Server) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.port
}

func (s *Server) ListenHost() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listen
}

// URLs 返回局域网访问地址（过滤虚拟网卡：WSL/Hyper-V/VMware 等，排除 198.18 基准网段与链路本地地址）。
func (s *Server) URLs() []string {
	s.mu.Lock()
	host, port := s.listen, s.port
	s.mu.Unlock()
	if host != "" && host != "0.0.0.0" && host != "::" {
		return []string{fmt.Sprintf("http://%s:%d", host, port)}
	}
	var out []string
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, ifc := range ifaces {
			if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
				continue
			}
			if isVirtualNIC(ifc.Name) {
				continue
			}
			addrs, _ := ifc.Addrs()
			for _, a := range addrs {
				ipn, ok := a.(*net.IPNet)
				if !ok {
					continue
				}
				ip4 := ipn.IP.To4()
				if ip4 == nil || ip4.IsLoopback() || ip4.IsLinkLocalUnicast() {
					continue
				}
				if ip4[0] == 198 && ip4[1]&0xFE == 18 { // 198.18.0.0/15 基准测试/代理 TUN 网段
					continue
				}
				out = append(out, fmt.Sprintf("http://%s:%d", ip4, port))
			}
		}
	}
	// 常用网段优先：192.168 → 10 → 172.16-31 → 其他
	rank := func(u string) int {
		switch {
		case strings.HasPrefix(u, "http://192.168."):
			return 0
		case strings.HasPrefix(u, "http://10."):
			return 1
		case strings.HasPrefix(u, "http://172."):
			return 2
		}
		return 3
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i]) < rank(out[j]) })
	if len(out) == 0 {
		out = append(out, fmt.Sprintf("http://127.0.0.1:%d", port))
	}
	return out
}

// isVirtualNIC 按网卡名识别虚拟适配器（大小写不敏感）。
func isVirtualNIC(name string) bool {
	n := strings.ToLower(name)
	for _, key := range []string{"loopback", "vethernet", "wsl", "vmware", "virtualbox", "tap-", "bluetooth", "蓝牙"} {
		if strings.Contains(n, key) {
			return true
		}
	}
	return false
}

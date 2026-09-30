// devserver 启动无 GUI 的 Hestia 服务，供开发调试 / 自动化测试使用。
//
// 用法：go run ./tools/devserver -web web -lib <媒体目录> [-port 8099] [-home <数据目录>]
// 默认监听 127.0.0.1，不会打开浏览器，也不会弹任何窗口。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"hestia/internal/config"
	"hestia/internal/index"
	"hestia/internal/progress"
	"hestia/internal/server"
)

func main() {
	webDir := flag.String("web", "web", "前端资源目录（含 index.html 与 assets/）")
	libDir := flag.String("lib", "", "启动时挂载并扫描的媒体库目录")
	port := flag.Int("port", 8099, "监听端口")
	home := flag.String("home", "", "数据目录（config.json / data 所在地，默认临时目录）")
	flag.Parse()

	if *home == "" {
		d, err := os.MkdirTemp("", "hestia-dev-*")
		if err != nil {
			log.Fatalf("创建临时目录失败: %v", err)
		}
		*home = d
		defer os.RemoveAll(d)
	}
	if err := os.MkdirAll(*home, 0o755); err != nil {
		log.Fatalf("创建数据目录失败: %v", err)
	}

	cfg := config.Default()
	cfg.Port = *port
	cfg.Listen = "127.0.0.1"
	cfg.OpenBrowser = false
	cfg.AutoStart = false
	if *libDir != "" {
		abs, err := filepath.Abs(*libDir)
		if err != nil {
			log.Fatalf("解析媒体目录失败: %v", err)
		}
		cfg.Libraries = []config.Library{{Path: abs, Enabled: true}}
	}
	cfgPath := filepath.Join(*home, "config.json")
	cfgRaw, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(cfgPath, cfgRaw, 0o644); err != nil {
		log.Fatalf("写入配置失败: %v", err)
	}

	logger := log.New(os.Stderr, "[dev] ", log.LstdFlags)
	dataDir := filepath.Join(*home, "data")
	cfgM, err := config.Load(cfgPath)
	if err != nil {
		logger.Fatalf("加载配置失败: %v", err)
	}
	store, err := index.New(dataDir)
	if err != nil {
		logger.Fatalf("初始化索引失败: %v", err)
	}
	prog, err := progress.New(dataDir)
	if err != nil {
		logger.Fatalf("初始化进度存储失败: %v", err)
	}
	tc := server.DetectToolchain()
	tr := server.NewTranscoder(tc.FFmpeg, dataDir, func(mediaID string) (string, error) {
		abs, _, err := store.ResolveMedia(mediaID)
		return abs, err
	})
	srv, err := server.New(cfgM, store, tc, tr, prog, dataDir, os.DirFS(*webDir), logger)
	if err != nil {
		logger.Fatalf("初始化服务失败: %v", err)
	}

	// 挂载并扫描媒体库（同步完成，便于测试断言）
	for _, l := range cfg.Libraries {
		id := store.LibID(config.AbsPath(l.Path))
		if _, ok := store.GetLibrary(id); !ok {
			if _, err := store.AddLibrary(config.AbsPath(l.Path), l.Label); err != nil {
				logger.Printf("添加媒体库失败: %v", err)
				continue
			}
		}
		if err := store.RescanLibrary(id); err != nil {
			logger.Printf("扫描失败: %v", err)
		}
		logger.Printf("媒体库已扫描: %s (%s)", l.Path, id)
	}

	if err := srv.ListenAndServe(); err != nil {
		logger.Fatalf("启动失败: %v", err)
	}
	fmt.Printf("DEVSERVER_READY http://127.0.0.1:%d home=%s ffmpeg=%v\n", srv.Port(), *home, tc.FFmpegOK())

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	srv.StopListener()
	tr.Close()
}

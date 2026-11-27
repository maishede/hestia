//go:build !windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"hestia/internal/config"
)

func setAutoStart(bool) error { return errors.New("开机自启仅支持 Windows") }

func runUpdateHelperIfRequested() bool { return false }

func singleInstanceTaken() bool { return false }

func showStartup() func() { return func() {} }

func runApp(a *app, _ func()) {
	if err := a.srv.ListenAndServe(); err != nil {
		fatal(a.logger, "%v", err)
	}
	if actual := a.srv.Port(); actual != a.cfgM.Get().Port {
		_ = a.cfgM.Update(func(c *config.Config) { c.Port = actual })
	}
	a.banner()
	if a.cfgM.Get().OpenBrowser {
		go func() {
			time.Sleep(600 * time.Millisecond)
			if url := a.srv.LocalURL(); url != "" {
				openBrowser(url)
			}
		}()
	}
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	fmt.Println("\n正在退出…")
	a.shutdown()
}

// shellOpen 非 Windows 平台用系统默认方式打开。
func shellOpen(target string) {
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("open", target)
	} else {
		cmd = exec.Command("xdg-open", target)
	}
	_ = cmd.Start()
}

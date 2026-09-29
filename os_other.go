//go:build !windows

package main

import (
	"errors"
	"os/exec"
	"runtime"
)

func setAutoStart(bool) error { return errors.New("开机自启仅支持 Windows") }

func runUpdateHelperIfRequested() bool { return false }

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

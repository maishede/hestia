//go:build windows

package server

import (
	"os/exec"
	"syscall"
)

// hideChildWindow 阻止控制台子进程（ffmpeg/ffprobe）弹出黑色窗口。
// 父进程为 GUI 子系统（无控制台）时，控制台子进程默认会新建可见控制台。
func hideChildWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

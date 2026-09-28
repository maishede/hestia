//go:build windows

package main

import (
	"os"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

var (
	oeShell32       = syscall.NewLazyDLL("shell32.dll")
	oeShellExecuteW = oeShell32.NewProc("ShellExecuteW")
)

func t16o(x string) *uint16 {
	p, err := syscall.UTF16PtrFromString(x)
	if err != nil {
		return nil
	}
	return p
}

// shellOpen 用 ShellExecuteW 打开 URL / 文件（不经过 cmd，无控制台闪烁）。
func shellOpen(target string) {
	pT := t16o(target)
	pVerb := t16o("open")
	if pT == nil || pVerb == nil {
		return
	}
	oeShellExecuteW.Call(0, uintptr(unsafe.Pointer(pVerb)), uintptr(unsafe.Pointer(pT)), 0, 0, 5)
}

func timeSleep(ms int) { time.Sleep(time.Duration(ms) * time.Millisecond) }

// setAutoStart 写入/移除 HKCU Run 注册表项（开机自启）。
func setAutoStart(enabled bool) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if enabled {
		return k.SetStringValue("Hestia", "\""+exe+"\"")
	}
	if err := k.DeleteValue("Hestia"); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}

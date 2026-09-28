//go:build windows

package main

import (
	"os"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	user32             = syscall.NewLazyDLL("user32.dll")
	pGetConsoleWindow  = kernel32.NewProc("GetConsoleWindow")
	pShowWindow        = user32.NewProc("ShowWindow")
	pIsWindowVisible   = user32.NewProc("IsWindowVisible")
)

func hideConsole(hide bool) {
	hwnd, _, _ := pGetConsoleWindow.Call()
	if hwnd == 0 {
		return
	}
	cmd := uintptr(5) // SW_SHOW
	if hide {
		cmd = 0 // SW_HIDE
	}
	_, _, _ = pShowWindow.Call(hwnd, cmd)
}

func toggleConsole() {
	hwnd, _, _ := pGetConsoleWindow.Call()
	if hwnd == 0 {
		return
	}
	vis, _, _ := pIsWindowVisible.Call(hwnd)
	if vis != 0 {
		_, _, _ = pShowWindow.Call(hwnd, 0)
	} else {
		_, _, _ = pShowWindow.Call(hwnd, 5)
	}
}

// setAutoStart 写入/移除 HKCU Run 注册表项（开机自启，附带 -hide 静默启动）。
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
		return k.SetStringValue("Hestia", "\""+exe+"\" -hide")
	}
	if err := k.DeleteValue("Hestia"); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}

//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

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

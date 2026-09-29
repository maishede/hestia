package main

import (
	_ "embed"
	"syscall"
	"unsafe"
)

// 应用图标（icon-master.png 生成，2026-09-29）。
//
//go:embed icon-32.png
var icon32PNG []byte

//go:embed icon-256.png
var icon256PNG []byte

// hiconFromPNG 用 CreateIconFromResourceEx 从 PNG 数据构建 HICON（Vista+）。
func hiconFromPNG(png []byte) syscall.Handle {
	h, _, _ := pTCreateIconFromRes.Call(
		uintptr(unsafe.Pointer(&png[0])), uintptr(len(png)), 1, 0x00030000, 0, 0, 0)
	return syscall.Handle(h)
}

// appIcons 窗口/托盘图标句柄（进程内缓存一次）。
var (
	appIconSmall syscall.Handle // 32px：托盘、窗口小图标
	appIconLarge syscall.Handle // 256px：窗口大图标（Alt-Tab）
	iconsLoaded  bool
)

func loadAppIcons() {
	if iconsLoaded {
		return
	}
	appIconSmall = hiconFromPNG(icon32PNG)
	appIconLarge = hiconFromPNG(icon256PNG)
	if appIconSmall == 0 {
		h, _, _ := pTLoadIconW.Call(0, 32512)
		appIconSmall = syscall.Handle(h)
	}
	if appIconLarge == 0 {
		appIconLarge = appIconSmall
	}
	iconsLoaded = true
}

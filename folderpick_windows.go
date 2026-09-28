//go:build windows

package main

// 文件夹选择对话框（SHBrowseForFolder，新版样式，支持新建文件夹）。

import (
	"runtime"
	"syscall"
	"unsafe"
)

var (
	tOle32              = syscall.NewLazyDLL("ole32.dll")
	pTCoInitializeEx    = tOle32.NewProc("CoInitializeEx")
	pTCoTaskMemFree     = tOle32.NewProc("CoTaskMemFree")
	pTSHBrowseForFolder = tShell32.NewProc("SHBrowseForFolderW")
	pTSHGetPathFromIDList = tShell32.NewProc("SHGetPathFromIDListW")
)

const (
	bifReturnOnlyFSDirs = 0x0001
	bifNewDialogStyle   = 0x0040
	coinitApartmentThreaded = 0x2
	maxPathW            = 32768
)

type browseInfoW struct {
	HwndOwner      syscall.Handle
	PidlRoot       uintptr
	DisplayName    uintptr
	Title          uintptr
	Flags          uint32
	Callback       uintptr
	LParam         uintptr
	Image          int32
}

// pickFolderDialog 弹出原生文件夹选择框；取消返回空串。
// 在独立系统线程上运行（COM STA），不阻塞 WebView2 消息循环。
func pickFolderDialog(title string) string {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pTCoInitializeEx.Call(0, coinitApartmentThreaded)
	defer pTCoTaskMemFree.Call(0)

	nameBuf := make([]uint16, maxPathW)
	bi := browseInfoW{
		DisplayName: uintptr(unsafe.Pointer(&nameBuf[0])),
		Title:       t16(title),
		Flags:       bifReturnOnlyFSDirs | bifNewDialogStyle,
	}
	pidl, _, _ := pTSHBrowseForFolder.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return ""
	}
	defer pTCoTaskMemFree.Call(pidl)
	p := make([]uint16, 260)
	r, _, _ := pTSHGetPathFromIDList.Call(pidl, uintptr(unsafe.Pointer(&p[0])))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(p)
}
